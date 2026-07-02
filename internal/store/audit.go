package store

import (
	"context"
	"time"
)

type AuditEntry struct {
	ID         string    `json:"id"`
	ActorEmail string    `json:"actorEmail"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (s *Store) RecordAudit(ctx context.Context, actorID, actorEmail, action, target, detail string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO audit_log (actor_id, actor_email, action, target, detail)
		VALUES (NULLIF($1,'')::uuid, $2, $3, $4, $5)`,
		actorID, actorEmail, action, target, detail)
	return err
}

func (s *Store) ListAudit(ctx context.Context, action string, limit int) ([]*AuditEntry, error) {
	sql := `SELECT id, actor_email, action, target, detail, created_at FROM audit_log`
	args := []any{}
	if action != "" {
		args = append(args, action)
		sql += ` WHERE action = $1`
	}
	args = append(args, limit)
	sql += ` ORDER BY created_at DESC LIMIT $` + itoa(len(args))
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuditEntry
	for rows.Next() {
		var a AuditEntry
		if err := rows.Scan(&a.ID, &a.ActorEmail, &a.Action, &a.Target, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}
