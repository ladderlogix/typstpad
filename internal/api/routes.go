package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/ratelimit"
)

// userKey rate-limits by the authenticated user id (falling back to IP).
func userKey(r *http.Request) string {
	if u := auth.UserFrom(r.Context()); u != nil {
		return u.ID
	}
	return ratelimit.ClientIP(r)
}

// mountAuthedRoutes wires every route that requires an authenticated user.
func (s *Server) mountAuthedRoutes(r chi.Router) {
	// Server-side compile is expensive; cap per user (in addition to the global
	// NumCPU semaphore in the compiler).
	compileLimit := ratelimit.New(40, time.Minute).Middleware(userKey)

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireUser)

		// Projects
		r.Get("/projects", s.handleListProjects)
		r.With(auth.RequireScope("write")).Post("/projects", s.handleCreateProject)
		r.With(auth.RequireScope("write")).Post("/projects/import", s.handleImportZip)
		r.Get("/projects/trash", s.handleListTrash)
		r.Get("/search", s.handleSearch)
		r.Route("/projects/{projectID}", func(r chi.Router) {
			r.Get("/", s.handleGetProject)
			r.With(auth.RequireScope("write")).Patch("/", s.handleUpdateProject)
			r.With(auth.RequireScope("write")).Delete("/", s.handleDeleteProject)
			r.With(auth.RequireScope("write")).Post("/duplicate", s.handleDuplicateProject)
			r.With(auth.RequireScope("write")).Post("/restore", s.handleRestoreProject)
			r.With(auth.RequireScope("write")).Delete("/permanent", s.handlePermanentDeleteProject)
			r.With(auth.RequireScope("write")).Post("/favorite", s.handleSetFavorite)
			r.With(auth.RequireScope("write")).Delete("/favorite", s.handleSetFavorite)
			r.Get("/collections", s.handleProjectCollections)
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
			r.Get("/public-share", s.handleGetPublicShare)
			r.With(auth.RequireScope("write")).Post("/public-share", s.handleEnablePublicShare)
			r.With(auth.RequireScope("write")).Delete("/public-share", s.handleDisablePublicShare)
			r.Get("/teams", s.handleListProjectTeams)
			r.With(auth.RequireScope("write")).Post("/teams", s.handleShareProjectWithTeam)
			r.With(auth.RequireScope("write")).Delete("/teams/{teamID}", s.handleUnshareProjectTeam)

			// Compile / export
			r.With(auth.RequireScope("compile"), compileLimit).Post("/compile", s.handleCompile)
			r.With(compileLimit).Get("/export/pdf", s.handleExportPDF)
			r.Get("/export.zip", s.handleExportZip)

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
		r.With(auth.RequireScope("write")).Patch("/suggestions/{suggestionID}", s.handleUpdateSuggestion)

		r.With(auth.RequireScope("write")).Patch("/comments/{commentID}", s.handleUpdateComment)
		r.With(auth.RequireScope("write")).Delete("/comments/{commentID}", s.handleDeleteComment)
		r.With(auth.RequireScope("write")).Post("/comments/{commentID}/resolve", s.handleResolveComment)

		r.With(auth.RequireScope("write")).Post("/versions/{versionID}/restore", s.handleRestoreVersion)
		r.Get("/versions/{versionID}", s.handleGetVersion)

		r.Get("/templates", s.handleListTemplates)
		r.Get("/templates/{templateID}/thumbnail.png", s.handleTemplateThumbnail)
		r.With(auth.RequireScope("write")).Post("/templates/{templateID}/use", s.handleUseTemplate)
		r.With(auth.RequireScope("write")).Delete("/templates/{templateID}", s.handleDeleteTemplate)

		r.With(auth.RequireScope("write")).Post("/links/{token}/join", s.handleJoinLink)
		r.With(auth.RequireScope("write")).Delete("/links/{linkID}", s.handleRevokeLink)

		// Collections (personal project groups)
		r.Get("/collections", s.handleListCollections)
		r.With(auth.RequireScope("write")).Post("/collections", s.handleCreateCollection)
		r.Route("/collections/{collectionID}", func(r chi.Router) {
			r.With(auth.RequireScope("write")).Patch("/", s.handleRenameCollection)
			r.With(auth.RequireScope("write")).Delete("/", s.handleDeleteCollection)
			r.With(auth.RequireScope("write")).Post("/projects", s.handleAddProjectToCollection)
			r.With(auth.RequireScope("write")).Delete("/projects/{projectID}", s.handleRemoveProjectFromCollection)
		})

		// Teams
		r.Get("/teams", s.handleListTeams)
		r.With(auth.RequireScope("write")).Post("/teams", s.handleCreateTeam)
		r.Route("/teams/{teamID}", func(r chi.Router) {
			r.Get("/", s.handleGetTeam)
			r.With(auth.RequireScope("write")).Patch("/", s.handleRenameTeam)
			r.With(auth.RequireScope("write")).Delete("/", s.handleDeleteTeam)
			r.Get("/members", s.handleListTeamMembers)
			r.With(auth.RequireScope("write")).Post("/members", s.handleAddTeamMember)
			r.With(auth.RequireScope("write")).Patch("/members/{userID}", s.handleUpdateTeamMember)
			r.With(auth.RequireScope("write")).Delete("/members/{userID}", s.handleRemoveTeamMember)
		})

		// Notifications
		r.Get("/notifications", s.handleListNotifications)
		r.Get("/notifications/unread-count", s.handleUnreadCount)
		r.Post("/notifications/read", s.handleMarkAllRead)
		r.Post("/notifications/{id}/read", s.handleMarkRead)

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
			r.Get("/admin/stats", s.handleAdminStats)
			r.Get("/admin/audit", s.handleAdminAudit)
		})
	})

	// Asset bytes are fetched with <img>/worker requests (no custom headers),
	// so they authenticate via session cookie inside the handler.
	r.Get("/assets/{fileID}", s.handleAssetBytes)

	// Public read-only share: anonymous access to a project's compiled PDF.
	// The PDF triggers a server compile, so rate-limit it per IP.
	publicCompileLimit := ratelimit.New(30, time.Minute).Middleware(ratelimit.ClientIP)
	r.Get("/public/{token}", s.handlePublicMeta)
	r.With(publicCompileLimit).Get("/public/{token}/pdf", s.handlePublicPDF)

	// Typst Universe package + font proxies for the in-browser compiler
	// (disk-cached, so browsers never need direct internet access).
	r.Get("/typst/packages/*", s.handlePackageProxy)
	r.Get("/typst/fonts/{repo}/{file}", s.handleFontProxy)
}
