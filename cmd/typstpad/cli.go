package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ---- API client ----

type clientConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "typstpad", "config.json")
}

func loadConfig() (*clientConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("not logged in — run `typstpad login` first (%w)", err)
	}
	var c clientConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

type apiClient struct {
	url   string
	token string
	http  *http.Client
}

func newClient() (*apiClient, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	return &apiClient{url: cfg.URL, token: cfg.Token, http: &http.Client{Timeout: 120 * time.Second}}, nil
}

func (c *apiClient) req(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, strings.TrimSuffix(c.url, "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	return resp, nil
}

func (c *apiClient) do(method, path string, body, out any) error {
	var rdr io.Reader
	ct := ""
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(data)
		ct = "application/json"
	}
	resp, err := c.req(method, path, rdr, ct)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

type fileInfo struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func (c *apiClient) listFiles(projectID string) ([]fileInfo, error) {
	var files []fileInfo
	err := c.do("GET", "/api/projects/"+projectID+"/files", nil, &files)
	return files, err
}

// ---- commands ----

func cliCommands() []*cobra.Command {
	return []*cobra.Command{
		loginCmd(), projectsCmd(), pullCmd(), pushCmd(), compileClientCmd(), watchCmd(), mcpCmd(),
	}
}

func loginCmd() *cobra.Command {
	var url, token string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save server URL and API token (create one under Settings → API tokens)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if url == "" || token == "" {
				return fmt.Errorf("--url and --token are required")
			}
			c := &apiClient{url: url, token: token, http: &http.Client{Timeout: 15 * time.Second}}
			var me struct {
				Email string `json:"email"`
			}
			if err := c.do("GET", "/api/auth/me", nil, &me); err != nil {
				return fmt.Errorf("login check failed: %w", err)
			}
			data, _ := json.MarshalIndent(clientConfig{URL: url, Token: token}, "", "  ")
			if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(configPath(), data, 0o600); err != nil {
				return err
			}
			fmt.Printf("Logged in as %s (%s)\n", me.Email, url)
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "server URL, e.g. http://server:8080")
	cmd.Flags().StringVar(&token, "token", "", "API token (tfp_...)")
	return cmd
}

func projectsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "projects", Short: "Project commands"}
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List your projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			var projects []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Role string `json:"role"`
			}
			if err := c.do("GET", "/api/projects", nil, &projects); err != nil {
				return err
			}
			w := bufio.NewWriter(os.Stdout)
			defer w.Flush()
			for _, p := range projects {
				fmt.Fprintf(w, "%s  %-10s %s\n", p.ID, p.Role, p.Name)
			}
			return nil
		},
	})
	return cmd
}

func pullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <projectID> [dir]",
		Short: "Download all project files to a local directory",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			dir := "."
			if len(args) > 1 {
				dir = args[1]
			}
			files, err := c.listFiles(args[0])
			if err != nil {
				return err
			}
			for _, f := range files {
				endpoint := "/api/files/" + f.ID + "/content"
				if f.Kind == "asset" {
					endpoint = "/api/assets/" + f.ID
				}
				resp, err := c.req("GET", endpoint, nil, "")
				if err != nil {
					return err
				}
				data, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					return err
				}
				dst := filepath.Join(dir, filepath.FromSlash(f.Path))
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dst, data, 0o644); err != nil {
					return err
				}
				fmt.Printf("pulled %s (%d bytes)\n", f.Path, len(data))
			}
			return nil
		},
	}
}

func pushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <projectID> [dir]",
		Short: "Upload local files into the project (merges with live editors)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			projectID := args[0]
			dir := "."
			if len(args) > 1 {
				dir = args[1]
			}
			remote, err := c.listFiles(projectID)
			if err != nil {
				return err
			}
			remoteByPath := map[string]fileInfo{}
			for _, f := range remote {
				remoteByPath[f.Path] = f
			}
			return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, err := filepath.Rel(dir, path)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if strings.HasPrefix(rel, ".") {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				isText := utf8Valid(data) && !isBinaryExt(rel)
				if rf, ok := remoteByPath[rel]; ok {
					if rf.Kind == "text" && isText {
						content := string(data)
						err = c.do("POST", "/api/files/"+rf.ID+"/edit", map[string]any{"content": content}, nil)
						if err == nil {
							fmt.Printf("pushed %s\n", rel)
						}
						return err
					}
					fmt.Printf("skipped %s (asset overwrite not supported; delete and re-upload)\n", rel)
					return nil
				}
				if isText {
					err = c.do("POST", "/api/projects/"+projectID+"/files",
						map[string]string{"path": rel, "content": string(data)}, nil)
				} else {
					err = c.uploadAsset(projectID, rel, data)
				}
				if err == nil {
					fmt.Printf("created %s\n", rel)
				}
				return err
			})
		},
	}
}

func (c *apiClient) uploadAsset(projectID, path string, data []byte) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("path", path)
	fw, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	mw.Close()
	resp, err := c.req("POST", "/api/projects/"+projectID+"/files", &buf, mw.FormDataContentType())
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func utf8Valid(data []byte) bool {
	return !bytes.Contains(data, []byte{0})
}

func isBinaryExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".pdf", ".ttf", ".otf", ".woff", ".woff2", ".svg":
		return true
	}
	return false
}

func compileClientCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "compile <projectID>",
		Short: "Compile server-side and download the PDF",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			return downloadPDF(c, args[0], out)
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "out.pdf", "output PDF path")
	return cmd
}

func downloadPDF(c *apiClient, projectID, out string) error {
	resp, err := c.req("GET", "/api/projects/"+projectID+"/export/pdf", nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, n)
	return nil
}

func watchCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "watch <projectID>",
		Short: "Recompile and re-download the PDF whenever the project changes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			projectID := args[0]
			if err := downloadPDF(c, projectID, out); err != nil {
				fmt.Fprintln(os.Stderr, "compile failed:", err)
			}
			// Follow the project SSE stream; recompile on content events.
			for {
				if err := watchOnce(c, projectID, out); err != nil {
					fmt.Fprintln(os.Stderr, "stream error, reconnecting in 3s:", err)
					time.Sleep(3 * time.Second)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "out.pdf", "output PDF path")
	return cmd
}

func watchOnce(c *apiClient, projectID, out string) error {
	resp, err := c.req("GET", "/api/projects/"+projectID+"/events", nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var debounce *time.Timer
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if debounce != nil {
			debounce.Stop()
		}
		debounce = time.AfterFunc(1500*time.Millisecond, func() {
			if err := downloadPDF(c, projectID, out); err != nil {
				fmt.Fprintln(os.Stderr, "compile failed:", err)
			}
		})
	}
	return scanner.Err()
}
