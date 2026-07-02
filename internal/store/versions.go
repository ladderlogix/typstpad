package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Snapshot struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"projectId"`
	Kind        string    `json:"kind"` // auto | named | pre_restore
	Name        *string   `json:"name,omitempty"`
	CreatedBy   *string   `json:"createdBy,omitempty"`
	ProjectHash []byte    `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
}

type SnapshotFile struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	ContentHash []byte `json:"-"`
}

func (s *Store) LatestSnapshotHash(ctx context.Context, projectID string) ([]byte, error) {
	var hash []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT project_hash FROM snapshots WHERE project_id=$1
		ORDER BY created_at DESC LIMIT 1`, projectID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return hash, err
}

// CreateSnapshot records a snapshot with its file manifest. Blob rows for the
// content hashes must already exist (see blob.Store.PutBytes).
func (s *Store) CreateSnapshot(ctx context.Context, projectID, kind string, name, createdBy *string, projectHash []byte, files []SnapshotFile) (*Snapshot, error) {
	var snap Snapshot
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO snapshots (project_id, kind, name, created_by, project_hash)
			VALUES ($1,$2,$3,$4,$5)
			RETURNING id, project_id, kind, name, created_by, project_hash, created_at`,
			projectID, kind, name, createdBy, projectHash).
			Scan(&snap.ID, &snap.ProjectID, &snap.Kind, &snap.Name, &snap.CreatedBy, &snap.ProjectHash, &snap.CreatedAt)
		if err != nil {
			return err
		}
		for _, f := range files {
			if _, err := tx.Exec(ctx, `
				INSERT INTO snapshot_files (snapshot_id, path, kind, content_hash) VALUES ($1,$2,$3,$4)`,
				snap.ID, f.Path, f.Kind, f.ContentHash); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) ListSnapshots(ctx context.Context, projectID string, limit int) ([]*Snapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, project_id, kind, name, created_by, project_hash, created_at
		FROM snapshots WHERE project_id=$1 ORDER BY created_at DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Snapshot
	for rows.Next() {
		var snap Snapshot
		if err := rows.Scan(&snap.ID, &snap.ProjectID, &snap.Kind, &snap.Name, &snap.CreatedBy, &snap.ProjectHash, &snap.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &snap)
	}
	return out, rows.Err()
}

func (s *Store) SnapshotByID(ctx context.Context, id string) (*Snapshot, error) {
	var snap Snapshot
	err := s.Pool.QueryRow(ctx, `
		SELECT id, project_id, kind, name, created_by, project_hash, created_at
		FROM snapshots WHERE id=$1`, id).
		Scan(&snap.ID, &snap.ProjectID, &snap.Kind, &snap.Name, &snap.CreatedBy, &snap.ProjectHash, &snap.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) SnapshotFiles(ctx context.Context, snapshotID string) ([]SnapshotFile, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT path, kind, content_hash FROM snapshot_files WHERE snapshot_id=$1 ORDER BY path`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SnapshotFile
	for rows.Next() {
		var f SnapshotFile
		if err := rows.Scan(&f.Path, &f.Kind, &f.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
