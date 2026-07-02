package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Collection struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	Count     int       `json:"count"`
	TeamID    *string   `json:"teamId,omitempty"`
	TeamName  string    `json:"teamName,omitempty"`
	CanManage bool      `json:"canManage"`
}

func (s *Store) CreateCollection(ctx context.Context, ownerID, name string, teamID *string) (*Collection, error) {
	var c Collection
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO collections (owner_id, name, team_id) VALUES ($1,$2,NULLIF($3,'')::uuid)
		RETURNING id, owner_id, name, created_at, team_id`, ownerID, name, ptrStr(teamID)).
		Scan(&c.ID, &c.OwnerID, &c.Name, &c.CreatedAt, &c.TeamID)
	c.CanManage = true
	return &c, err
}

// ListCollections returns the user's personal collections plus every collection
// belonging to a team they're a member of.
func (s *Store) ListCollections(ctx context.Context, userID string) ([]*Collection, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id, c.owner_id, c.name, c.created_at, c.team_id, COALESCE(t.name,''),
			(SELECT count(*) FROM project_collections pc
			 JOIN projects p ON p.id = pc.project_id
			 WHERE pc.collection_id = c.id AND p.deleted_at IS NULL),
			(c.owner_id = $1 OR tm.role = 'admin') AS can_manage
		FROM collections c
		LEFT JOIN teams t ON t.id = c.team_id
		LEFT JOIN team_members tm ON tm.team_id = c.team_id AND tm.user_id = $1
		WHERE (c.team_id IS NULL AND c.owner_id = $1)
		   OR (c.team_id IS NOT NULL AND tm.user_id IS NOT NULL)
		ORDER BY c.team_id NULLS FIRST, c.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.Name, &c.CreatedAt, &c.TeamID, &c.TeamName, &c.Count, &c.CanManage); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// CollectionForUser returns a collection the user can access (their own, or a
// team's they belong to). CanManage is set for the personal owner or a team admin.
func (s *Store) CollectionForUser(ctx context.Context, id, userID string) (*Collection, error) {
	var c Collection
	err := s.Pool.QueryRow(ctx, `
		SELECT c.id, c.owner_id, c.name, c.created_at, c.team_id,
			(c.owner_id = $2 OR tm.role = 'admin') AS can_manage
		FROM collections c
		LEFT JOIN team_members tm ON tm.team_id = c.team_id AND tm.user_id = $2
		WHERE c.id = $1
		  AND ((c.team_id IS NULL AND c.owner_id = $2)
		    OR (c.team_id IS NOT NULL AND tm.user_id IS NOT NULL))`, id, userID).
		Scan(&c.ID, &c.OwnerID, &c.Name, &c.CreatedAt, &c.TeamID, &c.CanManage)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Store) RenameCollection(ctx context.Context, id, name string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE collections SET name=$2 WHERE id=$1`, id, name)
	return err
}

func (s *Store) DeleteCollection(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM collections WHERE id=$1`, id)
	return err
}

func (s *Store) AddProjectToCollection(ctx context.Context, collectionID, projectID string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO project_collections (collection_id, project_id) VALUES ($1,$2)
		ON CONFLICT DO NOTHING`, collectionID, projectID)
	return err
}

func (s *Store) RemoveProjectFromCollection(ctx context.Context, collectionID, projectID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM project_collections WHERE collection_id=$1 AND project_id=$2`, collectionID, projectID)
	return err
}

// CollectionIDsForProject lists which of the user's collections a project is in.
func (s *Store) CollectionIDsForProject(ctx context.Context, ownerID, projectID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT pc.collection_id FROM project_collections pc
		JOIN collections c ON c.id = pc.collection_id
		WHERE pc.project_id = $2
		  AND ((c.team_id IS NULL AND c.owner_id = $1)
		    OR c.team_id IN (SELECT team_id FROM team_members WHERE user_id = $1))`, ownerID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
