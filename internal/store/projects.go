package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Project struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	OwnerID      string          `json:"ownerId"`
	MainPath     string          `json:"mainPath"`
	IsTemplate   bool            `json:"isTemplate"`
	TemplateMeta json.RawMessage `json:"templateMeta,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
	// Role is the requesting user's role, filled in by list/get queries.
	Role string `json:"role,omitempty"`
}

const projectCols = `p.id, p.name, p.description, p.owner_id, p.main_path, p.is_template, p.template_meta, p.created_at, p.updated_at`

func scanProject(row pgx.Row, withRole bool) (*Project, error) {
	var p Project
	dest := []any{&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.MainPath, &p.IsTemplate, &p.TemplateMeta, &p.CreatedAt, &p.UpdatedAt}
	if withRole {
		dest = append(dest, &p.Role)
	}
	err := row.Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateProject(ctx context.Context, name, description, ownerID string) (*Project, error) {
	var p *Project
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		p, err = scanProject(tx.QueryRow(ctx, `
			INSERT INTO projects (name, description, owner_id) VALUES ($1,$2,$3)
			RETURNING `+projectCols, name, description, ownerID), false)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,'owner')`, p.ID, ownerID)
		return err
	})
	if err != nil {
		return nil, err
	}
	p.Role = "owner"
	return p, nil
}

// ProjectForUser returns the project and the user's role ("" if no access).
func (s *Store) ProjectForUser(ctx context.Context, projectID, userID string) (*Project, error) {
	return scanProject(s.Pool.QueryRow(ctx, `
		SELECT `+projectCols+`, m.role FROM projects p
		JOIN project_members m ON m.project_id = p.id AND m.user_id = $2
		WHERE p.id = $1 AND p.deleted_at IS NULL`, projectID, userID), true)
}

func (s *Store) ProjectByID(ctx context.Context, projectID string) (*Project, error) {
	return scanProject(s.Pool.QueryRow(ctx, `
		SELECT `+projectCols+` FROM projects p WHERE p.id = $1 AND p.deleted_at IS NULL`, projectID), false)
}

func (s *Store) ListProjectsForUser(ctx context.Context, userID string, query string) ([]*Project, error) {
	sql := `
		SELECT ` + projectCols + `, m.role FROM projects p
		JOIN project_members m ON m.project_id = p.id AND m.user_id = $1
		WHERE p.deleted_at IS NULL AND p.is_template = false`
	args := []any{userID}
	if query != "" {
		sql += ` AND p.name ILIKE '%' || $2 || '%'`
		args = append(args, query)
	}
	sql += ` ORDER BY p.updated_at DESC`
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ListTemplates(ctx context.Context) ([]*Project, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+projectCols+` FROM projects p
		WHERE p.deleted_at IS NULL AND p.is_template = true ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProject(ctx context.Context, id string, name, description, mainPath *string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE projects SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			main_path = COALESCE($4, main_path),
			updated_at = now()
		WHERE id = $1`, id, name, description, mainPath)
	return err
}

func (s *Store) TouchProject(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE projects SET updated_at = now() WHERE id = $1`, id)
	return err
}

func (s *Store) SoftDeleteProject(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE projects SET deleted_at = now() WHERE id = $1`, id)
	return err
}

func (s *Store) SetProjectTemplate(ctx context.Context, id string, isTemplate bool, meta json.RawMessage) error {
	_, err := s.Pool.Exec(ctx, `UPDATE projects SET is_template=$2, template_meta=$3 WHERE id=$1`, id, isTemplate, meta)
	return err
}

// Members

type Member struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	Role   string `json:"role"`
}

func (s *Store) ListMembers(ctx context.Context, projectID string) ([]*Member, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT m.user_id, u.email, u.name, u.color, m.role
		FROM project_members m JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1 ORDER BY m.created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Color, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (s *Store) UpsertMember(ctx context.Context, projectID, userID, role string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role`, projectID, userID, role)
	return err
}

// AddMemberIfAbsent adds a member but never downgrades/changes an existing role.
func (s *Store) AddMemberIfAbsent(ctx context.Context, projectID, userID, role string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)
		ON CONFLICT (project_id, user_id) DO NOTHING`, projectID, userID, role)
	return err
}

func (s *Store) RemoveMember(ctx context.Context, projectID, userID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, userID)
	return err
}

// Share links

type ShareLink struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"projectId"`
	Role      string     `json:"role"`
	CreatedBy string     `json:"createdBy"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	// Token is only set on creation.
	Token string `json:"token,omitempty"`
}

func (s *Store) CreateShareLink(ctx context.Context, projectID string, tokenHash []byte, role, createdBy string, expiresAt *time.Time) (*ShareLink, error) {
	var l ShareLink
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO share_links (project_id, token_hash, role, created_by, expires_at)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, project_id, role, created_by, expires_at, created_at`,
		projectID, tokenHash, role, createdBy, expiresAt).
		Scan(&l.ID, &l.ProjectID, &l.Role, &l.CreatedBy, &l.ExpiresAt, &l.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Store) ListShareLinks(ctx context.Context, projectID string) ([]*ShareLink, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, project_id, role, created_by, expires_at, created_at
		FROM share_links WHERE project_id=$1 AND revoked_at IS NULL ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ShareLink
	for rows.Next() {
		var l ShareLink
		if err := rows.Scan(&l.ID, &l.ProjectID, &l.Role, &l.CreatedBy, &l.ExpiresAt, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *Store) ShareLinkByTokenHash(ctx context.Context, tokenHash []byte) (*ShareLink, error) {
	var l ShareLink
	err := s.Pool.QueryRow(ctx, `
		SELECT id, project_id, role, created_by, expires_at, created_at
		FROM share_links
		WHERE token_hash=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`,
		tokenHash).Scan(&l.ID, &l.ProjectID, &l.Role, &l.CreatedBy, &l.ExpiresAt, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Store) RevokeShareLink(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE share_links SET revoked_at=now() WHERE id=$1`, id)
	return err
}
