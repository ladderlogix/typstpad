package api

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/collab"
	"typstpad/internal/store"
)

func encodeAnchors(sg *store.Suggestion) {
	sg.AnchorStartB64 = base64.StdEncoding.EncodeToString(sg.AnchorStart)
	if sg.AnchorEnd != nil {
		sg.AnchorEndB64 = base64.StdEncoding.EncodeToString(sg.AnchorEnd)
	}
}

func encodeCommentAnchors(c *store.Comment) {
	if c.AnchorStart != nil {
		c.AnchorStartB64 = base64.StdEncoding.EncodeToString(c.AnchorStart)
	}
	if c.AnchorEnd != nil {
		c.AnchorEndB64 = base64.StdEncoding.EncodeToString(c.AnchorEnd)
	}
}

// Suggestions

func (s *Server) handleListSuggestions(w http.ResponseWriter, r *http.Request) {
	f, _, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "viewer")
	if !ok {
		return
	}
	openOnly := r.URL.Query().Get("all") == ""
	suggestions, err := s.Store.ListSuggestions(r.Context(), f.ID, openOnly)
	if err != nil {
		fail(w, err)
		return
	}
	if suggestions == nil {
		suggestions = []*store.Suggestion{}
	}
	for _, sg := range suggestions {
		encodeAnchors(sg)
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// handleCreateSuggestion records a proposed change anchored to the live doc.
// Suggesters (and editors in suggest mode, and AI tools) use this.
func (s *Server) handleCreateSuggestion(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	f, p, ok := s.fileAccess(w, r, chi.URLParam(r, "fileID"), "suggester")
	if !ok {
		return
	}
	var req struct {
		Type string `json:"type"` // insert | delete | replace
		From int    `json:"from"`
		To   int    `json:"to"`
		Text string `json:"text"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Type != "insert" && req.Type != "delete" && req.Type != "replace" {
		writeErr(w, http.StatusBadRequest, "type must be insert, delete or replace")
		return
	}
	if req.From < 0 || req.To < req.From {
		writeErr(w, http.StatusBadRequest, "invalid range")
		return
	}
	if req.Type == "insert" {
		req.To = req.From
	} else if req.To == req.From {
		writeErr(w, http.StatusBadRequest, "delete/replace requires a non-empty range")
		return
	}

	text, err := s.currentText(r.Context(), f.ID)
	if err != nil {
		fail(w, err)
		return
	}
	runes := []rune(text)
	if req.To > len(runes) {
		writeErr(w, http.StatusBadRequest, "range beyond end of document")
		return
	}
	var insertedText, deletedPreview *string
	if req.Type != "delete" {
		insertedText = &req.Text
	}
	if req.Type != "insert" {
		preview := string(runes[req.From:req.To])
		deletedPreview = &preview
	}

	anchorStartB64, anchorEndB64, err := s.Collab.RelPos(r.Context(), f.ID, req.From, req.To)
	if err != nil {
		fail(w, err)
		return
	}
	anchorStart, err := base64.StdEncoding.DecodeString(anchorStartB64)
	if err != nil {
		fail(w, fmt.Errorf("bad anchor from sidecar: %w", err))
		return
	}
	var anchorEnd []byte
	if req.Type != "insert" {
		anchorEnd, err = base64.StdEncoding.DecodeString(anchorEndB64)
		if err != nil {
			fail(w, fmt.Errorf("bad anchor from sidecar: %w", err))
			return
		}
	}

	sg, err := s.Store.CreateSuggestion(r.Context(), p.ID, f.ID, u.ID, req.Type, anchorStart, anchorEnd, insertedText, deletedPreview)
	if err != nil {
		fail(w, err)
		return
	}
	encodeAnchors(sg)
	s.Hub.Publish(p.ID, Event{Type: "suggestions.changed", Payload: map[string]string{"fileId": f.ID}})
	writeJSON(w, http.StatusCreated, sg)
}

// handleResolveSuggestion accepts or rejects a suggestion. Accepting applies
// the change through the CRDT server-side; concurrent accepts serialize on the
// open-status row guard.
func (s *Server) handleResolveSuggestion(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFrom(r.Context())
		sg, err := s.Store.SuggestionByID(r.Context(), chi.URLParam(r, "suggestionID"))
		if err != nil {
			fail(w, err)
			return
		}
		// Rejecting your own suggestion needs no special rights; anything else
		// requires editor access.
		minRole := "editor"
		if status == "rejected" && sg.AuthorID == u.ID {
			minRole = "suggester"
		}
		p, ok := s.projectAccess(w, r, sg.ProjectID, minRole)
		if !ok {
			return
		}
		if sg.Status != "open" {
			writeErr(w, http.StatusConflict, "suggestion already resolved")
			return
		}

		// Claim the row first so two concurrent accepts can't both apply.
		if err := s.Store.ResolveSuggestion(r.Context(), sg.ID, status, u.ID); err != nil {
			if err == store.ErrNotFound {
				writeErr(w, http.StatusConflict, "suggestion already resolved")
				return
			}
			fail(w, err)
			return
		}

		if status == "accepted" {
			if err := s.applySuggestion(r, sg); err != nil {
				// Roll the status back so the suggestion isn't lost.
				_, _ = s.Store.Pool.Exec(r.Context(), `
					UPDATE suggestions SET status='open', resolved_by=NULL, resolved_at=NULL
					WHERE id=$1`, sg.ID)
				fail(w, err)
				return
			}
		}
		s.Hub.Publish(p.ID, Event{Type: "suggestions.changed", Payload: map[string]string{"fileId": sg.FileID}})
		writeJSON(w, http.StatusOK, map[string]string{"status": status})
	}
}

// applySuggestion resolves the anchors to current offsets and applies the edit.
func (s *Server) applySuggestion(r *http.Request, sg *store.Suggestion) error {
	anchors := []string{base64.StdEncoding.EncodeToString(sg.AnchorStart)}
	if sg.AnchorEnd != nil {
		anchors = append(anchors, base64.StdEncoding.EncodeToString(sg.AnchorEnd))
	}
	positions, err := s.Collab.AbsPos(r.Context(), sg.FileID, anchors)
	if err != nil {
		return err
	}
	from := positions[0]
	to := from
	if len(positions) > 1 {
		to = positions[1]
	}
	if from < 0 || to < from {
		return fmt.Errorf("suggestion anchors no longer resolve (document changed too much)")
	}
	insert := ""
	if sg.InsertedText != nil {
		insert = *sg.InsertedText
	}
	return s.Collab.Edit(r.Context(), sg.FileID, collab.EditRequest{
		From: from, To: to, Insert: insert, Origin: "suggestion:" + sg.ID,
	})
}

// Comments

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	comments, err := s.Store.ListComments(r.Context(), p.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if comments == nil {
		comments = []*store.Comment{}
	}
	for _, c := range comments {
		encodeCommentAnchors(c)
	}
	writeJSON(w, http.StatusOK, comments)
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "suggester")
	if !ok {
		return
	}
	var req struct {
		FileID   *string `json:"fileId"`
		ParentID *string `json:"parentId"`
		Body     string  `json:"body"`
		From     *int    `json:"from"`
		To       *int    `json:"to"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Body == "" {
		writeErr(w, http.StatusBadRequest, "body required")
		return
	}
	var anchorStart, anchorEnd []byte
	if req.FileID != nil && req.From != nil && req.To != nil {
		f, err := s.Store.FileByID(r.Context(), *req.FileID)
		if err != nil || f.ProjectID != p.ID {
			writeErr(w, http.StatusBadRequest, "invalid fileId")
			return
		}
		sB64, eB64, err := s.Collab.RelPos(r.Context(), f.ID, *req.From, *req.To)
		if err != nil {
			fail(w, err)
			return
		}
		anchorStart, _ = base64.StdEncoding.DecodeString(sB64)
		anchorEnd, _ = base64.StdEncoding.DecodeString(eB64)
	}
	c, err := s.Store.CreateComment(r.Context(), p.ID, req.FileID, req.ParentID, u.ID, req.Body, anchorStart, anchorEnd)
	if err != nil {
		fail(w, err)
		return
	}
	encodeCommentAnchors(c)
	s.Hub.Publish(p.ID, Event{Type: "comments.changed"})
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) commentAccess(w http.ResponseWriter, r *http.Request, minRole string) (*store.Comment, bool) {
	c, err := s.Store.CommentByID(r.Context(), chi.URLParam(r, "commentID"))
	if err != nil {
		fail(w, err)
		return nil, false
	}
	if _, ok := s.projectAccess(w, r, c.ProjectID, minRole); !ok {
		return nil, false
	}
	return c, true
}

func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	c, ok := s.commentAccess(w, r, "suggester")
	if !ok {
		return
	}
	if c.AuthorID != u.ID {
		writeErr(w, http.StatusForbidden, "can only edit your own comments")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Store.UpdateCommentBody(r.Context(), c.ID, req.Body); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(c.ProjectID, Event{Type: "comments.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	c, ok := s.commentAccess(w, r, "suggester")
	if !ok {
		return
	}
	if c.AuthorID != u.ID {
		if _, ok := s.projectAccess(w, r, c.ProjectID, "editor"); !ok {
			return
		}
	}
	if err := s.Store.DeleteComment(r.Context(), c.ID); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(c.ProjectID, Event{Type: "comments.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleResolveComment(w http.ResponseWriter, r *http.Request) {
	c, ok := s.commentAccess(w, r, "suggester")
	if !ok {
		return
	}
	status := "resolved"
	if c.Status == "resolved" {
		status = "open"
	}
	if err := s.Store.SetCommentStatus(r.Context(), c.ID, status); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(c.ProjectID, Event{Type: "comments.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}
