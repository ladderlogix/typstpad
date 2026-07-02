package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"typstpad/internal/auth"
	"typstpad/internal/store"
)

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	items, err := s.Store.ListNotifications(r.Context(), u.ID, 50)
	if err != nil {
		fail(w, err)
		return
	}
	if items == nil {
		items = []*store.Notification{}
	}
	unread, err := s.Store.CountUnreadNotifications(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "unread": unread})
}

func (s *Server) handleUnreadCount(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	unread, err := s.Store.CountUnreadNotifications(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": unread})
}

func (s *Server) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if err := s.Store.MarkAllNotificationsRead(r.Context(), u.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if err := s.Store.MarkNotificationRead(r.Context(), u.ID, chi.URLParam(r, "id")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
