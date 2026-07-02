package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("not found")

type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	PasswordHash  *string   `json:"-"`
	IsAdmin       bool      `json:"isAdmin"`
	Color         string    `json:"color"`
	EmailVerified bool      `json:"emailVerified"`
	CreatedAt     time.Time `json:"createdAt"`
}

const userCols = `id, email, name, password_hash, is_admin, color, email_verified, created_at`

// userColsU is userCols qualified with the `u` alias, for queries that join
// another table which also has id/created_at columns (e.g. oidc_identities).
const userColsU = `u.id, u.email, u.name, u.password_hash, u.is_admin, u.color, u.email_verified, u.created_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsAdmin, &u.Color, &u.EmailVerified, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser inserts a user; the first user ever created becomes admin.
func (s *Store) CreateUser(ctx context.Context, email, name string, passwordHash *string, color string) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `
		INSERT INTO users (email, name, password_hash, color, is_admin)
		VALUES ($1, $2, $3, $4, NOT EXISTS(SELECT 1 FROM users))
		RETURNING `+userCols, email, name, passwordHash, color))
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE email=$1`, email))
}

func (s *Store) UserByID(ctx context.Context, id string) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
}

func (s *Store) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) SetUserAdmin(ctx context.Context, id string, isAdmin bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET is_admin=$2 WHERE id=$1`, id, isAdmin)
	return err
}

func (s *Store) UpdateUser(ctx context.Context, id, name, color string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET name=$2, color=$3 WHERE id=$1`, id, name, color)
	return err
}

func (s *Store) SetPassword(ctx context.Context, id, passwordHash string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, id, passwordHash)
	return err
}

// DeleteUserSessions logs a user out everywhere (e.g. after a password reset).
func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID)
	return err
}

// Password resets

func (s *Store) CreatePasswordReset(ctx context.Context, tokenHash []byte, userID string, expiresAt time.Time) error {
	if _, err := s.Pool.Exec(ctx, `DELETE FROM password_resets WHERE user_id=$1`, userID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO password_resets (token_hash, user_id, expires_at) VALUES ($1,$2,$3)`,
		tokenHash, userID, expiresAt)
	return err
}

// ConsumePasswordReset validates and deletes a reset token, returning its user.
func (s *Store) ConsumePasswordReset(ctx context.Context, tokenHash []byte) (string, error) {
	var userID string
	err := s.Pool.QueryRow(ctx, `
		DELETE FROM password_resets
		WHERE token_hash=$1 AND expires_at > now()
		RETURNING user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	return err
}

// UserByOIDC finds a user by OIDC identity.
func (s *Store) UserByOIDC(ctx context.Context, issuer, subject string) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `
		SELECT `+userColsU+` FROM users u
		JOIN oidc_identities oi ON oi.user_id = u.id
		WHERE oi.issuer=$1 AND oi.subject=$2`, issuer, subject))
}

func (s *Store) LinkOIDC(ctx context.Context, userID, issuer, subject string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO oidc_identities (user_id, issuer, subject) VALUES ($1,$2,$3)
		ON CONFLICT (issuer, subject) DO NOTHING`, userID, issuer, subject)
	return err
}

// Email verification

func (s *Store) MarkEmailVerified(ctx context.Context, userID string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET email_verified = true WHERE id = $1`, userID)
	return err
}

func (s *Store) CreateEmailVerification(ctx context.Context, tokenHash []byte, userID string, expiresAt time.Time) error {
	// One active token per user is enough; clear old ones first.
	if _, err := s.Pool.Exec(ctx, `DELETE FROM email_verifications WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO email_verifications (token_hash, user_id, expires_at) VALUES ($1,$2,$3)`,
		tokenHash, userID, expiresAt)
	return err
}

// ConsumeEmailVerification validates and deletes a token, returning its user.
func (s *Store) ConsumeEmailVerification(ctx context.Context, tokenHash []byte) (string, error) {
	var userID string
	err := s.Pool.QueryRow(ctx, `
		DELETE FROM email_verifications
		WHERE token_hash = $1 AND expires_at > now()
		RETURNING user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

// Sessions

type Session struct {
	UserID    string
	ExpiresAt time.Time
}

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, userID string, expiresAt time.Time, userAgent string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at, user_agent)
		VALUES ($1,$2,$3,$4)`, tokenHash, userID, expiresAt, userAgent)
	return err
}

// SessionUser resolves a session token hash to its user, sliding the expiry.
func (s *Store) SessionUser(ctx context.Context, tokenHash []byte, slideTo time.Time) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `
		WITH sess AS (
			UPDATE sessions SET last_seen_at=now(), expires_at=$2
			WHERE token_hash=$1 AND expires_at > now()
			RETURNING user_id
		)
		SELECT `+userCols+` FROM users JOIN sess ON users.id = sess.user_id`,
		tokenHash, slideTo))
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	return err
}

// Settings

func (s *Store) SettingBool(ctx context.Context, key string, def bool) bool {
	var v bool
	err := s.Pool.QueryRow(ctx, `SELECT (value)::boolean FROM settings WHERE key=$1`, key).Scan(&v)
	if err != nil {
		return def
	}
	return v
}

func (s *Store) SetSetting(ctx context.Context, key string, valueJSON string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO settings (key, value) VALUES ($1, $2::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, valueJSON)
	return err
}

// AllSettings returns every setting as key → scalar text (jsonb strings,
// bools and numbers all unwrap via #>>'{}').
func (s *Store) AllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT key, value #>> '{}' FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM settings WHERE key=$1`, key)
	return err
}
