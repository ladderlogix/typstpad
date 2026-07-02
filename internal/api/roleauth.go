package api

import (
	"errors"
	"net/http"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

var roleRank = map[string]int{"viewer": 1, "suggester": 2, "editor": 3, "owner": 4}

func roleAtLeast(role, min string) bool {
	return roleRank[role] >= roleRank[min]
}

var errForbidden = errors.New("forbidden")

// projectAccess loads the project and verifies the current user has at least
// minRole. Writes the HTTP error response itself when returning an error.
func (s *Server) projectAccess(w http.ResponseWriter, r *http.Request, projectID, minRole string) (*store.Project, bool) {
	u := auth.UserFrom(r.Context())
	p, err := s.Store.ProjectForUser(r.Context(), projectID, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		// Hide existence from non-members.
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	if err != nil {
		fail(w, err)
		return nil, false
	}
	if !roleAtLeast(p.Role, minRole) {
		writeErr(w, http.StatusForbidden, "requires "+minRole+" access")
		return nil, false
	}
	return p, true
}

// fileAccess resolves a file and checks project access in one step.
func (s *Server) fileAccess(w http.ResponseWriter, r *http.Request, fileID, minRole string) (*store.File, *store.Project, bool) {
	f, err := s.Store.FileByID(r.Context(), fileID)
	if err != nil {
		fail(w, err)
		return nil, nil, false
	}
	p, ok := s.projectAccess(w, r, f.ProjectID, minRole)
	if !ok {
		return nil, nil, false
	}
	return f, p, true
}
