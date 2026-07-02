package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type PublicShare struct {
	ProjectID string    `json:"projectId"`
	Token     string    `json:"token"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

func scanPublicShare(row pgx.Row) (*PublicShare, error) {
	var ps PublicShare
	err := row.Scan(&ps.ProjectID, &ps.Token, &ps.CreatedBy, &ps.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ps, nil
}

// GetPublicShare returns the project's public link, or ErrNotFound if disabled.
func (s *Store) GetPublicShare(ctx context.Context, projectID string) (*PublicShare, error) {
	return scanPublicShare(s.Pool.QueryRow(ctx,
		`SELECT project_id, token, created_by, created_at FROM public_shares WHERE project_id=$1`, projectID))
}

// EnablePublicShare creates the link if absent and returns it; if one already
// exists it is returned unchanged (idempotent — the token stays stable).
func (s *Store) EnablePublicShare(ctx context.Context, projectID, token, createdBy string) (*PublicShare, error) {
	return scanPublicShare(s.Pool.QueryRow(ctx, `
		INSERT INTO public_shares (project_id, token, created_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id) DO UPDATE SET project_id = EXCLUDED.project_id
		RETURNING project_id, token, created_by, created_at`, projectID, token, createdBy))
}

func (s *Store) DisablePublicShare(ctx context.Context, projectID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM public_shares WHERE project_id=$1`, projectID)
	return err
}

// ProjectByPublicToken resolves a public token to its (non-deleted) project.
func (s *Store) ProjectByPublicToken(ctx context.Context, token string) (*Project, error) {
	return scanProject(s.Pool.QueryRow(ctx, `
		SELECT `+projectCols+` FROM projects p
		JOIN public_shares ps ON ps.project_id = p.id
		WHERE ps.token = $1 AND p.deleted_at IS NULL`, token), false)
}
