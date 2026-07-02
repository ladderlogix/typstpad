package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	// Token is only set on creation.
	Token string `json:"token,omitempty"`
}

func (s *Store) CreateAPIToken(ctx context.Context, userID, name string, tokenHash []byte, scopes []string, expiresAt *time.Time) (*APIToken, error) {
	var t APIToken
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO api_tokens (user_id, name, token_hash, scopes, expires_at)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, user_id, name, scopes, expires_at, last_used_at, created_at`,
		userID, name, tokenHash, scopes, expiresAt).
		Scan(&t.ID, &t.UserID, &t.Name, &t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListAPITokens(ctx context.Context, userID string) ([]*APIToken, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, user_id, name, scopes, expires_at, last_used_at, created_at
		FROM api_tokens WHERE user_id=$1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// APITokenUser resolves a token hash to (user, scopes), enforcing expiry.
func (s *Store) APITokenUser(ctx context.Context, tokenHash []byte) (*User, []string, error) {
	var scopes []string
	u, err := func() (*User, error) {
		row := s.Pool.QueryRow(ctx, `
			WITH tok AS (
				UPDATE api_tokens SET last_used_at=now()
				WHERE token_hash=$1 AND (expires_at IS NULL OR expires_at > now())
				RETURNING user_id, scopes
			)
			SELECT `+userCols+`, tok.scopes FROM users JOIN tok ON users.id = tok.user_id`, tokenHash)
		var u User
		err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsAdmin, &u.Color, &u.EmailVerified, &u.CreatedAt, &scopes)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		return &u, nil
	}()
	if err != nil {
		return nil, nil, err
	}
	return u, scopes, nil
}

func (s *Store) DeleteAPIToken(ctx context.Context, id, userID string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM api_tokens WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
