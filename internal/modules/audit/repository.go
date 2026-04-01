package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListGlobal(ctx context.Context, filter Filter) (ListResult, error) {
	query := `
		SELECT id, actor_user_id, actor_role, store_id, action, target_type, target_id, payload_masked, host(ip_address), user_agent, created_at
		FROM audit_logs
	`
	args := make([]any, 0, 4)
	clauses, _ := appendFilterClauses(nil, 1, filter, &args)
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}

	totalCount, err := r.countLogs(ctx, `
		SELECT count(*) FROM audit_logs
	`, clauses, args)
	if err != nil {
		return ListResult{}, err
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", sanitizeLimit(filter.Limit), sanitizeOffset(filter.Offset))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list global audit logs: %w", err)
	}
	defer rows.Close()

	items, err := collectLogs(rows)
	if err != nil {
		return ListResult{}, err
	}

	return ListResult{
		Items:      items,
		TotalCount: totalCount,
	}, nil
}

func (r *Repository) ListOwnerScoped(ctx context.Context, ownerUserID string, filter Filter) (ListResult, error) {
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
	clauses, _ := appendFilterClauses([]string{}, 2, filter, &args)
	if len(clauses) > 0 {
		query += " AND " + strings.Join(clauses, " AND ")
	}

	totalCount, err := r.countLogs(ctx, `
		SELECT count(*)
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
	`, clauses, args)
	if err != nil {
		return ListResult{}, err
	}

	query += fmt.Sprintf(" ORDER BY al.created_at DESC LIMIT %d OFFSET %d", sanitizeLimit(filter.Limit), sanitizeOffset(filter.Offset))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list owner audit logs: %w", err)
	}
	defer rows.Close()

	items, err := collectLogs(rows)
	if err != nil {
		return ListResult{}, err
	}

	return ListResult{
		Items:      items,
		TotalCount: totalCount,
	}, nil
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

func (r *Repository) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM audit_logs
		WHERE created_at < $1
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune audit logs: %w", err)
	}

	return tag.RowsAffected(), nil
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

func (r *Repository) countLogs(ctx context.Context, baseQuery string, clauses []string, args []any) (int, error) {
	query := baseQuery
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}

	var totalCount int
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&totalCount); err != nil {
		return 0, fmt.Errorf("count audit logs: %w", err)
	}

	return totalCount, nil
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

func sanitizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func appendFilterClauses(clauses []string, startAt int, filter Filter, args *[]any) ([]string, int) {
	next := startAt

	if filter.StoreID != nil {
		clauses = append(clauses, fmt.Sprintf("al.store_id = $%d", next))
		if startAt == 1 {
			clauses[len(clauses)-1] = fmt.Sprintf("store_id = $%d", next)
		}
		*args = append(*args, *filter.StoreID)
		next++
	}

	if filter.Action != nil {
		clauses = append(clauses, fmt.Sprintf("action ILIKE $%d", next))
		if startAt != 1 {
			clauses[len(clauses)-1] = fmt.Sprintf("al.action ILIKE $%d", next)
		}
		*args = append(*args, "%"+*filter.Action+"%")
		next++
	}

	if filter.ActorRole != nil {
		clauses = append(clauses, fmt.Sprintf("actor_role = $%d", next))
		if startAt != 1 {
			clauses[len(clauses)-1] = fmt.Sprintf("al.actor_role = $%d", next)
		}
		*args = append(*args, *filter.ActorRole)
		next++
	}

	if filter.TargetType != nil {
		clauses = append(clauses, fmt.Sprintf("target_type = $%d", next))
		if startAt != 1 {
			clauses[len(clauses)-1] = fmt.Sprintf("al.target_type = $%d", next)
		}
		*args = append(*args, *filter.TargetType)
		next++
	}

	if filter.CreatedFrom != nil {
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", next))
		if startAt != 1 {
			clauses[len(clauses)-1] = fmt.Sprintf("al.created_at >= $%d", next)
		}
		*args = append(*args, *filter.CreatedFrom)
		next++
	}

	if filter.CreatedTo != nil {
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", next))
		if startAt != 1 {
			clauses[len(clauses)-1] = fmt.Sprintf("al.created_at <= $%d", next)
		}
		*args = append(*args, *filter.CreatedTo)
		next++
	}

	return clauses, next
}
