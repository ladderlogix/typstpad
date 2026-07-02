package api

import (
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/store"
)

func cleanPath(p string) string {
	p = path.Clean("/" + strings.TrimSpace(p))
	return strings.TrimPrefix(p, "/")
}

func validProjectPath(p string) bool {
	return p != "" && p != "." && !strings.HasPrefix(p, "..") && !strings.Contains(p, "/../")
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	files, err := s.Store.ListFiles(r.Context(), p.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if files == nil {
		files = []*store.File{}
	}
	writeJSON(w, http.StatusOK, files)
}

// handleCreateFile creates a text file (JSON body) or uploads an asset
// (multipart form with "file" + "path").
func (s *Server) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "editor")
	if !ok {
		return
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		s.handleUploadAsset(w, r, p)
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Path = cleanPath(req.Path)
	if !validProjectPath(req.Path) {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	f, err := s.Store.CreateTextFile(r.Context(), p.ID, req.Path, req.Content)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "a file with this path already exists")
			return
		}
		fail(w, err)
		return
	}
	_ = s.Store.TouchProject(r.Context(), p.ID)
	s.Hub.Publish(p.ID, Event{Type: "files.changed"})
	writeJSON(w, http.StatusCreated, f)
}

const maxAssetSize = 50 << 20

func (s *Server) handleUploadAsset(w http.ResponseWriter, r *http.Request, p *store.Project) {
	if err := r.ParseMultipartForm(maxAssetSize); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()
	fpath := cleanPath(r.FormValue("path"))
	if fpath == "" {
		fpath = cleanPath(header.Filename)
	}
	if !validProjectPath(fpath) {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAssetSize+1))
	if err != nil {
		fail(w, err)
		return
	}
	if len(data) > maxAssetSize {
		writeErr(w, http.StatusRequestEntityTooLarge, "asset too large (max 50MB)")
		return
	}
	if !s.checkAssetQuota(w, r, p.OwnerID, int64(len(data))) {
		return
	}
	hash, err := s.Blob.Put(data)
	if err != nil {
		fail(w, err)
		return
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = mime.TypeByExtension(path.Ext(fpath))
	}
	if err := s.Store.UpsertBlob(r.Context(), hash, int64(len(data))); err != nil {
		fail(w, err)
		return
	}
	f, err := s.Store.CreateAssetFile(r.Context(), p.ID, fpath, hash, mimeType, int64(len(data)))
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "a file with this path already exists")
			return
		}
		fail(w, err)
		return
	}
	_ = s.Store.TouchProject(r.Context(), p.ID)
	s.Hub.Publish(p.ID, Event{Type: "files.changed"})
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	f, _, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "viewer")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleFileContent(w http.ResponseWriter, r *http.Request) {
	f, _, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "viewer")
	if !ok {
		return
	}
	if f.Kind != "text" {
		writeErr(w, http.StatusBadRequest, "not a text file")
		return
	}
	content, err := s.currentText(r.Context(), f.ID)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

func (s *Server) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	f, p, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "editor")
	if !ok {
		return
	}
	if fileLockedFor(f, p) {
		writeErr(w, http.StatusForbidden, "this file is locked; only the owner can rename it")
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Path = cleanPath(req.Path)
	if !validProjectPath(req.Path) {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	if err := s.Store.RenameFile(r.Context(), f.ID, req.Path); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "a file with this path already exists")
			return
		}
		fail(w, err)
		return
	}
	_ = s.Store.TouchProject(r.Context(), p.ID)
	s.Hub.Publish(p.ID, Event{Type: "files.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSetFileLock locks/unlocks a file. Only the project owner may change it.
func (s *Server) handleSetFileLock(w http.ResponseWriter, r *http.Request) {
	f, p, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "owner")
	if !ok {
		return
	}
	var req struct {
		Locked bool `json:"locked"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Store.SetFileLocked(r.Context(), f.ID, req.Locked); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(p.ID, Event{Type: "files.changed"})
	writeJSON(w, http.StatusOK, map[string]bool{"locked": req.Locked})
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	f, p, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "editor")
	if !ok {
		return
	}
	if fileLockedFor(f, p) {
		writeErr(w, http.StatusForbidden, "this file is locked; only the owner can delete it")
		return
	}
	if f.Path == p.MainPath {
		writeErr(w, http.StatusBadRequest, "cannot delete the main file")
		return
	}
	if err := s.Store.DeleteFile(r.Context(), f.ID); err != nil {
		fail(w, err)
		return
	}
	_ = s.Store.TouchProject(r.Context(), p.ID)
	s.Hub.Publish(p.ID, Event{Type: "files.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAssetBytes streams asset bytes (session-cookie auth; used by <img> and
// the compiler worker).
func (s *Server) handleAssetBytes(w http.ResponseWriter, r *http.Request) {
	f, _, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "viewer")
	if !ok {
		return
	}
	if f.Kind != "asset" {
		writeErr(w, http.StatusBadRequest, "not an asset")
		return
	}
	blob, err := s.Blob.Open(f.BlobHash)
	if err != nil {
		fail(w, err)
		return
	}
	defer blob.Close()
	if f.Mime != "" {
		w.Header().Set("Content-Type", f.Mime)
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, path.Base(f.Path), f.CreatedAt, blob)
}
