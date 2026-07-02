package api

import (
	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
)

// mountAuthedRoutes wires every route that requires an authenticated user.
func (s *Server) mountAuthedRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireUser)

		// Projects
		r.Get("/projects", s.handleListProjects)
		r.With(auth.RequireScope("write")).Post("/projects", s.handleCreateProject)
		r.Get("/search", s.handleSearch)
		r.Route("/projects/{projectID}", func(r chi.Router) {
			r.Get("/", s.handleGetProject)
			r.With(auth.RequireScope("write")).Patch("/", s.handleUpdateProject)
			r.With(auth.RequireScope("write")).Delete("/", s.handleDeleteProject)
			r.With(auth.RequireScope("write")).Post("/duplicate", s.handleDuplicateProject)
			r.Get("/events", s.handleProjectEvents)

			// Files
			r.Get("/files", s.handleListFiles)
			r.With(auth.RequireScope("write")).Post("/files", s.handleCreateFile)

			// Versions
			r.Get("/versions", s.handleListVersions)
			r.With(auth.RequireScope("write")).Post("/versions", s.handleCreateVersion)
			r.Get("/diff", s.handleDiff)

			// Comments
			r.Get("/comments", s.handleListComments)
			r.With(auth.RequireScope("write")).Post("/comments", s.handleCreateComment)

			// Sharing
			r.Get("/members", s.handleListMembers)
			r.With(auth.RequireScope("write")).Post("/members", s.handleAddMember)
			r.With(auth.RequireScope("write")).Patch("/members/{userID}", s.handleUpdateMember)
			r.With(auth.RequireScope("write")).Delete("/members/{userID}", s.handleRemoveMember)
			r.Get("/links", s.handleListLinks)
			r.With(auth.RequireScope("write")).Post("/links", s.handleCreateLink)

			// Compile / export
			r.With(auth.RequireScope("compile")).Post("/compile", s.handleCompile)
			r.Get("/export/pdf", s.handleExportPDF)

			// Templates
			r.With(auth.RequireScope("write")).Post("/publish-template", s.handlePublishTemplate)
		})

		r.Route("/files/{fileID}", func(r chi.Router) {
			r.Get("/", s.handleGetFile)
			r.Get("/content", s.handleFileContent)
			r.With(auth.RequireScope("write")).Patch("/", s.handleRenameFile)
			r.With(auth.RequireScope("write")).Delete("/", s.handleDeleteFile)
			r.Post("/collab-token", s.handleCollabToken)
			r.Get("/suggestions", s.handleListSuggestions)
			r.With(auth.RequireScope("write")).Post("/suggestions", s.handleCreateSuggestion)
			r.With(auth.RequireScope("write")).Post("/edit", s.handleApplyEdit)
		})

		r.With(auth.RequireScope("write")).Post("/suggestions/{suggestionID}/accept", s.handleResolveSuggestion("accepted"))
		r.With(auth.RequireScope("write")).Post("/suggestions/{suggestionID}/reject", s.handleResolveSuggestion("rejected"))

		r.With(auth.RequireScope("write")).Patch("/comments/{commentID}", s.handleUpdateComment)
		r.With(auth.RequireScope("write")).Delete("/comments/{commentID}", s.handleDeleteComment)
		r.With(auth.RequireScope("write")).Post("/comments/{commentID}/resolve", s.handleResolveComment)

		r.With(auth.RequireScope("write")).Post("/versions/{versionID}/restore", s.handleRestoreVersion)
		r.Get("/versions/{versionID}", s.handleGetVersion)

		r.Get("/templates", s.handleListTemplates)
		r.With(auth.RequireScope("write")).Post("/templates/{templateID}/use", s.handleUseTemplate)

		r.With(auth.RequireScope("write")).Post("/links/{token}/join", s.handleJoinLink)
		r.With(auth.RequireScope("write")).Delete("/links/{linkID}", s.handleRevokeLink)

		// PATs (session-only: a PAT must not mint more PATs)
		r.Get("/tokens", s.handleListTokens)
		r.Post("/tokens", s.handleCreateToken)
		r.Delete("/tokens/{tokenID}", s.handleDeleteToken)

		// Admin
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAdmin)
			r.Get("/admin/users", s.handleAdminListUsers)
			r.Patch("/admin/users/{userID}", s.handleAdminUpdateUser)
			r.Delete("/admin/users/{userID}", s.handleAdminDeleteUser)
			r.Get("/admin/settings", s.handleAdminGetSettings)
			r.Put("/admin/settings", s.handleAdminPutSettings)
		})
	})

	// Asset bytes are fetched with <img>/worker requests (no custom headers),
	// so they authenticate via session cookie inside the handler.
	r.Get("/assets/{fileID}", s.handleAssetBytes)

	// Typst Universe package proxy for the in-browser compiler.
	r.Get("/typst/packages/*", s.handlePackageProxy)
}
