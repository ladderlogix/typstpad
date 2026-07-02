package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

const defaultMainTyp = `#set page(paper: "a4", margin: 2.5cm)
#set text(size: 11pt)

= Untitled Document

Start writing here. Typst is a modern typesetting system —
see the #link("https://typst.app/docs/")[documentation] for syntax.
`

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	favoritesOnly := r.URL.Query().Get("favorites") == "true"
	projects, err := s.Store.ListProjectsForUser(r.Context(), u.ID, r.URL.Query().Get("q"), r.URL.Query().Get("collection"), favoritesOnly)
	if err != nil {
		fail(w, err)
		return
	}
	if projects == nil {
		projects = []*store.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	projects, err := s.Store.ListTrashedProjects(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if projects == nil {
		projects = []*store.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

// ownsTrashed authorizes an action on a soft-deleted project: only its owner
// (or an admin) may restore or permanently delete it.
func (s *Server) ownsTrashed(w http.ResponseWriter, r *http.Request, projectID string) bool {
	u := auth.UserFrom(r.Context())
	owner, ok := s.Store.TrashedProjectOwner(r.Context(), projectID)
	if !ok {
		writeErr(w, http.StatusNotFound, "not found in trash")
		return false
	}
	if owner != u.ID && !u.IsAdmin {
		writeErr(w, http.StatusForbidden, "only the owner can do that")
		return false
	}
	return true
}

func (s *Server) handleRestoreProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	if !s.ownsTrashed(w, r, id) {
		return
	}
	if err := s.Store.RestoreProject(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePermanentDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	if !s.ownsTrashed(w, r, id) {
		return
	}
	if err := s.Store.HardDeleteProject(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSetFavorite(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	on := r.Method == http.MethodPost
	if err := s.Store.SetFavorite(r.Context(), u.ID, p.ID, on); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"favorite": on})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		TemplateID  string `json:"templateId"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		req.Name = "Untitled project"
	}
	if !s.checkProjectQuota(w, r) {
		return
	}
	var p *store.Project
	var err error
	if req.TemplateID != "" {
		p, err = s.copyProject(r, req.TemplateID, req.Name, u.ID, true)
	} else {
		p, err = s.Store.CreateProject(r.Context(), req.Name, req.Description, u.ID)
		if err == nil {
			_, err = s.Store.CreateTextFile(r.Context(), p.ID, p.MainPath, defaultMainTyp)
		}
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// copyProject clones a project's current files into a new project owned by
// userID. fromTemplate requires the source be a template; otherwise the caller
// must already have viewer access to the source.
func (s *Server) copyProject(r *http.Request, sourceID, name, userID string, fromTemplate bool) (*store.Project, error) {
	ctx := r.Context()
	src, err := s.Store.ProjectByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if fromTemplate && !src.IsTemplate {
		return nil, store.ErrNotFound
	}
	p, err := s.Store.CreateProject(ctx, name, src.Description, userID)
	if err != nil {
		return nil, err
	}
	if src.MainPath != "main.typ" {
		mp := src.MainPath
		if err := s.Store.UpdateProject(ctx, p.ID, nil, nil, &mp); err != nil {
			return nil, err
		}
		p.MainPath = src.MainPath
	}
	files, err := s.Store.ListFiles(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		switch f.Kind {
		case "text":
			content, err := s.Store.FileContent(ctx, f.ID)
			if err != nil {
				return nil, err
			}
			if _, err := s.Store.CreateTextFile(ctx, p.ID, f.Path, content); err != nil {
				return nil, err
			}
		case "asset":
			if err := s.Store.UpsertBlob(ctx, f.BlobHash, f.Size); err != nil {
				return nil, err
			}
			if _, err := s.Store.CreateAssetFile(ctx, p.ID, f.Path, f.BlobHash, f.Mime, f.Size); err != nil {
				return nil, err
			}
		}
	}
	return p, nil
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "editor")
	if !ok {
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		MainPath    *string `json:"mainPath"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Store.UpdateProject(r.Context(), p.ID, req.Name, req.Description, req.MainPath); err != nil {
		fail(w, err)
		return
	}
	s.Hub.Publish(p.ID, Event{Type: "project.updated"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	if err := s.Store.SoftDeleteProject(r.Context(), p.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDuplicateProject(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	if !s.checkProjectQuota(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = p.Name + " (copy)"
	}
	dup, err := s.copyProject(r, p.ID, req.Name, u.ID, false)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dup)
}

func (s *Server) handleProjectEvents(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "viewer")
	if !ok {
		return
	}
	s.Hub.ServeSSE(w, r, p.ID)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	hits, err := s.Store.Search(r.Context(), u.ID, q, 50)
	if err != nil {
		fail(w, err)
		return
	}
	if hits == nil {
		hits = []*store.SearchHit{}
	}
	writeJSON(w, http.StatusOK, hits)
}

// Templates

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := s.Store.ListTemplates(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if templates == nil {
		templates = []*store.Project{}
	}
	writeJSON(w, http.StatusOK, templates)
}

func (s *Server) handleUseTemplate(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if !s.checkProjectQuota(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = "Untitled project"
	}
	p, err := s.copyProject(r, chi.URLParam(r, "templateID"), req.Name, u.ID, true)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handlePublishTemplate(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	p, ok := s.projectAccess(w, r, chi.URLParam(r, "projectID"), "owner")
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = p.Name
	}
	if req.Category == "" {
		req.Category = "Community"
	}
	// Publishing copies the project into a frozen template owned by the caller.
	tpl, err := s.copyProject(r, p.ID, req.Name, u.ID, false)
	if err != nil {
		fail(w, err)
		return
	}
	meta, _ := json.Marshal(map[string]string{
		"description": req.Description, "category": req.Category, "publishedFrom": p.ID,
	})
	if err := s.Store.SetProjectTemplate(r.Context(), tpl.ID, true, meta); err != nil {
		fail(w, err)
		return
	}
	tpl.IsTemplate = true
	writeJSON(w, http.StatusCreated, tpl)
}

// handleDeleteTemplate unpublishes/removes a template the caller owns (admins
// can remove any).
func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	tpl, err := s.Store.ProjectByID(r.Context(), chi.URLParam(r, "templateID"))
	if err != nil || !tpl.IsTemplate {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if tpl.OwnerID != u.ID && !u.IsAdmin {
		writeErr(w, http.StatusForbidden, "only the template owner or an admin can remove it")
		return
	}
	if err := s.Store.SoftDeleteProject(r.Context(), tpl.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
