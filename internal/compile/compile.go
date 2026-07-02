// Package compile runs the native typst CLI over a materialized project
// directory, with timeouts and bounded concurrency.
package compile

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Compiler struct {
	TypstBin string
	WorkDir  string // scratch space for compile jobs
	FontDirs string // TYPST_FONT_PATHS value ("" = typst defaults)
	CacheDir string // TYPST_PACKAGE_CACHE_PATH (Typst Universe cache)
	Timeout  time.Duration
	sem      chan struct{}
}

func New(typstBin, workDir, cacheDir string, timeout time.Duration) (*Compiler, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	if cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return nil, err
		}
	}
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	return &Compiler{
		TypstBin: typstBin,
		WorkDir:  workDir,
		CacheDir: cacheDir,
		Timeout:  timeout,
		sem:      make(chan struct{}, n),
	}, nil
}

type JobFile struct {
	Path string
	Data []byte
}

type Diagnostic struct {
	Severity string `json:"severity"` // error | warning
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Message  string `json:"message"`
}

type Result struct {
	OK          bool         `json:"ok"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	PDF         []byte       `json:"-"`
}

// short diagnostic format: <path>:<line>:<col>: <severity>: <message>
var shortDiagRe = regexp.MustCompile(`^(.+?):(\d+):(\d+): (error|warning): (.*)$`)

// bare format without position: <severity>: <message>
var bareDiagRe = regexp.MustCompile(`^(error|warning): (.*)$`)

// Compile materializes files into a job dir and runs `typst compile`.
func (c *Compiler) Compile(ctx context.Context, mainPath string, files []JobFile) (*Result, error) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	jobDir, err := os.MkdirTemp(c.WorkDir, "job-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(jobDir)

	for _, f := range files {
		dst := filepath.Join(jobDir, filepath.FromSlash(f.Path))
		if !strings.HasPrefix(dst, jobDir+string(os.PathSeparator)) {
			return nil, fmt.Errorf("path escapes job dir: %s", f.Path)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, f.Data, 0o644); err != nil {
			return nil, err
		}
	}

	outPDF := filepath.Join(jobDir, "__out.pdf")
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.TypstBin, "compile",
		"--root", jobDir,
		"--diagnostic-format", "short",
		filepath.Join(jobDir, filepath.FromSlash(mainPath)),
		outPDF)
	cmd.Env = append(os.Environ(), "TYPST_PACKAGE_CACHE_PATH="+c.CacheDir)
	if c.FontDirs != "" {
		cmd.Env = append(cmd.Env, "TYPST_FONT_PATHS="+c.FontDirs)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = 2 * time.Second

	runErr := cmd.Run()
	res := &Result{Diagnostics: parseDiagnostics(stderr.String(), jobDir)}
	if ctx.Err() == context.DeadlineExceeded {
		res.Diagnostics = append(res.Diagnostics, Diagnostic{
			Severity: "error",
			Message:  fmt.Sprintf("compilation timed out after %s", c.Timeout),
		})
		return res, nil
	}
	if runErr == nil {
		res.OK = true
		pdf, err := os.ReadFile(outPDF)
		if err != nil {
			return nil, err
		}
		res.PDF = pdf
	} else if len(res.Diagnostics) == 0 {
		res.Diagnostics = append(res.Diagnostics, Diagnostic{
			Severity: "error",
			Message:  strings.TrimSpace(stderr.String()),
		})
	}
	return res, nil
}

func parseDiagnostics(stderr, jobDir string) []Diagnostic {
	var out []Diagnostic
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := shortDiagRe.FindStringSubmatch(line); m != nil {
			lineNo, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			file := strings.TrimPrefix(m[1], jobDir)
			file = strings.TrimPrefix(filepath.ToSlash(file), "/")
			out = append(out, Diagnostic{
				Severity: m[4], File: file, Line: lineNo, Col: col, Message: m[5],
			})
			continue
		}
		if m := bareDiagRe.FindStringSubmatch(line); m != nil {
			out = append(out, Diagnostic{Severity: m[1], Message: m[2]})
		}
	}
	return out
}
