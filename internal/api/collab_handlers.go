package api

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/collab"
	"typstpad/internal/store"
)

// handleCollabToken issues a short-lived websocket JWT for one file's Y.Doc.
// Editors and owners get read-write; suggesters and viewers get read-only.
func (s *Server) handleCollabToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	f, p, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "viewer")
	if !ok {
		return
	}
	if f.Kind != "text" {
		writeErr(w, http.StatusBadRequest, "not a text file")
		return
	}
	mode := "ro"
	if roleAtLeast(p.Role, "editor") && auth.HasScope(r.Context(), "write") {
		mode = "rw"
	}
	token, err := s.Collab.MintToken(f.ID, u.ID, u.Name, u.Color, mode)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"token": token,
		"url":   "/collab",
		"doc":   f.ID,
		"mode":  mode,
	})
}

// handleApplyEdit applies a range edit through the CRDT — the REST/MCP/CLI
// write path. Concurrent browser editors merge via Yjs.
func (s *Server) handleApplyEdit(w http.ResponseWriter, r *http.Request) {
	f, p, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "editor")
	if !ok {
		return
	}
	var req struct {
		From    *int    `json:"from"`
		To      *int    `json:"to"`
		Insert  string  `json:"insert"`
		Content *string `json:"content"` // full-content replace (diff-based)
	}
	if !readJSON(w, r, &req) {
		return
	}
	var err error
	switch {
	case req.Content != nil:
		err = s.Collab.SetContent(r.Context(), f.ID, *req.Content)
	case req.From != nil && req.To != nil:
		if *req.From < 0 || *req.To < *req.From {
			writeErr(w, http.StatusBadRequest, "invalid range")
			return
		}
		err = s.Collab.Edit(r.Context(), f.ID, collab.EditRequest{From: *req.From, To: *req.To, Insert: req.Insert, Origin: "api"})
	default:
		writeErr(w, http.StatusBadRequest, "provide either content or from/to")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	_ = s.Store.TouchProject(r.Context(), p.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// currentText returns live text from the sidecar when the doc is loaded there,
// falling back to the Postgres mirror.
func (s *Server) currentText(ctx context.Context, fileID string) (string, error) {
	text, err := s.Collab.Text(ctx, fileID, true)
	if err == nil {
		return text, nil
	}
	return s.Store.FileContent(ctx, fileID)
}

// Internal endpoints, called only by the collab sidecar.

func (s *Server) requireInternal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Internal-Secret")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.Cfg.CollabSecret)) != 1 {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) mountInternalRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(s.requireInternal)
		r.Get("/internal/ydoc/{fileID}", s.handleInternalGetYDoc)
		r.Put("/internal/ydoc/{fileID}", s.handleInternalPutYDoc)
	})
}

// handleInternalGetYDoc returns the persisted Y.Doc state (204 if none yet,
// in which case the sidecar seeds the doc from the "content" field).
func (s *Server) handleInternalGetYDoc(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")
	state, err := s.Store.YjsState(r.Context(), fileID)
	if errors.Is(err, store.ErrNotFound) {
		content, cerr := s.Store.FileContent(r.Context(), fileID)
		if cerr != nil {
			fail(w, cerr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"state": nil, "content": content})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"state": base64.StdEncoding.EncodeToString(state),
	})
}

func (s *Server) handleInternalPutYDoc(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")
	var req struct {
		State string `json:"state"` // base64 Yjs update
		Text  string `json:"text"`  // extracted plain text mirror
	}
	if !readJSON(w, r, &req) {
		return
	}
	state, err := base64.StdEncoding.DecodeString(req.State)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid base64 state")
		return
	}
	f, err := s.Store.FileByID(r.Context(), fileID)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.UpsertYjsState(r.Context(), fileID, state); err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.UpsertFileContent(r.Context(), fileID, req.Text); err != nil {
		fail(w, err)
		return
	}
	_ = s.Store.TouchProject(r.Context(), f.ProjectID)
	if s.OnDocStored != nil {
		s.OnDocStored(f.ProjectID)
	}
	s.Hub.Publish(f.ProjectID, Event{Type: "doc.stored", Payload: map[string]string{"fileId": f.ID}})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
