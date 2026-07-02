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

type suggestionAnchors struct {
	start, end     []byte
	insertedText   *string
	deletedPreview *string
}

// computeSuggestionAnchors validates a suggestion range against the live doc
// and produces the Yjs anchors + display texts.
//
// `inline` marks suggest-mode insertions whose text is ALREADY in the shared
// document (typed live, rendered as a suggested range): those anchor a range
// and accepting them is a no-op while rejecting deletes the range. Dialog/API
// insertions anchor a point and hold the pending text in insertedText.
func (s *Server) computeSuggestionAnchors(r *http.Request, fileID, typ string, from, to int, text string, inline bool) (*suggestionAnchors, error, int) {
	if typ != "insert" && typ != "delete" && typ != "replace" {
		return nil, fmt.Errorf("type must be insert, delete or replace"), http.StatusBadRequest
	}
	if from < 0 || to < from {
		return nil, fmt.Errorf("invalid range"), http.StatusBadRequest
	}
	if typ == "insert" && !inline {
		to = from
	} else if typ != "insert" && to == from {
		return nil, fmt.Errorf("delete/replace requires a non-empty range"), http.StatusBadRequest
	}

	startB64, endB64, slice, err := s.Collab.RelPos(r.Context(), fileID, from, to)
	if err != nil {
		return nil, fmt.Errorf("range beyond end of document"), http.StatusBadRequest
	}

	out := &suggestionAnchors{}
	out.start, err = base64.StdEncoding.DecodeString(startB64)
	if err != nil {
		return nil, fmt.Errorf("bad anchor from sidecar: %w", err), http.StatusInternalServerError
	}
	needEnd := typ != "insert" || inline
	if needEnd {
		out.end, err = base64.StdEncoding.DecodeString(endB64)
		if err != nil {
			return nil, fmt.Errorf("bad anchor from sidecar: %w", err), http.StatusInternalServerError
		}
	}
	switch {
	case typ == "insert" && inline:
		out.insertedText = &slice // the typed range, authoritative from the doc
	case typ == "insert":
		out.insertedText = &text
	case typ == "delete":
		out.deletedPreview = &slice
	case typ == "replace":
		out.insertedText = &text
		out.deletedPreview = &slice
	}
	return out, nil, 0
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
		Type   string `json:"type"` // insert | delete | replace
		From   int    `json:"from"`
		To     int    `json:"to"`
		Text   string `json:"text"`
		Inline bool   `json:"inline"` // suggest-mode: text already typed into the doc
	}
	if !readJSON(w, r, &req) {
		return
	}
	// Inline insertions require write access to the doc (the text is already
	// there); read-only suggesters can only use the pending-text form.
	if req.Inline && !roleAtLeast(p.Role, "editor") {
		writeErr(w, http.StatusForbidden, "inline suggestions require editor access")
		return
	}
	anchors, err, status := s.computeSuggestionAnchors(r, f.ID, req.Type, req.From, req.To, req.Text, req.Inline)
	if err != nil {
		writeErr(w, status, err.Error())
		return
	}
	sg, err := s.Store.CreateSuggestion(r.Context(), p.ID, f.ID, u.ID, req.Type, anchors.start, anchors.end, anchors.insertedText, anchors.deletedPreview)
	if err != nil {
		fail(w, err)
		return
	}
	encodeAnchors(sg)
	s.Hub.Publish(p.ID, Event{Type: "suggestions.changed", Payload: map[string]string{"fileId": f.ID}})
	writeJSON(w, http.StatusCreated, sg)
}

// handleUpdateSuggestion re-anchors an open suggestion — inline suggest mode
// coalesces a burst of typing/deleting into one growing record.
func (s *Server) handleUpdateSuggestion(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	sg, err := s.Store.SuggestionByID(r.Context(), chi.URLParam(r, "suggestionID"))
	if err != nil {
		fail(w, err)
		return
	}
	if _, ok := s.projectAccess(w, r, sg.ProjectID, "suggester"); !ok {
		return
	}
	if sg.AuthorID != u.ID {
		writeErr(w, http.StatusForbidden, "can only update your own suggestions")
		return
	}
	if sg.Status != "open" {
		writeErr(w, http.StatusConflict, "suggestion already resolved")
		return
	}
	var req struct {
		From int    `json:"from"`
		To   int    `json:"to"`
		Text string `json:"text"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	inline := sg.Type == "insert" && sg.AnchorEnd != nil
	anchors, err, status := s.computeSuggestionAnchors(r, sg.FileID, sg.Type, req.From, req.To, req.Text, inline)
	if err != nil {
		writeErr(w, status, err.Error())
		return
	}
	if err := s.Store.UpdateSuggestionAnchors(r.Context(), sg.ID, anchors.start, anchors.end, anchors.insertedText, anchors.deletedPreview); err != nil {
		fail(w, err)
		return
	}
	updated, err := s.Store.SuggestionByID(r.Context(), sg.ID)
	if err != nil {
		fail(w, err)
		return
	}
	encodeAnchors(updated)
	s.Hub.Publish(sg.ProjectID, Event{Type: "suggestions.changed", Payload: map[string]string{"fileId": sg.FileID}})
	writeJSON(w, http.StatusOK, updated)
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

		// Inline insertions (suggest-mode typing) already live in the doc:
		// accepting keeps them as-is, rejecting removes the range. Everything
		// else applies its pending edit on accept only.
		inline := sg.Type == "insert" && sg.AnchorEnd != nil
		var applyErr error
		if status == "accepted" && !inline {
			applyErr = s.applySuggestion(r, sg, false)
		} else if status == "rejected" && inline {
			applyErr = s.applySuggestion(r, sg, true)
		}
		if applyErr != nil {
			// Roll the status back so the suggestion isn't lost.
			_, _ = s.Store.Pool.Exec(r.Context(), `
				UPDATE suggestions SET status='open', resolved_by=NULL, resolved_at=NULL
				WHERE id=$1`, sg.ID)
			fail(w, applyErr)
			return
		}
		s.Hub.Publish(p.ID, Event{Type: "suggestions.changed", Payload: map[string]string{"fileId": sg.FileID}})
		writeJSON(w, http.StatusOK, map[string]string{"status": status})
	}
}

// applySuggestion resolves the anchors to current offsets and applies the
// edit. With deleteOnly it removes the anchored range (rejecting an inline
// insertion) instead of applying the pending text.
func (s *Server) applySuggestion(r *http.Request, sg *store.Suggestion, deleteOnly bool) error {
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
	if !deleteOnly && sg.InsertedText != nil {
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
		sB64, eB64, _, err := s.Collab.RelPos(r.Context(), f.ID, *req.From, *req.To)
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
	s.notifyComment(r.Context(), u, p, req.Body)
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
