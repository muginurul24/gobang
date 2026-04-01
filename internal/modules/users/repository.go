package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListDirectory(ctx context.Context, filter ListFilter) (Page, error) {
	whereClause, args := buildDirectoryWhere(filter)

	var summary DirectorySummary
	if err := r.pool.QueryRow(ctx, `
		SELECT
			count(*)::int AS total_count,
			COALESCE(count(*) FILTER (WHERE role = 'owner'), 0)::int AS owner_count,
			COALESCE(count(*) FILTER (WHERE role = 'superadmin'), 0)::int AS superadmin_count,
			COALESCE(count(*) FILTER (WHERE role = 'dev'), 0)::int AS dev_count,
			COALESCE(count(*) FILTER (WHERE is_active), 0)::int AS active_count,
			COALESCE(count(*) FILTER (WHERE NOT is_active), 0)::int AS inactive_count
		FROM users
		WHERE `+whereClause, args...).Scan(
		&summary.TotalCount,
		&summary.OwnerCount,
		&summary.SuperadminCount,
		&summary.DevCount,
		&summary.ActiveCount,
		&summary.InactiveCount,
	); err != nil {
		return Page{}, fmt.Errorf("summarize user directory: %w", err)
	}

	listArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			email,
			username,
			role,
			is_active,
			created_by_user_id,
			created_at,
			updated_at,
			last_login_at
		FROM users
		WHERE `+whereClause+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)+1)+`
		OFFSET $`+fmt.Sprint(len(args)+2), listArgs...)
	if err != nil {
		return Page{}, fmt.Errorf("list user directory: %w", err)
	}
	defer rows.Close()

	items, err := collectUsers(rows)
	if err != nil {
		return Page{}, err
	}

	return Page{
		Items:   items,
		Summary: summary,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	}, nil
}

func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (
			email,
			username,
			password_hash,
			role,
			is_active,
			created_by_user_id,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, TRUE, $5::uuid, $6, $6)
		RETURNING
			id,
			email,
			username,
			role,
			is_active,
			created_by_user_id,
			created_at,
			updated_at,
			last_login_at
	`, params.Email, params.Username, params.PasswordHash, params.Role, params.CreatedByUserID, params.OccurredAt).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.Role,
		&user.IsActive,
		&user.CreatedByUserID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	if err != nil {
		if duplicateIdentity(err) {
			return User{}, ErrDuplicateIdentity
		}

		return User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			email,
			username,
			role,
			is_active,
			created_by_user_id,
			created_at,
			updated_at,
			last_login_at
		FROM users
		WHERE id = $1::uuid
		LIMIT 1
	`, strings.TrimSpace(userID)).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.Role,
		&user.IsActive,
		&user.CreatedByUserID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}

		return User{}, fmt.Errorf("get user by id: %w", err)
	}

	return user, nil
}

func (r *Repository) UpdateUserStatus(ctx context.Context, params UpdateUserStatusParams) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		UPDATE users
		SET
			is_active = $2,
			updated_at = $3
		WHERE id = $1::uuid
		RETURNING
			id,
			email,
			username,
			role,
			is_active,
			created_by_user_id,
			created_at,
			updated_at,
			last_login_at
	`, params.UserID, params.IsActive, params.OccurredAt).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.Role,
		&user.IsActive,
		&user.CreatedByUserID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}

		return User{}, fmt.Errorf("update user status: %w", err)
	}

	return user, nil
}

func (r *Repository) InsertAuditLog(
	ctx context.Context,
	actorUserID *string,
	actorRole string,
	action string,
	targetID *string,
	payload map[string]any,
	ipAddress string,
	userAgent string,
	occurredAt time.Time,
) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		encoded = []byte("{}")
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_logs (
			actor_user_id,
			actor_role,
			action,
			target_type,
			target_id,
			payload_masked,
			ip_address,
			user_agent,
			created_at
		)
		VALUES ($1::uuid, $2, $3, 'user', $4::uuid, $5::jsonb, $6, $7, $8)
	`, actorUserID, actorRole, action, targetID, string(encoded), nullableStringValue(ipAddress), nullableStringValue(userAgent), occurredAt)
	if err != nil {
		return fmt.Errorf("insert user audit log: %w", err)
	}

	return nil
}

func collectUsers(rows pgx.Rows) ([]User, error) {
	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.Username,
			&user.Role,
			&user.IsActive,
			&user.CreatedByUserID,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.LastLoginAt,
		); err != nil {
			return nil, fmt.Errorf("scan user directory row: %w", err)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user directory rows: %w", err)
	}

	return users, nil
}

func buildDirectoryWhere(filter ListFilter) (string, []any) {
	clauses := []string{
		"role IN ('dev', 'superadmin', 'owner')",
	}
	args := make([]any, 0, 6)
	next := 1

	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		placeholder := fmt.Sprintf("$%d", next)
		clauses = append(clauses, "(username::text ILIKE "+placeholder+" OR email::text ILIKE "+placeholder+")")
		next++
	}

	if filter.Role != nil {
		args = append(args, *filter.Role)
		clauses = append(clauses, fmt.Sprintf("role = $%d", next))
		next++
	}

	if filter.IsActive != nil {
		args = append(args, *filter.IsActive)
		clauses = append(clauses, fmt.Sprintf("is_active = $%d", next))
		next++
	}

	if filter.CreatedFrom != nil {
		args = append(args, *filter.CreatedFrom)
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", next))
		next++
	}

	if filter.CreatedTo != nil {
		args = append(args, *filter.CreatedTo)
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", next))
	}

	return strings.Join(clauses, " AND "), args
}

func duplicateIdentity(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}

	return pgErr.ConstraintName == "users_email_key" || pgErr.ConstraintName == "users_username_key"
}

func nullableStringValue(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
