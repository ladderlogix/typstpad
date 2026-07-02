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
}

func (s *Store) CreateCollection(ctx context.Context, ownerID, name string) (*Collection, error) {
	var c Collection
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO collections (owner_id, name) VALUES ($1,$2)
		RETURNING id, owner_id, name, created_at`, ownerID, name).
		Scan(&c.ID, &c.OwnerID, &c.Name, &c.CreatedAt)
	return &c, err
}

func (s *Store) ListCollections(ctx context.Context, ownerID string) ([]*Collection, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id, c.owner_id, c.name, c.created_at,
			(SELECT count(*) FROM project_collections pc
			 JOIN projects p ON p.id = pc.project_id
			 WHERE pc.collection_id = c.id AND p.deleted_at IS NULL)
		FROM collections c WHERE c.owner_id = $1 ORDER BY c.name`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.Name, &c.CreatedAt, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// CollectionOwnedBy returns the collection only if owned by userID.
func (s *Store) CollectionOwnedBy(ctx context.Context, id, ownerID string) (*Collection, error) {
	var c Collection
	err := s.Pool.QueryRow(ctx, `
		SELECT id, owner_id, name, created_at FROM collections WHERE id=$1 AND owner_id=$2`, id, ownerID).
		Scan(&c.ID, &c.OwnerID, &c.Name, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
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
		JOIN collections c ON c.id = pc.collection_id AND c.owner_id = $1
		WHERE pc.project_id = $2`, ownerID, projectID)
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
