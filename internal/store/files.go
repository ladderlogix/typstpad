package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type File struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Path      string    `json:"path"`
	Kind      string    `json:"kind"` // text | asset
	BlobHash  []byte    `json:"-"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	Locked    bool      `json:"locked"`
	CreatedAt time.Time `json:"createdAt"`
}

const fileCols = `id, project_id, path, kind, blob_hash, mime, size, locked, created_at`

func scanFile(row pgx.Row) (*File, error) {
	var f File
	err := row.Scan(&f.ID, &f.ProjectID, &f.Path, &f.Kind, &f.BlobHash, &f.Mime, &f.Size, &f.Locked, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// CreateTextFile creates a text file with initial content (mirror + hash).
func (s *Store) CreateTextFile(ctx context.Context, projectID, path, content string) (*File, error) {
	var f *File
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		f, err = scanFile(tx.QueryRow(ctx, `
			INSERT INTO files (project_id, path, kind, size) VALUES ($1,$2,'text',$3)
			RETURNING `+fileCols, projectID, path, len(content)))
		if err != nil {
			return err
		}
		h := sha256.Sum256([]byte(content))
		_, err = tx.Exec(ctx, `INSERT INTO file_contents (file_id, content, content_hash) VALUES ($1,$2,$3)`,
			f.ID, content, h[:])
		return err
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Store) CreateAssetFile(ctx context.Context, projectID, path string, blobHash []byte, mime string, size int64) (*File, error) {
	return scanFile(s.Pool.QueryRow(ctx, `
		INSERT INTO files (project_id, path, kind, blob_hash, mime, size) VALUES ($1,$2,'asset',$3,$4,$5)
		RETURNING `+fileCols, projectID, path, blobHash, mime, size))
}

func (s *Store) FileByID(ctx context.Context, fileID string) (*File, error) {
	return scanFile(s.Pool.QueryRow(ctx, `SELECT `+fileCols+` FROM files WHERE id=$1`, fileID))
}

func (s *Store) FileByPath(ctx context.Context, projectID, path string) (*File, error) {
	return scanFile(s.Pool.QueryRow(ctx, `SELECT `+fileCols+` FROM files WHERE project_id=$1 AND path=$2`, projectID, path))
}

func (s *Store) ListFiles(ctx context.Context, projectID string) ([]*File, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+fileCols+` FROM files WHERE project_id=$1 ORDER BY path`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) RenameFile(ctx context.Context, fileID, newPath string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE files SET path=$2 WHERE id=$1`, fileID, newPath)
	return err
}

func (s *Store) DeleteFile(ctx context.Context, fileID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM files WHERE id=$1`, fileID)
	return err
}

func (s *Store) SetFileLocked(ctx context.Context, fileID string, locked bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE files SET locked=$2 WHERE id=$1`, fileID, locked)
	return err
}

// Text content mirror

func (s *Store) FileContent(ctx context.Context, fileID string) (string, error) {
	var content string
	err := s.Pool.QueryRow(ctx, `SELECT content FROM file_contents WHERE file_id=$1`, fileID).Scan(&content)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return content, err
}

func (s *Store) UpsertFileContent(ctx context.Context, fileID, content string) error {
	h := sha256.Sum256([]byte(content))
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO file_contents (file_id, content, content_hash) VALUES ($1,$2,$3)
		ON CONFLICT (file_id) DO UPDATE SET content=EXCLUDED.content, content_hash=EXCLUDED.content_hash, updated_at=now()`,
		fileID, content, h[:])
	return err
}

// Yjs binary state

func (s *Store) YjsState(ctx context.Context, fileID string) ([]byte, error) {
	var state []byte
	err := s.Pool.QueryRow(ctx, `SELECT state FROM yjs_state WHERE file_id=$1`, fileID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return state, err
}

func (s *Store) UpsertYjsState(ctx context.Context, fileID string, state []byte) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO yjs_state (file_id, state) VALUES ($1,$2)
		ON CONFLICT (file_id) DO UPDATE SET state=EXCLUDED.state, updated_at=now()`, fileID, state)
	return err
}

func (s *Store) DeleteYjsState(ctx context.Context, fileID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM yjs_state WHERE file_id=$1`, fileID)
	return err
}

// Blobs (content-addressed asset metadata; bytes live on disk)

func (s *Store) UpsertBlob(ctx context.Context, hash []byte, size int64) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO blobs (hash, size, refcount) VALUES ($1,$2,1)
		ON CONFLICT (hash) DO UPDATE SET refcount = blobs.refcount + 1`, hash, size)
	return err
}

// Search

type SearchHit struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	FileID      string `json:"fileId,omitempty"`
	Path        string `json:"path,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
}

func (s *Store) Search(ctx context.Context, userID, query string, limit int) ([]*SearchHit, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, p.name, f.id, f.path,
			substring(fc.content from greatest(position(lower($2) in lower(fc.content))-40, 1) for 120)
		FROM projects p
		JOIN project_members m ON m.project_id = p.id AND m.user_id = $1
		JOIN files f ON f.project_id = p.id AND f.kind = 'text'
		JOIN file_contents fc ON fc.file_id = f.id
		WHERE p.deleted_at IS NULL AND fc.content ILIKE '%' || $2 || '%'
		LIMIT $3`, userID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ProjectID, &h.ProjectName, &h.FileID, &h.Path, &h.Snippet); err != nil {
			return nil, err
		}
		out = append(out, &h)
	}
	return out, rows.Err()
}
