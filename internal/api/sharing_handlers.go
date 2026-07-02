package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

func validMemberRole(role string) bool {
	return role == "editor" || role == "suggester" || role == "viewer"
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	members, err := s.Store.ListMembers(r.Context(), p.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !validMemberRole(req.Role) {
		writeErr(w, http.StatusBadRequest, "role must be editor, suggester or viewer")
		return
	}
	target, err := s.Store.UserByEmail(r.Context(), strings.TrimSpace(strings.ToLower(req.Email)))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "no user with that email")
			return
		}
		fail(w, err)
		return
	}
	if target.ID == p.OwnerID {
		writeErr(w, http.StatusBadRequest, "cannot change the owner's role")
		return
	}
	if err := s.Store.UpsertMember(r.Context(), p.ID, target.ID, req.Role); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(p.ID, Event{Type: "members.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	userID := chi.URLParam(r, "userID")
	if userID == p.OwnerID {
		writeErr(w, http.StatusBadRequest, "cannot change the owner's role")
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !validMemberRole(req.Role) {
		writeErr(w, http.StatusBadRequest, "invalid role")
		return
	}
	if err := s.Store.UpsertMember(r.Context(), p.ID, userID, req.Role); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(p.ID, Event{Type: "members.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	userID := chi.URLParam(r, "userID")
	// Members may remove themselves ("leave project"); otherwise owner only.
	minRole := "owner"
	if userID == u.ID {
		minRole = "viewer"
	}
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), minRole)
	if !ok {
		return
	}
	if userID == p.OwnerID {
		writeErr(w, http.StatusBadRequest, "the owner cannot be removed")
		return
	}
	if err := s.Store.RemoveMember(r.Context(), p.ID, userID); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(p.ID, Event{Type: "members.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Share links

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "editor")
	if !ok {
		return
	}
	links, err := s.Store.ListShareLinks(r.Context(), p.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if links == nil {
		links = []*store.ShareLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	var req struct {
		Role      string     `json:"role"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !validMemberRole(req.Role) {
		writeErr(w, http.StatusBadRequest, "role must be editor, suggester or viewer")
		return
	}
	token, hash, err := auth.NewToken("share_")
	if err != nil {
		fail(w, err)
		return
	}
	link, err := s.Store.CreateShareLink(r.Context(), p.ID, hash, req.Role, u.ID, req.ExpiresAt)
	if err != nil {
		fail(w, err)
		return
	}
	link.Token = token // returned once; only the hash is stored
	writeJSON(w, http.StatusCreated, link)
}

func (s *Server) handleRevokeLink(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkID")
	var projectID string
	err := s.Store.Pool.QueryRow(r.Context(), `SELECT project_id FROM share_links WHERE id=$1`, linkID).Scan(&projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := s.projectAccess(w, r, projectID, "owner"); !ok {
		return
	}
	if err := s.Store.RevokeShareLink(r.Context(), linkID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleJoinLink lets any authenticated user redeem a share-link token for
// project membership at the link's role.
func (s *Server) handleJoinLink(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	link, err := s.Store.ShareLinkByTokenHash(r.Context(), auth.HashToken(chi.URLParam(r, "token")))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "invalid or expired link")
			return
		}
		fail(w, err)
		return
	}
	if err := s.Store.AddMemberIfAbsent(r.Context(), link.ProjectID, u.ID, link.Role); err != nil {
		fail(w, err)
		return
	}
	p, err := s.Store.ProjectForUser(r.Context(), link.ProjectID, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(link.ProjectID, Event{Type: "members.changed"})
	writeJSON(w, http.StatusOK, p)
}
