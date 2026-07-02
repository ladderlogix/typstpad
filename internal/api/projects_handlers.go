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
	projects, err := s.Store.ListProjectsForUser(r.Context(), u.ID, r.URL.Query().Get("q"))
	if err != nil {
		fail(w, err)
		return
	}
	if projects == nil {
		projects = []*store.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
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
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = p.Name
	}
	// Publishing copies the project into a frozen template owned by the caller.
	tpl, err := s.copyProject(r, p.ID, req.Name, u.ID, false)
	if err != nil {
		fail(w, err)
		return
	}
	meta, _ := json.Marshal(map[string]string{"description": req.Description, "publishedFrom": p.ID})
	if err := s.Store.SetProjectTemplate(r.Context(), tpl.ID, true, meta); err != nil {
		fail(w, err)
		return
	}
	tpl.IsTemplate = true
	writeJSON(w, http.StatusCreated, tpl)
}
