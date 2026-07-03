package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ladderlogix/typstpad/internal/auth"
	"github.com/ladderlogix/typstpad/internal/store"
)

func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	cols, err := s.Store.ListCollections(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if cols == nil {
		cols = []*store.Collection{}
	}
	writeJSON(w, http.StatusOK, cols)
}

func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		Name   string `json:"name"`
		TeamID string `json:"teamId"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	var teamID *string
	if req.TeamID != "" {
		// Only members of the team may create a collection under it.
		if _, err := s.Store.TeamForUser(r.Context(), req.TeamID, u.ID); err != nil {
			writeErr(w, http.StatusForbidden, "you're not a member of that team")
			return
		}
		teamID = &req.TeamID
	}
	c, err := s.Store.CreateCollection(r.Context(), u.ID, req.Name, teamID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// collectionAccess loads a collection the caller can access (personal or a
// team's). If manage is true, it additionally requires manage rights (personal
// owner or team admin).
func (s *Server) collectionAccess(w http.ResponseWriter, r *http.Request, manage bool) (*store.Collection, bool) {
	u := auth.UserFrom(r.Context())
	c, err := s.Store.CollectionForUser(r.Context(), chi.URLParam(r, "collectionID"), u.ID)
	if err != nil {
		fail(w, err)
		return nil, false
	}
	if manage && !c.CanManage {
		writeErr(w, http.StatusForbidden, "only the owner or a team admin can do that")
		return nil, false
	}
	return c, true
}

func (s *Server) handleRenameCollection(w http.ResponseWriter, r *http.Request) {
	c, ok := s.collectionAccess(w, r, true)
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
	if err := s.Store.RenameCollection(r.Context(), c.ID, strings.TrimSpace(req.Name)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	c, ok := s.collectionAccess(w, r, true)
	if !ok {
		return
	}
	if err := s.Store.DeleteCollection(r.Context(), c.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSetProjectCollection adds/removes a project (the caller must be able to
// access it) to/from one of the caller's collections.
func (s *Server) handleAddProjectToCollection(w http.ResponseWriter, r *http.Request) {
	c, ok := s.collectionAccess(w, r, false)
	if !ok {
		return
	}
	var req struct {
		ProjectID string `json:"projectId"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	// Ensure the caller can see the project before filing it.
	if _, ok := s.projectAccess(w, r, req.ProjectID, "viewer"); !ok {
		return
	}
	if err := s.Store.AddProjectToCollection(r.Context(), c.ID, req.ProjectID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRemoveProjectFromCollection(w http.ResponseWriter, r *http.Request) {
	c, ok := s.collectionAccess(w, r, false)
	if !ok {
		return
	}
	if err := s.Store.RemoveProjectFromCollection(r.Context(), c.ID, chi.URLParam(r, "projectID")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleProjectCollections returns the collection ids a project belongs to (for
// the caller), so the UI can show/toggle membership.
func (s *Server) handleProjectCollections(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if _, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer"); !ok {
		return
	}
	ids, err := s.Store.CollectionIDsForProject(r.Context(), u.ID, chi.URLParam(r, "projectID"))
	if err != nil {
		fail(w, err)
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, ids)
}
