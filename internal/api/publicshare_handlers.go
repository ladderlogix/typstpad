package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

// publicShareView is what the owner sees: whether the link is on and, if so,
// the shareable URL.
type publicShareView struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token,omitempty"`
	URL     string `json:"url,omitempty"`
}

func (s *Server) publicShareURL(token string) string {
	return strings.TrimRight(s.Cfg.PublicURL, "/") + "/share/" + token
}

func (s *Server) handleGetPublicShare(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	ps, err := s.Store.GetPublicShare(r.Context(), p.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, publicShareView{Enabled: false})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicShareView{Enabled: true, Token: ps.Token, URL: s.publicShareURL(ps.Token)})
}

func (s *Server) handleEnablePublicShare(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	token, _, err := auth.NewToken("pub_")
	if err != nil {
		fail(w, err)
		return
	}
	ps, err := s.Store.EnablePublicShare(r.Context(), p.ID, token, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicShareView{Enabled: true, Token: ps.Token, URL: s.publicShareURL(ps.Token)})
}

func (s *Server) handleDisablePublicShare(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	if err := s.Store.DisablePublicShare(r.Context(), p.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicShareView{Enabled: false})
}

// ---- anonymous (no-auth) endpoints ----

func (s *Server) publicProject(w http.ResponseWriter, r *http.Request) (*store.Project, bool) {
	p, err := s.Store.ProjectByPublicToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "this link is invalid or has been turned off")
		return nil, false
	}
	return p, true
}

// handlePublicMeta returns just enough for the share page to render a title.
func (s *Server) handlePublicMeta(w http.ResponseWriter, r *http.Request) {
	p, ok := s.publicProject(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": p.Name})
}

// handlePublicPDF server-compiles the shared project and streams the PDF inline.
func (s *Server) handlePublicPDF(w http.ResponseWriter, r *http.Request) {
	p, ok := s.publicProject(w, r)
	if !ok {
		return
	}
	res := s.runCompile(w, r, p)
	if res == nil {
		return
	}
	if !res.OK {
		writeErr(w, http.StatusUnprocessableEntity, "the document has compile errors and can't be previewed")
		return
	}
	name := strings.ReplaceAll(p.Name, `"`, "") + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(res.PDF)
}
