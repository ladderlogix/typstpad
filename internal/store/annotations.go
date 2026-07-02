package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Suggestions (tracked changes) and comments share the anchored-range model:
// anchors are opaque encoded Yjs relative positions produced by the collab sidecar.

type Suggestion struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"projectId"`
	FileID         string     `json:"fileId"`
	AuthorID       string     `json:"authorId"`
	AuthorName     string     `json:"authorName"`
	AuthorColor    string     `json:"authorColor"`
	Type           string     `json:"type"` // insert | delete | replace
	AnchorStart    []byte     `json:"-"`
	AnchorEnd      []byte     `json:"-"`
	AnchorStartB64 string     `json:"anchorStart"`
	AnchorEndB64   string     `json:"anchorEnd,omitempty"`
	InsertedText   *string    `json:"insertedText,omitempty"`
	DeletedPreview *string    `json:"deletedPreview,omitempty"`
	Status         string     `json:"status"`
	ResolvedBy     *string    `json:"resolvedBy,omitempty"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

const suggestionCols = `s.id, s.project_id, s.file_id, s.author_id, u.name, u.color, s.type,
	s.anchor_start, s.anchor_end, s.inserted_text, s.deleted_preview, s.status, s.resolved_by, s.resolved_at, s.created_at`

func scanSuggestion(row pgx.Row) (*Suggestion, error) {
	var sg Suggestion
	err := row.Scan(&sg.ID, &sg.ProjectID, &sg.FileID, &sg.AuthorID, &sg.AuthorName, &sg.AuthorColor, &sg.Type,
		&sg.AnchorStart, &sg.AnchorEnd, &sg.InsertedText, &sg.DeletedPreview, &sg.Status, &sg.ResolvedBy, &sg.ResolvedAt, &sg.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sg, nil
}

func (s *Store) CreateSuggestion(ctx context.Context, projectID, fileID, authorID, typ string, anchorStart, anchorEnd []byte, insertedText, deletedPreview *string) (*Suggestion, error) {
	return scanSuggestion(s.Pool.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO suggestions (project_id, file_id, author_id, type, anchor_start, anchor_end, inserted_text, deleted_preview)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			RETURNING *
		)
		SELECT `+suggestionCols+` FROM ins s JOIN users u ON u.id = s.author_id`,
		projectID, fileID, authorID, typ, anchorStart, anchorEnd, insertedText, deletedPreview))
}

func (s *Store) SuggestionByID(ctx context.Context, id string) (*Suggestion, error) {
	return scanSuggestion(s.Pool.QueryRow(ctx, `
		SELECT `+suggestionCols+` FROM suggestions s JOIN users u ON u.id = s.author_id WHERE s.id=$1`, id))
}

func (s *Store) ListSuggestions(ctx context.Context, fileID string, openOnly bool) ([]*Suggestion, error) {
	sql := `SELECT ` + suggestionCols + ` FROM suggestions s JOIN users u ON u.id = s.author_id WHERE s.file_id=$1`
	if openOnly {
		sql += ` AND s.status='open'`
	}
	sql += ` ORDER BY s.created_at`
	rows, err := s.Pool.Query(ctx, sql, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Suggestion
	for rows.Next() {
		sg, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sg)
	}
	return out, rows.Err()
}

// UpdateSuggestionAnchors re-anchors an open suggestion (inline suggest-mode
// coalescing: extending a pending record as the author keeps typing/deleting).
func (s *Store) UpdateSuggestionAnchors(ctx context.Context, id string, anchorStart, anchorEnd []byte, insertedText, deletedPreview *string) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE suggestions SET anchor_start=$2, anchor_end=$3, inserted_text=$4, deleted_preview=$5
		WHERE id=$1 AND status='open'`, id, anchorStart, anchorEnd, insertedText, deletedPreview)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveSuggestion flips an open suggestion to accepted/rejected; returns
// ErrNotFound if it was not open (guards concurrent double-accepts).
func (s *Store) ResolveSuggestion(ctx context.Context, id, status, resolvedBy string) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE suggestions SET status=$2, resolved_by=$3, resolved_at=now()
		WHERE id=$1 AND status='open'`, id, status, resolvedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteSuggestion(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM suggestions WHERE id=$1`, id)
	return err
}

// Comments

type Comment struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"projectId"`
	FileID         *string    `json:"fileId,omitempty"`
	AuthorID       string     `json:"authorId"`
	AuthorName     string     `json:"authorName"`
	AuthorColor    string     `json:"authorColor"`
	ParentID       *string    `json:"parentId,omitempty"`
	Body           string     `json:"body"`
	AnchorStart    []byte     `json:"-"`
	AnchorEnd      []byte     `json:"-"`
	AnchorStartB64 string     `json:"anchorStart,omitempty"`
	AnchorEndB64   string     `json:"anchorEnd,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	EditedAt       *time.Time `json:"editedAt,omitempty"`
}

const commentCols = `c.id, c.project_id, c.file_id, c.author_id, u.name, u.color, c.parent_id,
	c.body, c.anchor_start, c.anchor_end, c.status, c.created_at, c.edited_at`

func scanComment(row pgx.Row) (*Comment, error) {
	var c Comment
	err := row.Scan(&c.ID, &c.ProjectID, &c.FileID, &c.AuthorID, &c.AuthorName, &c.AuthorColor, &c.ParentID,
		&c.Body, &c.AnchorStart, &c.AnchorEnd, &c.Status, &c.CreatedAt, &c.EditedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) CreateComment(ctx context.Context, projectID string, fileID, parentID *string, authorID, body string, anchorStart, anchorEnd []byte) (*Comment, error) {
	return scanComment(s.Pool.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO comments (project_id, file_id, parent_id, author_id, body, anchor_start, anchor_end)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			RETURNING *
		)
		SELECT `+commentCols+` FROM ins c JOIN users u ON u.id = c.author_id`,
		projectID, fileID, parentID, authorID, body, anchorStart, anchorEnd))
}

func (s *Store) CommentByID(ctx context.Context, id string) (*Comment, error) {
	return scanComment(s.Pool.QueryRow(ctx, `
		SELECT `+commentCols+` FROM comments c JOIN users u ON u.id = c.author_id WHERE c.id=$1`, id))
}

func (s *Store) ListComments(ctx context.Context, projectID string) ([]*Comment, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+commentCols+` FROM comments c JOIN users u ON u.id = c.author_id
		WHERE c.project_id=$1 ORDER BY c.created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateCommentBody(ctx context.Context, id, body string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE comments SET body=$2, edited_at=now() WHERE id=$1`, id, body)
	return err
}

func (s *Store) SetCommentStatus(ctx context.Context, id, status string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE comments SET status=$2 WHERE id=$1`, id, status)
	return err
}

func (s *Store) DeleteComment(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM comments WHERE id=$1`, id)
	return err
}
