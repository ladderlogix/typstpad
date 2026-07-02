package store

import (
	"context"
	"time"
)

type Notification struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	ActorName   string     `json:"actorName,omitempty"`
	ProjectID   *string    `json:"projectId,omitempty"`
	ProjectName string     `json:"projectName,omitempty"`
	Summary     string     `json:"summary"`
	ReadAt      *time.Time `json:"readAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// CreateNotification records one feed entry. actorID/projectID may be empty.
func (s *Store) CreateNotification(ctx context.Context, userID, actorID, typ, projectID, summary string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO notifications (user_id, actor_id, type, project_id, summary)
		VALUES ($1, NULLIF($2,'')::uuid, $3, NULLIF($4,'')::uuid, $5)`,
		userID, actorID, typ, projectID, summary)
	return err
}

func (s *Store) ListNotifications(ctx context.Context, userID string, limit int) ([]*Notification, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT n.id, n.type, COALESCE(a.name,''), n.project_id, COALESCE(p.name,''),
		       n.summary, n.read_at, n.created_at
		FROM notifications n
		LEFT JOIN users a ON a.id = n.actor_id
		LEFT JOIN projects p ON p.id = n.project_id
		WHERE n.user_id = $1
		ORDER BY n.created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.ActorName, &n.ProjectID, &n.ProjectName,
			&n.Summary, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &n)
	}
	return out, rows.Err()
}

func (s *Store) CountUnreadNotifications(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE notifications SET read_at=now() WHERE user_id=$1 AND read_at IS NULL`, userID)
	return err
}

func (s *Store) MarkNotificationRead(ctx context.Context, userID, id string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE notifications SET read_at=now() WHERE user_id=$1 AND id=$2`, userID, id)
	return err
}
