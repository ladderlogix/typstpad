package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/compile"
	"typstpad/internal/store"
)

// materialize gathers all project files (live text + asset bytes) for a
// server-side compile.
func (s *Server) materialize(r *http.Request, projectID string) ([]compile.JobFile, error) {
	files, err := s.Store.ListFiles(r.Context(), projectID)
	if err != nil {
		return nil, err
	}
	out := make([]compile.JobFile, 0, len(files))
	for _, f := range files {
		switch f.Kind {
		case "text":
			text, err := s.currentText(r.Context(), f.ID)
			if err != nil {
				return nil, err
			}
			out = append(out, compile.JobFile{Path: f.Path, Data: []byte(text)})
		case "asset":
			data, err := s.Blob.Get(f.BlobHash)
			if err != nil {
				return nil, err
			}
			out = append(out, compile.JobFile{Path: f.Path, Data: data})
		}
	}
	return out, nil
}

func (s *Server) runCompile(w http.ResponseWriter, r *http.Request, p *store.Project) *compile.Result {
	jobFiles, err := s.materialize(r, p.ID)
	if err != nil {
		fail(w, err)
		return nil
	}
	res, err := s.Compiler.Compile(r.Context(), p.MainPath, jobFiles)
	if err != nil {
		fail(w, err)
		return nil
	}
	return res
}

func (s *Server) handleCompile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	res := s.runCompile(w, r, p)
	if res == nil {
		return
	}
	if res.Diagnostics == nil {
		res.Diagnostics = []compile.Diagnostic{}
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExportPDF(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	res := s.runCompile(w, r, p)
	if res == nil {
		return
	}
	if !res.OK {
		writeJSON(w, http.StatusUnprocessableEntity, res)
		return
	}
	name := strings.ReplaceAll(p.Name, `"`, "") + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(res.PDF)
}

// handlePackageProxy proxies Typst Universe package downloads for the
// in-browser WASM compiler (avoids CORS), disk-cached.
func (s *Server) handlePackageProxy(w http.ResponseWriter, r *http.Request) {
	rest := chi.URLParam(r, "*")
	if strings.Contains(rest, "..") {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	s.cachedProxy(w, r, "https://packages.typst.org/"+rest, "package-cache", "")
}
