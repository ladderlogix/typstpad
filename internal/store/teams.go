package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Team struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	// Role is the requesting user's role in the team ("" if not a member).
	Role        string `json:"role,omitempty"`
	MemberCount int    `json:"memberCount,omitempty"`
}

// CreateTeam creates a team and makes the creator its admin.
func (s *Store) CreateTeam(ctx context.Context, name, creatorID string) (*Team, error) {
	var t Team
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO teams (name, created_by) VALUES ($1,$2)
			RETURNING id, name, created_by, created_at`, name, creatorID).
			Scan(&t.ID, &t.Name, &t.CreatedBy, &t.CreatedAt); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO team_members (team_id, user_id, role) VALUES ($1,$2,'admin')`, t.ID, creatorID)
		return err
	})
	if err != nil {
		return nil, err
	}
	t.Role = "admin"
	t.MemberCount = 1
	return &t, nil
}

// TeamForUser returns the team with the caller's role, or ErrNotFound if the
// caller is not a member.
func (s *Store) TeamForUser(ctx context.Context, teamID, userID string) (*Team, error) {
	var t Team
	err := s.Pool.QueryRow(ctx, `
		SELECT t.id, t.name, t.created_by, t.created_at, tm.role,
			(SELECT count(*) FROM team_members WHERE team_id = t.id)
		FROM teams t
		JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = $2
		WHERE t.id = $1`, teamID, userID).
		Scan(&t.ID, &t.Name, &t.CreatedBy, &t.CreatedAt, &t.Role, &t.MemberCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListTeamsForUser(ctx context.Context, userID string) ([]*Team, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.name, t.created_by, t.created_at, tm.role,
			(SELECT count(*) FROM team_members WHERE team_id = t.id)
		FROM teams t
		JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = $1
		ORDER BY t.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedBy, &t.CreatedAt, &t.Role, &t.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (s *Store) RenameTeam(ctx context.Context, teamID, name string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE teams SET name=$2 WHERE id=$1`, teamID, name)
	return err
}

func (s *Store) DeleteTeam(ctx context.Context, teamID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, teamID)
	return err
}

// Team members

func (s *Store) ListTeamMembers(ctx context.Context, teamID string) ([]*Member, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT tm.user_id, u.email, u.name, u.color, tm.role
		FROM team_members tm JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = $1 ORDER BY tm.created_at`, teamID)
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

func (s *Store) UpsertTeamMember(ctx context.Context, teamID, userID, role string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO team_members (team_id, user_id, role) VALUES ($1,$2,$3)
		ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role`, teamID, userID, role)
	return err
}

func (s *Store) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM team_members WHERE team_id=$1 AND user_id=$2`, teamID, userID)
	return err
}

// CountTeamAdmins guards against removing/demoting the last admin.
func (s *Store) CountTeamAdmins(ctx context.Context, teamID string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM team_members WHERE team_id=$1 AND role='admin'`, teamID).Scan(&n)
	return n, err
}

// Project ↔ team shares

type ProjectTeam struct {
	TeamID   string `json:"teamId"`
	TeamName string `json:"teamName"`
	Role     string `json:"role"`
}

func (s *Store) ListProjectTeams(ctx context.Context, projectID string) ([]*ProjectTeam, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT pt.team_id, t.name, pt.role
		FROM project_teams pt JOIN teams t ON t.id = pt.team_id
		WHERE pt.project_id = $1 ORDER BY t.name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ProjectTeam
	for rows.Next() {
		var pt ProjectTeam
		if err := rows.Scan(&pt.TeamID, &pt.TeamName, &pt.Role); err != nil {
			return nil, err
		}
		out = append(out, &pt)
	}
	return out, rows.Err()
}

func (s *Store) UpsertProjectTeam(ctx context.Context, projectID, teamID, role string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO project_teams (project_id, team_id, role) VALUES ($1,$2,$3)
		ON CONFLICT (project_id, team_id) DO UPDATE SET role = EXCLUDED.role`, projectID, teamID, role)
	return err
}

func (s *Store) RemoveProjectTeam(ctx context.Context, projectID, teamID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM project_teams WHERE project_id=$1 AND team_id=$2`, projectID, teamID)
	return err
}

// IsTeamMember reports whether a user belongs to a team.
func (s *Store) IsTeamMember(ctx context.Context, teamID, userID string) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id=$1 AND user_id=$2)`, teamID, userID).Scan(&exists)
	return exists, err
}
