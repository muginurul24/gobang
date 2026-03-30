package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RepositoryContract interface {
	Create(ctx context.Context, params CreateParams, createdAt time.Time) (Notification, error)
	ListByScope(ctx context.Context, params ListParams) ([]Notification, error)
	MarkRead(ctx context.Context, id string, readAt time.Time) error
	CountUnread(ctx context.Context, scopeType ScopeType, scopeID string) (int, error)
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, params CreateParams, createdAt time.Time) (Notification, error) {
	var n Notification
	err := r.pool.QueryRow(ctx, `
		INSERT INTO notifications (scope_type, scope_id, event_type, title, body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, scope_type, scope_id, event_type, title, body, read_at, created_at
	`, params.ScopeType, params.ScopeID, params.EventType, params.Title, params.Body, createdAt).Scan(
		&n.ID, &n.ScopeType, &n.ScopeID, &n.EventType, &n.Title, &n.Body, &n.ReadAt, &n.CreatedAt,
	)
	if err != nil {
		return Notification{}, fmt.Errorf("insert notification: %w", err)
	}

	return n, nil
}

func (r *Repository) ListByScope(ctx context.Context, params ListParams) ([]Notification, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, scope_type, scope_id, event_type, title, body, read_at, created_at
		FROM notifications
		WHERE scope_type = $1 AND scope_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, params.ScopeType, params.ScopeID, limit, params.Offset)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.ScopeType, &n.ScopeID, &n.EventType, &n.Title, &n.Body, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}

	return notifications, nil
}

func (r *Repository) MarkRead(ctx context.Context, id string, readAt time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notifications SET read_at = $2 WHERE id = $1 AND read_at IS NULL
	`, id, readAt)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) CountUnread(ctx context.Context, scopeType ScopeType, scopeID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications
		WHERE scope_type = $1 AND scope_id = $2 AND read_at IS NULL
	`, scopeType, scopeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}

	return count, nil
}
