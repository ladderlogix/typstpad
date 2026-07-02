package store

import "context"

type Stats struct {
	Users          int `json:"users"`
	Projects       int `json:"projects"`
	Documents      int `json:"documents"` // text files
	Templates      int `json:"templates"`
	Teams          int `json:"teams"`
	ActiveSessions int `json:"activeSessions"`
}

func (s *Store) Stats(ctx context.Context) (*Stats, error) {
	var st Stats
	err := s.Pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM projects WHERE deleted_at IS NULL AND is_template = false),
			(SELECT count(*) FROM files WHERE kind = 'text'),
			(SELECT count(*) FROM projects WHERE deleted_at IS NULL AND is_template = true),
			(SELECT count(*) FROM teams),
			(SELECT count(*) FROM sessions WHERE expires_at > now())`).
		Scan(&st.Users, &st.Projects, &st.Documents, &st.Templates, &st.Teams, &st.ActiveSessions)
	return &st, err
}
