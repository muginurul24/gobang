package notifications

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RepositoryContract interface {
	Create(ctx context.Context, params CreateParams, createdAt time.Time) (Notification, error)
	ListByScope(ctx context.Context, params ListParams) (ListResult, error)
	MarkRead(ctx context.Context, params MarkReadParams, readAt time.Time) error
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

func (r *Repository) ListByScope(ctx context.Context, params ListParams) (ListResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	clauses := []string{
		"scope_type = $1",
		"scope_id = $2",
	}
	args := []any{params.ScopeType, params.ScopeID}

	if query := strings.TrimSpace(params.Query); query != "" {
		args = append(args, "%"+query+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		clauses = append(clauses, fmt.Sprintf("(event_type ILIKE %s OR title ILIKE %s OR body ILIKE %s)", placeholder, placeholder, placeholder))
	}

	switch strings.TrimSpace(params.ReadState) {
	case "unread":
		clauses = append(clauses, "read_at IS NULL")
	case "read":
		clauses = append(clauses, "read_at IS NOT NULL")
	}

	if params.CreatedFrom != nil {
		args = append(args, *params.CreatedFrom)
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}

	if params.CreatedTo != nil {
		args = append(args, *params.CreatedTo)
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", len(args)))
	}

	whereClause := strings.Join(clauses, " AND ")

	var totalCount int
	countQuery := "SELECT count(*) FROM notifications WHERE " + whereClause
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return ListResult{}, fmt.Errorf("count notifications: %w", err)
	}

	args = append(args, limit, params.Offset)
	rows, err := r.pool.Query(ctx, `
		SELECT id, scope_type, scope_id, event_type, title, body, read_at, created_at
		FROM notifications
		WHERE `+whereClause+`
		ORDER BY created_at DESC
		LIMIT $`+fmt.Sprintf("%d", len(args)-1)+` OFFSET $`+fmt.Sprintf("%d", len(args)), args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.ScopeType, &n.ScopeID, &n.EventType, &n.Title, &n.Body, &n.ReadAt, &n.CreatedAt); err != nil {
			return ListResult{}, fmt.Errorf("scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}

	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate notifications: %w", err)
	}

	return ListResult{
		Items:      notifications,
		TotalCount: totalCount,
	}, nil
}

func (r *Repository) MarkRead(ctx context.Context, params MarkReadParams, readAt time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notifications
		SET read_at = $4
		WHERE id = $1
			AND scope_type = $2
			AND scope_id = $3
			AND read_at IS NULL
	`, params.ID, params.ScopeType, params.ScopeID, readAt)
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
