package api

import (
	"net/http"
	"strconv"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

// audit records a security-relevant action, attributed to the current user.
// Best-effort: failures are swallowed so they never block the primary action.
func (s *Server) audit(r *http.Request, action, target, detail string) {
	u := auth.UserFrom(r.Context())
	var id, email string
	if u != nil {
		id, email = u.ID, u.Email
	}
	_ = s.Store.RecordAudit(r.Context(), id, email, action, target, detail)
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	entries, err := s.Store.ListAudit(r.Context(), r.URL.Query().Get("action"), limit)
	if err != nil {
		fail(w, err)
		return
	}
	if entries == nil {
		entries = []*store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
