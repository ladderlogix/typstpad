package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	snaps, err := s.Store.ListSnapshots(r.Context(), p.ID, 500)
	if err != nil {
		fail(w, err)
		return
	}
	if snaps == nil {
		snaps = []*store.Snapshot{}
	}
	writeJSON(w, http.StatusOK, snaps)
}

func (s *Server) handleCreateVersion(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "editor")
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	snap, err := s.Versions.Snapshot(r.Context(), p.ID, "named", &req.Name, &u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, snap)
}

// handleGetVersion returns the snapshot manifest with text contents inlined.
func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	snap, files, ok := s.versionAccess(w, r)
	if !ok {
		return
	}
	type versionFile struct {
		Path    string  `json:"path"`
		Kind    string  `json:"kind"`
		Content *string `json:"content,omitempty"`
	}
	out := make([]versionFile, 0, len(files))
	for _, f := range files {
		vf := versionFile{Path: f.Path, Kind: f.Kind}
		if f.Kind == "text" {
			data, err := s.Blob.Get(f.ContentHash)
			if err != nil {
				fail(w, err)
				return
			}
			content := string(data)
			vf.Content = &content
		}
		out = append(out, vf)
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshot": snap, "files": out})
}

func (s *Server) versionAccess(w http.ResponseWriter, r *http.Request) (*store.Snapshot, []store.SnapshotFile, bool) {
	snap, err := s.Store.SnapshotByID(r.Context(), chi.URLParam(r, "versionID"))
	if err != nil {
		fail(w, err)
		return nil, nil, false
	}
	if _, ok := s.projectAccess(w, r, snap.ProjectID, "viewer"); !ok {
		return nil, nil, false
	}
	files, err := s.Store.SnapshotFiles(r.Context(), snap.ID)
	if err != nil {
		fail(w, err)
		return nil, nil, false
	}
	return snap, files, true
}

// handleDiff returns per-file old/new text between two snapshots, or between
// a snapshot and the live project (to=current or omitted).
func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		writeErr(w, http.StatusBadRequest, "from snapshot id required")
		return
	}
	oldFiles, err := s.snapshotTexts(r, p.ID, from)
	if err != nil {
		fail(w, err)
		return
	}
	var newFiles map[string]string
	if to == "" || to == "current" {
		newFiles, err = s.liveTexts(r, p.ID)
	} else {
		newFiles, err = s.snapshotTexts(r, p.ID, to)
	}
	if err != nil {
		fail(w, err)
		return
	}

	type diffFile struct {
		Path   string `json:"path"`
		Status string `json:"status"` // added | removed | modified
		Old    string `json:"old"`
		New    string `json:"new"`
	}
	var out []diffFile
	for path, oldText := range oldFiles {
		newText, exists := newFiles[path]
		switch {
		case !exists:
			out = append(out, diffFile{Path: path, Status: "removed", Old: oldText})
		case newText != oldText:
			out = append(out, diffFile{Path: path, Status: "modified", Old: oldText, New: newText})
		}
		delete(newFiles, path)
	}
	for path, newText := range newFiles {
		out = append(out, diffFile{Path: path, Status: "added", New: newText})
	}
	if out == nil {
		out = []diffFile{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) snapshotTexts(r *http.Request, projectID, snapshotID string) (map[string]string, error) {
	snap, err := s.Store.SnapshotByID(r.Context(), snapshotID)
	if err != nil {
		return nil, err
	}
	if snap.ProjectID != projectID {
		return nil, store.ErrNotFound
	}
	files, err := s.Store.SnapshotFiles(r.Context(), snap.ID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		if f.Kind != "text" {
			continue
		}
		data, err := s.Blob.Get(f.ContentHash)
		if err != nil {
			return nil, err
		}
		out[f.Path] = string(data)
	}
	return out, nil
}

func (s *Server) liveTexts(r *http.Request, projectID string) (map[string]string, error) {
	files, err := s.Store.ListFiles(r.Context(), projectID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		if f.Kind != "text" {
			continue
		}
		text, err := s.currentText(r.Context(), f.ID)
		if err != nil {
			return nil, err
		}
		out[f.Path] = text
	}
	return out, nil
}

func (s *Server) handleRestoreVersion(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	snap, err := s.Store.SnapshotByID(r.Context(), chi.URLParam(r, "versionID"))
	if err != nil {
		fail(w, err)
		return
	}
	if _, ok := s.projectAccess(w, r, snap.ProjectID, "editor"); !ok {
		return
	}
	if err := s.Versions.Restore(r.Context(), snap.ProjectID, snap.ID, u.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
