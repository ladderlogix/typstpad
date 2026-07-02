package api

import (
	"archive/zip"
	"bytes"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
)

// handleExportZip streams the whole project (live text + assets) as a .zip.
func (s *Server) handleExportZip(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	files, err := s.Store.ListFiles(r.Context(), p.ID)
	if err != nil {
		fail(w, err)
		return
	}
	name := strings.ReplaceAll(p.Name, `"`, "") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, f := range files {
		fw, err := zw.Create(f.Path)
		if err != nil {
			return
		}
		switch f.Kind {
		case "text":
			content, err := s.currentText(r.Context(), f.ID)
			if err != nil {
				return
			}
			_, _ = fw.Write([]byte(content))
		case "asset":
			data, err := s.Blob.Get(f.BlobHash)
			if err != nil {
				return
			}
			_, _ = fw.Write(data)
		}
	}
}

const maxImportSize = 50 << 20

var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".pdf": true, ".ttf": true, ".otf": true, ".woff": true, ".woff2": true,
	".svg": true, ".ico": true, ".bmp": true,
}

// handleImportZip creates a new project from an uploaded .zip.
func (s *Server) handleImportZip(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if !s.checkProjectQuota(w, r) {
		return
	}
	if err := r.ParseMultipartForm(maxImportSize); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImportSize+1))
	if err != nil {
		fail(w, err)
		return
	}
	if len(data) > maxImportSize {
		writeErr(w, http.StatusRequestEntityTooLarge, "zip too large (max 50MB)")
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "not a valid zip file")
		return
	}

	name := strings.TrimSuffix(header.Filename, ".zip")
	if name == "" {
		name = "Imported project"
	}
	p, err := s.Store.CreateProject(r.Context(), name, "", u.ID)
	if err != nil {
		fail(w, err)
		return
	}

	// Strip a single common top-level directory (zips often nest everything).
	prefix := commonZipPrefix(zr.File)
	var addedMain, addedAny bool
	var firstTyp string
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		p2 := cleanPath(strings.TrimPrefix(zf.Name, prefix))
		if !validProjectPath(p2) || strings.HasPrefix(path.Base(p2), ".") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		content, _ := io.ReadAll(io.LimitReader(rc, maxImportSize))
		rc.Close()

		ext := strings.ToLower(path.Ext(p2))
		if binaryExts[ext] || !utf8.Valid(content) {
			hash, err := s.Blob.Put(content)
			if err != nil {
				continue
			}
			_ = s.Store.UpsertBlob(r.Context(), hash, int64(len(content)))
			mimeType := mime.TypeByExtension(ext)
			_, _ = s.Store.CreateAssetFile(r.Context(), p.ID, p2, hash, mimeType, int64(len(content)))
		} else {
			if _, err := s.Store.CreateTextFile(r.Context(), p.ID, p2, string(content)); err != nil {
				continue
			}
			if firstTyp == "" && ext == ".typ" {
				firstTyp = p2
			}
			if p2 == "main.typ" {
				addedMain = true
			}
		}
		addedAny = true
	}
	if !addedAny {
		_ = s.Store.SoftDeleteProject(r.Context(), p.ID)
		writeErr(w, http.StatusBadRequest, "the zip contained no importable files")
		return
	}
	// Point main_path at the entry file we found.
	if !addedMain && firstTyp != "" {
		_ = s.Store.UpdateProject(r.Context(), p.ID, nil, nil, &firstTyp)
		p.MainPath = firstTyp
	}
	writeJSON(w, http.StatusCreated, p)
}

// commonZipPrefix returns a single shared top-level directory ("foo/") if every
// entry lives under it, else "".
func commonZipPrefix(files []*zip.File) string {
	var prefix string
	for i, f := range files {
		top := f.Name
		if j := strings.IndexByte(top, '/'); j >= 0 {
			top = top[:j+1]
		} else {
			return "" // a top-level file exists; no common dir
		}
		if i == 0 {
			prefix = top
		} else if top != prefix {
			return ""
		}
	}
	return prefix
}
