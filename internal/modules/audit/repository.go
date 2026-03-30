package audit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListGlobal(ctx context.Context, filter Filter) ([]LogEntry, error) {
	query := `
		SELECT id, actor_user_id, actor_role, store_id, action, target_type, target_id, payload_masked, host(ip_address), user_agent, created_at
		FROM audit_logs
	`
	args := []any{}
	if filter.StoreID != nil && *filter.StoreID != "" {
		query += ` WHERE store_id = $1`
		args = append(args, *filter.StoreID)
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d", sanitizeLimit(filter.Limit))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list global audit logs: %w", err)
	}
	defer rows.Close()

	return collectLogs(rows)
}

func (r *Repository) ListOwnerScoped(ctx context.Context, ownerUserID string, filter Filter) ([]LogEntry, error) {
	query := `
		SELECT
			al.id,
			al.actor_user_id,
			al.actor_role,
			al.store_id,
			al.action,
			al.target_type,
			al.target_id,
			al.payload_masked,
			host(al.ip_address),
			al.user_agent,
			al.created_at
		FROM audit_logs al
		LEFT JOIN stores s ON s.id = al.store_id
		LEFT JOIN users actor ON actor.id = al.actor_user_id
		LEFT JOIN users target_user
			ON al.target_type = 'user'
			AND target_user.id = al.target_id
		WHERE (
			al.actor_user_id = $1 OR
			s.owner_user_id = $1 OR
			actor.created_by_user_id = $1 OR
			target_user.created_by_user_id = $1
		)
	`
	args := []any{ownerUserID}
	if filter.StoreID != nil && *filter.StoreID != "" {
		query += " AND al.store_id = $2"
		args = append(args, *filter.StoreID)
	}

	query += fmt.Sprintf(" ORDER BY al.created_at DESC LIMIT %d", sanitizeLimit(filter.Limit))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list owner audit logs: %w", err)
	}
	defer rows.Close()

	return collectLogs(rows)
}

func (r *Repository) OwnerHasStore(ctx context.Context, ownerUserID string, storeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM stores
			WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL
		)
	`, storeID, ownerUserID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check owner store scope: %w", err)
	}

	return exists, nil
}

func collectLogs(rows pgx.Rows) ([]LogEntry, error) {
	var logs []LogEntry
	for rows.Next() {
		var entry LogEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.ActorUserID,
			&entry.ActorRole,
			&entry.StoreID,
			&entry.Action,
			&entry.TargetType,
			&entry.TargetID,
			&entry.Payload,
			&entry.IPAddress,
			&entry.UserAgent,
			&entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}

		logs = append(logs, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit logs: %w", err)
	}

	return logs, nil
}

func sanitizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return 50
	case limit > 100:
		return 100
	default:
		return limit
	}
}
