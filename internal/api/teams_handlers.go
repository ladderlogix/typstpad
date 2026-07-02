package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

// teamAccess loads a team and verifies the caller is a member with at least
// minRole ("member" or "admin"). Writes the HTTP error itself on failure.
func (s *Server) teamAccess(w http.ResponseWriter, r *http.Request, teamID, minRole string) (*store.Team, bool) {
	u := auth.UserFrom(r.Context())
	t, err := s.Store.TeamForUser(r.Context(), teamID, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	if err != nil {
		fail(w, err)
		return nil, false
	}
	if minRole == "admin" && t.Role != "admin" {
		writeErr(w, http.StatusForbidden, "requires team admin")
		return nil, false
	}
	return t, true
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	teams, err := s.Store.ListTeamsForUser(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if teams == nil {
		teams = []*store.Team{}
	}
	writeJSON(w, http.StatusOK, teams)
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	t, err := s.Store.CreateTeam(r.Context(), req.Name, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamAccess(w, r, chi.URLParam(r, "teamID"), "member")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleRenameTeam(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamAccess(w, r, chi.URLParam(r, "teamID"), "admin")
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if err := s.Store.RenameTeam(r.Context(), t.ID, strings.TrimSpace(req.Name)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamAccess(w, r, chi.URLParam(r, "teamID"), "admin")
	if !ok {
		return
	}
	if err := s.Store.DeleteTeam(r.Context(), t.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamAccess(w, r, chi.URLParam(r, "teamID"), "member")
	if !ok {
		return
	}
	members, err := s.Store.ListTeamMembers(r.Context(), t.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (s *Server) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamAccess(w, r, chi.URLParam(r, "teamID"), "admin")
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
	if req.Role != "admin" && req.Role != "member" {
		writeErr(w, http.StatusBadRequest, "role must be admin or member")
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
	if err := s.Store.UpsertTeamMember(r.Context(), t.ID, target.ID, req.Role); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleUpdateTeamMember(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamAccess(w, r, chi.URLParam(r, "teamID"), "admin")
	if !ok {
		return
	}
	targetID := chi.URLParam(r, "userID")
	var req struct {
		Role string `json:"role"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Role != "admin" && req.Role != "member" {
		writeErr(w, http.StatusBadRequest, "role must be admin or member")
		return
	}
	// Don't allow demoting the last admin.
	if req.Role == "member" {
		if admins, err := s.Store.CountTeamAdmins(r.Context(), t.ID); err != nil {
			fail(w, err)
			return
		} else if admins <= 1 {
			writeErr(w, http.StatusBadRequest, "a team must keep at least one admin")
			return
		}
	}
	if err := s.Store.UpsertTeamMember(r.Context(), t.ID, targetID, req.Role); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	targetID := chi.URLParam(r, "userID")
	// Members may remove themselves ("leave team"); otherwise admin only.
	minRole := "admin"
	if targetID == u.ID {
		minRole = "member"
	}
	t, ok := s.teamAccess(w, r, chi.URLParam(r, "teamID"), minRole)
	if !ok {
		return
	}
	// Don't let the last admin leave/remove themselves and orphan the team.
	members, err := s.Store.ListTeamMembers(r.Context(), t.ID)
	if err != nil {
		fail(w, err)
		return
	}
	admins := 0
	targetIsAdmin := false
	for _, m := range members {
		if m.Role == "admin" {
			admins++
			if m.UserID == targetID {
				targetIsAdmin = true
			}
		}
	}
	if targetIsAdmin && admins <= 1 {
		writeErr(w, http.StatusBadRequest, "the last admin cannot leave; delete the team or promote someone first")
		return
	}
	if err := s.Store.RemoveTeamMember(r.Context(), t.ID, targetID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Project ↔ team sharing

func (s *Server) handleListProjectTeams(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	shares, err := s.Store.ListProjectTeams(r.Context(), p.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if shares == nil {
		shares = []*store.ProjectTeam{}
	}
	writeJSON(w, http.StatusOK, shares)
}

func (s *Server) handleShareProjectWithTeam(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	var req struct {
		TeamID string `json:"teamId"`
		Role   string `json:"role"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !validMemberRole(req.Role) {
		writeErr(w, http.StatusBadRequest, "role must be editor, suggester or viewer")
		return
	}
	// You can only share with a team you belong to.
	member, err := s.Store.IsTeamMember(r.Context(), req.TeamID, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !member {
		writeErr(w, http.StatusForbidden, "you can only share with teams you belong to")
		return
	}
	if err := s.Store.UpsertProjectTeam(r.Context(), p.ID, req.TeamID, req.Role); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(p.ID, Event{Type: "members.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleUnshareProjectTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	if err := s.Store.RemoveProjectTeam(r.Context(), p.ID, chi.URLParam(r, "teamID")); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(p.ID, Event{Type: "members.changed"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
