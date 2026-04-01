package stores

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

func (r *Repository) AcquireLowBalanceSweepLock(ctx context.Context) (LowBalanceSweepLock, bool, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire low balance sweep connection: %w", err)
	}

	var locked bool
	if err := conn.QueryRow(ctx, `
		SELECT pg_try_advisory_lock($1, $2)
	`, lowBalanceSweepLockNamespace, 1).Scan(&locked); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("try low balance sweep advisory lock: %w", err)
	}

	if !locked {
		conn.Release()
		return nil, false, nil
	}

	return &repositoryLowBalanceSweepLock{conn: conn}, true, nil
}

func (r *Repository) ListStoresForLowBalanceSweep(ctx context.Context) ([]Store, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			owner_user_id,
			name,
			slug,
			status,
			callback_url,
			current_balance::text,
			low_balance_threshold::text,
			0,
			created_at,
			updated_at,
			deleted_at
		FROM stores
		WHERE deleted_at IS NULL
			AND status = 'active'
			AND low_balance_threshold IS NOT NULL
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list stores for low balance sweep: %w", err)
	}
	defer rows.Close()

	return collectStores(rows)
}

func (r *Repository) HasRecentLowBalanceNotification(ctx context.Context, storeID string, since time.Time) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM notifications
			WHERE scope_type = 'store'
				AND scope_id = $1
				AND event_type = 'store.low_balance'
				AND created_at >= $2
		)
	`, strings.TrimSpace(storeID), since).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check recent low balance notification: %w", err)
	}

	return exists, nil
}

func (r *Repository) ListStoresForOwner(ctx context.Context, ownerUserID string) ([]Store, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			s.id,
			s.owner_user_id,
			s.name,
			s.slug,
			s.status,
			s.callback_url,
			s.current_balance::text,
			s.low_balance_threshold::text,
			COUNT(ss.id)::int AS staff_count,
			s.created_at,
			s.updated_at,
			s.deleted_at
		FROM stores s
		LEFT JOIN store_staff ss ON ss.store_id = s.id
		WHERE s.owner_user_id = $1::uuid AND s.deleted_at IS NULL
		GROUP BY s.id
		ORDER BY s.created_at DESC
	`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list stores for owner: %w", err)
	}
	defer rows.Close()

	return collectStores(rows)
}

func (r *Repository) ListStoresForStaff(ctx context.Context, userID string) ([]Store, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			s.id,
			s.owner_user_id,
			s.name,
			s.slug,
			s.status,
			s.callback_url,
			s.current_balance::text,
			s.low_balance_threshold::text,
			COUNT(ss2.id)::int AS staff_count,
			s.created_at,
			s.updated_at,
			s.deleted_at
		FROM store_staff ss
		JOIN stores s ON s.id = ss.store_id
		LEFT JOIN store_staff ss2 ON ss2.store_id = s.id
		WHERE ss.user_id = $1::uuid AND s.deleted_at IS NULL
		GROUP BY s.id
		ORDER BY s.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list stores for staff: %w", err)
	}
	defer rows.Close()

	return collectStores(rows)
}

func (r *Repository) ListAllStores(ctx context.Context) ([]Store, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			s.id,
			s.owner_user_id,
			s.name,
			s.slug,
			s.status,
			s.callback_url,
			s.current_balance::text,
			s.low_balance_threshold::text,
			COUNT(ss.id)::int AS staff_count,
			s.created_at,
			s.updated_at,
			s.deleted_at
		FROM stores s
		LEFT JOIN store_staff ss ON ss.store_id = s.id
		WHERE s.deleted_at IS NULL
		GROUP BY s.id
		ORDER BY s.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list all stores: %w", err)
	}
	defer rows.Close()

	return collectStores(rows)
}

func (r *Repository) ListStoreDirectoryForOwner(ctx context.Context, ownerUserID string, filter ListStoreDirectoryFilter) (StorePage, error) {
	return r.listStoreDirectory(ctx, storeDirectoryQuery{
		joins: []string{},
		clauses: []string{
			"s.deleted_at IS NULL",
			"s.owner_user_id = $1::uuid",
		},
		args: []any{ownerUserID},
	}, filter)
}

func (r *Repository) ListStoreDirectoryForStaff(ctx context.Context, userID string, filter ListStoreDirectoryFilter) (StorePage, error) {
	return r.listStoreDirectory(ctx, storeDirectoryQuery{
		joins: []string{
			"INNER JOIN store_staff access_ss ON access_ss.store_id = s.id AND access_ss.user_id = $1::uuid",
		},
		clauses: []string{
			"s.deleted_at IS NULL",
		},
		args: []any{userID},
	}, filter)
}

func (r *Repository) ListStoreDirectoryForPlatform(ctx context.Context, filter ListStoreDirectoryFilter) (StorePage, error) {
	return r.listStoreDirectory(ctx, storeDirectoryQuery{
		joins: []string{},
		clauses: []string{
			"s.deleted_at IS NULL",
		},
		args: []any{},
	}, filter)
}

func (r *Repository) GetStoreByID(ctx context.Context, storeID string) (Store, error) {
	var store Store
	err := r.pool.QueryRow(ctx, `
		SELECT
			s.id,
			s.owner_user_id,
			s.name,
			s.slug,
			s.status,
			s.callback_url,
			s.current_balance::text,
			s.low_balance_threshold::text,
			(
				SELECT COUNT(*)
				FROM store_staff ss
				WHERE ss.store_id = s.id
			)::int AS staff_count,
			s.created_at,
			s.updated_at,
			s.deleted_at
		FROM stores s
		WHERE s.id = $1::uuid
		LIMIT 1
	`, storeID).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.Name,
		&store.Slug,
		&store.Status,
		&store.CallbackURL,
		&store.CurrentBalance,
		&store.LowBalanceThreshold,
		&store.StaffCount,
		&store.CreatedAt,
		&store.UpdatedAt,
		&store.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Store{}, ErrNotFound
		}

		return Store{}, fmt.Errorf("get store by id: %w", err)
	}

	return store, nil
}

func (r *Repository) IsStoreStaff(ctx context.Context, storeID string, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM store_staff
			WHERE store_id = $1::uuid AND user_id = $2::uuid
		)
	`, storeID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check store staff relation: %w", err)
	}

	return exists, nil
}

func (r *Repository) CreateStore(ctx context.Context, params CreateStoreParams) (Store, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Store{}, fmt.Errorf("begin create store transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var store Store
	err = tx.QueryRow(ctx, `
		INSERT INTO stores (
			owner_user_id,
			name,
			slug,
			status,
			api_token_hash,
			low_balance_threshold,
			created_at,
			updated_at
		)
		VALUES ($1::uuid, $2, $3, 'active', $4, $5, $6, $6)
		RETURNING
			id,
			owner_user_id,
			name,
			slug,
			status,
			callback_url,
			current_balance::text,
			low_balance_threshold::text,
			0,
			created_at,
			updated_at,
			deleted_at
	`, params.OwnerUserID, params.Name, params.Slug, params.APITokenHash, nullableString(params.LowBalanceThreshold), params.OccurredAt).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.Name,
		&store.Slug,
		&store.Status,
		&store.CallbackURL,
		&store.CurrentBalance,
		&store.LowBalanceThreshold,
		&store.StaffCount,
		&store.CreatedAt,
		&store.UpdatedAt,
		&store.DeletedAt,
	)
	if err != nil {
		if duplicateSlug(err) {
			return Store{}, ErrDuplicateSlug
		}

		return Store{}, fmt.Errorf("create store: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_accounts (store_id, currency, created_at)
		VALUES ($1, 'IDR', $2)
	`, store.ID, params.OccurredAt); err != nil {
		return Store{}, fmt.Errorf("create ledger account for store: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Store{}, fmt.Errorf("commit create store transaction: %w", err)
	}

	return store, nil
}

func (r *Repository) UpdateStore(ctx context.Context, params UpdateStoreParams) (Store, error) {
	var store Store
	err := r.pool.QueryRow(ctx, `
		UPDATE stores
		SET
			name = $2,
			status = $3,
			low_balance_threshold = $4,
			updated_at = $5
		WHERE id = $1::uuid AND deleted_at IS NULL
		RETURNING
			id,
			owner_user_id,
			name,
			slug,
			status,
			callback_url,
			current_balance::text,
			low_balance_threshold::text,
			(
				SELECT COUNT(*)
				FROM store_staff ss
				WHERE ss.store_id = stores.id
			)::int,
			created_at,
			updated_at,
			deleted_at
	`, params.StoreID, params.Name, params.Status, nullableString(params.LowBalanceThreshold), params.OccurredAt).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.Name,
		&store.Slug,
		&store.Status,
		&store.CallbackURL,
		&store.CurrentBalance,
		&store.LowBalanceThreshold,
		&store.StaffCount,
		&store.CreatedAt,
		&store.UpdatedAt,
		&store.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Store{}, ErrNotFound
		}

		return Store{}, fmt.Errorf("update store: %w", err)
	}

	return store, nil
}

func (r *Repository) SoftDeleteStore(ctx context.Context, params SoftDeleteStoreParams) error {
	commandTag, err := r.pool.Exec(ctx, `
		UPDATE stores
		SET
			status = 'deleted',
			deleted_at = $2,
			updated_at = $2
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, params.StoreID, params.OccurredAt)
	if err != nil {
		return fmt.Errorf("soft delete store: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) RotateToken(ctx context.Context, params RotateTokenParams) error {
	commandTag, err := r.pool.Exec(ctx, `
		UPDATE stores
		SET api_token_hash = $2, updated_at = $3
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, params.StoreID, params.APITokenHash, params.OccurredAt)
	if err != nil {
		return fmt.Errorf("rotate store token: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) UpdateCallbackURL(ctx context.Context, params UpdateCallbackParams) (Store, error) {
	var store Store
	err := r.pool.QueryRow(ctx, `
		UPDATE stores
		SET callback_url = $2, updated_at = $3
		WHERE id = $1::uuid AND deleted_at IS NULL
		RETURNING
			id,
			owner_user_id,
			name,
			slug,
			status,
			callback_url,
			current_balance::text,
			low_balance_threshold::text,
			(
				SELECT COUNT(*)
				FROM store_staff ss
				WHERE ss.store_id = stores.id
			)::int,
			created_at,
			updated_at,
			deleted_at
	`, params.StoreID, params.CallbackURL, params.OccurredAt).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.Name,
		&store.Slug,
		&store.Status,
		&store.CallbackURL,
		&store.CurrentBalance,
		&store.LowBalanceThreshold,
		&store.StaffCount,
		&store.CreatedAt,
		&store.UpdatedAt,
		&store.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Store{}, ErrNotFound
		}

		return Store{}, fmt.Errorf("update store callback url: %w", err)
	}

	return store, nil
}

func (r *Repository) CreateEmployee(ctx context.Context, params CreateEmployeeParams) (StaffUser, error) {
	var user StaffUser
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
		VALUES ($1, $2, $3, 'karyawan', TRUE, $4::uuid, $5, $5)
		RETURNING
			id,
			email,
			username,
			role,
			created_by_user_id,
			created_at,
			last_login_at
	`, params.Email, params.Username, params.PasswordHash, params.OwnerUserID, params.OccurredAt).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.Role,
		&user.CreatedByUserID,
		&user.CreatedAt,
		&user.LastLoginAt,
	)
	if err != nil {
		if duplicateIdentity(err) {
			return StaffUser{}, ErrDuplicateIdentity
		}

		return StaffUser{}, fmt.Errorf("create employee: %w", err)
	}

	return user, nil
}

func (r *Repository) ListEmployeesByOwner(ctx context.Context, ownerUserID string) ([]StaffUser, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			email,
			username,
			role,
			created_by_user_id,
			created_at,
			last_login_at
		FROM users
		WHERE role = 'karyawan' AND created_by_user_id = $1::uuid
		ORDER BY created_at DESC
	`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list employees by owner: %w", err)
	}
	defer rows.Close()

	var users []StaffUser
	for rows.Next() {
		var user StaffUser
		if err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.Username,
			&user.Role,
			&user.CreatedByUserID,
			&user.CreatedAt,
			&user.LastLoginAt,
		); err != nil {
			return nil, fmt.Errorf("scan employee: %w", err)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate employees: %w", err)
	}

	return users, nil
}

func (r *Repository) ListEmployeeDirectoryByOwner(ctx context.Context, ownerUserID string, filter ListEmployeesFilter) (StaffUserPage, error) {
	whereClause, args := buildEmployeeDirectoryWhere(ownerUserID, filter)

	var totalCount int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int
		FROM users
		WHERE `+whereClause, args...).Scan(&totalCount); err != nil {
		return StaffUserPage{}, fmt.Errorf("count employee directory: %w", err)
	}

	listArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			email,
			username,
			role,
			created_by_user_id,
			created_at,
			last_login_at,
			NULL::timestamptz
		FROM users
		WHERE `+whereClause+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)+1)+`
		OFFSET $`+fmt.Sprint(len(args)+2), listArgs...)
	if err != nil {
		return StaffUserPage{}, fmt.Errorf("list employee directory: %w", err)
	}
	defer rows.Close()

	users, err := collectStaffUsers(rows)
	if err != nil {
		return StaffUserPage{}, err
	}

	return StaffUserPage{
		Items:      users,
		TotalCount: totalCount,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
	}, nil
}

func (r *Repository) GetEmployeeByID(ctx context.Context, userID string) (StaffUser, error) {
	var user StaffUser
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			email,
			username,
			role,
			created_by_user_id,
			created_at,
			last_login_at
		FROM users
		WHERE id = $1::uuid
		LIMIT 1
	`, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.Role,
		&user.CreatedByUserID,
		&user.CreatedAt,
		&user.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StaffUser{}, ErrEmployeeNotFound
		}

		return StaffUser{}, fmt.Errorf("get employee by id: %w", err)
	}

	return user, nil
}

func (r *Repository) ListStoreStaff(ctx context.Context, storeID string) ([]StaffUser, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			u.id,
			u.email,
			u.username,
			u.role,
			u.created_by_user_id,
			u.created_at,
			u.last_login_at
		FROM store_staff ss
		JOIN users u ON u.id = ss.user_id
		WHERE ss.store_id = $1::uuid
		ORDER BY ss.created_at DESC
	`, storeID)
	if err != nil {
		return nil, fmt.Errorf("list store staff: %w", err)
	}
	defer rows.Close()

	var staff []StaffUser
	for rows.Next() {
		var user StaffUser
		if err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.Username,
			&user.Role,
			&user.CreatedByUserID,
			&user.CreatedAt,
			&user.LastLoginAt,
		); err != nil {
			return nil, fmt.Errorf("scan store staff: %w", err)
		}

		staff = append(staff, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate store staff: %w", err)
	}

	return staff, nil
}

func (r *Repository) ListStoreStaffPage(ctx context.Context, filter ListStoreStaffFilter) (StaffUserPage, error) {
	whereClause, args := buildStoreStaffWhere(filter)

	var totalCount int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int
		FROM store_staff ss
		INNER JOIN users u ON u.id = ss.user_id
		WHERE `+whereClause, args...).Scan(&totalCount); err != nil {
		return StaffUserPage{}, fmt.Errorf("count store staff directory: %w", err)
	}

	listArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	rows, err := r.pool.Query(ctx, `
		SELECT
			u.id,
			u.email,
			u.username,
			u.role,
			u.created_by_user_id,
			u.created_at,
			u.last_login_at,
			ss.created_at
		FROM store_staff ss
		INNER JOIN users u ON u.id = ss.user_id
		WHERE `+whereClause+`
		ORDER BY ss.created_at DESC, u.id DESC
		LIMIT $`+fmt.Sprint(len(args)+1)+`
		OFFSET $`+fmt.Sprint(len(args)+2), listArgs...)
	if err != nil {
		return StaffUserPage{}, fmt.Errorf("list store staff directory: %w", err)
	}
	defer rows.Close()

	staff, err := collectStaffUsers(rows)
	if err != nil {
		return StaffUserPage{}, err
	}

	return StaffUserPage{
		Items:      staff,
		TotalCount: totalCount,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
	}, nil
}

func (r *Repository) AssignStaff(ctx context.Context, params AssignStaffParams) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO store_staff (
			store_id,
			user_id,
			created_by_owner_id,
			created_at
		)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
	`, params.StoreID, params.UserID, params.CreatedByOwnerID, params.OccurredAt)
	if err != nil {
		if duplicateStaff(err) {
			return ErrDuplicateStaff
		}

		return fmt.Errorf("assign staff to store: %w", err)
	}

	return nil
}

func (r *Repository) UnassignStaff(ctx context.Context, storeID string, userID string) error {
	commandTag, err := r.pool.Exec(ctx, `
		DELETE FROM store_staff
		WHERE store_id = $1::uuid AND user_id = $2::uuid
	`, storeID, userID)
	if err != nil {
		return fmt.Errorf("unassign staff from store: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) InsertAuditLog(
	ctx context.Context,
	actorUserID *string,
	actorRole string,
	storeID *string,
	action string,
	targetType string,
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
			store_id,
			action,
			target_type,
			target_id,
			payload_masked,
			ip_address,
			user_agent,
			created_at
		)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6::uuid, $7::jsonb, $8, $9, $10)
	`, actorUserID, actorRole, storeID, action, targetType, targetID, string(encoded), nullableStringValue(ipAddress), nullableStringValue(userAgent), occurredAt)
	if err != nil {
		return fmt.Errorf("insert store audit log: %w", err)
	}

	return nil
}

type storeDirectoryQuery struct {
	joins   []string
	clauses []string
	args    []any
}

func (r *Repository) listStoreDirectory(ctx context.Context, query storeDirectoryQuery, filter ListStoreDirectoryFilter) (StorePage, error) {
	whereClause, args := buildStoreDirectoryWhere(query, filter)
	joinClause := strings.Join(query.joins, "\n")

	var summary StoreDirectorySummary
	summarySQL := `
		SELECT
			count(*)::int AS total_count,
			COALESCE(count(*) FILTER (WHERE s.status = 'active'), 0)::int AS active_count,
			COALESCE(count(*) FILTER (WHERE s.status = 'inactive'), 0)::int AS inactive_count,
			COALESCE(count(*) FILTER (WHERE s.status = 'banned'), 0)::int AS banned_count,
			COALESCE(count(*) FILTER (WHERE s.status = 'deleted'), 0)::int AS deleted_count,
			COALESCE(count(*) FILTER (
				WHERE s.low_balance_threshold IS NOT NULL
					AND s.current_balance <= s.low_balance_threshold
			), 0)::int AS low_balance_count
		FROM stores s
	`
	if joinClause != "" {
		summarySQL += joinClause + "\n"
	}
	summarySQL += `WHERE ` + whereClause

	if err := r.pool.QueryRow(ctx, summarySQL, args...).Scan(
		&summary.TotalCount,
		&summary.ActiveCount,
		&summary.InactiveCount,
		&summary.BannedCount,
		&summary.DeletedCount,
		&summary.LowBalanceCount,
	); err != nil {
		return StorePage{}, fmt.Errorf("summarize store directory: %w", err)
	}

	listArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	listSQL := `
		SELECT
			s.id,
			s.owner_user_id,
			s.name,
			s.slug,
			s.status,
			s.callback_url,
			s.current_balance::text,
			s.low_balance_threshold::text,
			(
				SELECT COUNT(*)
				FROM store_staff ss
				WHERE ss.store_id = s.id
			)::int AS staff_count,
			s.created_at,
			s.updated_at,
			s.deleted_at
		FROM stores s
	`
	if joinClause != "" {
		listSQL += joinClause + "\n"
	}
	listSQL += `WHERE ` + whereClause + `
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $` + fmt.Sprint(len(args)+1) + `
		OFFSET $` + fmt.Sprint(len(args)+2)

	rows, err := r.pool.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return StorePage{}, fmt.Errorf("list store directory: %w", err)
	}
	defer rows.Close()

	items, err := collectStores(rows)
	if err != nil {
		return StorePage{}, err
	}

	return StorePage{
		Items:   items,
		Summary: summary,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	}, nil
}

func collectStores(rows pgx.Rows) ([]Store, error) {
	var stores []Store
	for rows.Next() {
		var store Store
		if err := rows.Scan(
			&store.ID,
			&store.OwnerUserID,
			&store.Name,
			&store.Slug,
			&store.Status,
			&store.CallbackURL,
			&store.CurrentBalance,
			&store.LowBalanceThreshold,
			&store.StaffCount,
			&store.CreatedAt,
			&store.UpdatedAt,
			&store.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan store: %w", err)
		}

		stores = append(stores, store)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stores: %w", err)
	}

	return stores, nil
}

func collectStaffUsers(rows pgx.Rows) ([]StaffUser, error) {
	var users []StaffUser
	for rows.Next() {
		var user StaffUser
		if err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.Username,
			&user.Role,
			&user.CreatedByUserID,
			&user.CreatedAt,
			&user.LastLoginAt,
			&user.AssignedAt,
		); err != nil {
			return nil, fmt.Errorf("scan staff user: %w", err)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate staff users: %w", err)
	}

	return users, nil
}

type repositoryLowBalanceSweepLock struct {
	conn *pgxpool.Conn
}

func (l *repositoryLowBalanceSweepLock) Unlock(ctx context.Context) error {
	if l == nil || l.conn == nil {
		return nil
	}

	var unlocked bool
	err := l.conn.QueryRow(ctx, `
		SELECT pg_advisory_unlock($1, $2)
	`, lowBalanceSweepLockNamespace, 1).Scan(&unlocked)
	l.conn.Release()
	l.conn = nil
	if err != nil {
		return fmt.Errorf("unlock low balance sweep advisory lock: %w", err)
	}
	if !unlocked {
		return fmt.Errorf("unlock low balance sweep advisory lock: lock not held")
	}

	return nil
}

func duplicateSlug(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "stores_slug_key"
}

func duplicateIdentity(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}

	return pgErr.ConstraintName == "users_email_key" || pgErr.ConstraintName == "users_username_key"
}

func buildStoreDirectoryWhere(query storeDirectoryQuery, filter ListStoreDirectoryFilter) (string, []any) {
	clauses := append([]string{}, query.clauses...)
	args := append([]any{}, query.args...)
	next := len(args) + 1

	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		placeholder := fmt.Sprintf("$%d", next)
		clauses = append(clauses,
			"(s.name ILIKE "+placeholder+" OR s.slug::text ILIKE "+placeholder+" OR s.callback_url ILIKE "+placeholder+")",
		)
		next++
	}

	if filter.Status != nil {
		args = append(args, *filter.Status)
		clauses = append(clauses, fmt.Sprintf("s.status = $%d", next))
		next++
	}

	switch filter.LowBalanceState {
	case LowBalanceStateOnlyLow:
		clauses = append(clauses, "s.low_balance_threshold IS NOT NULL AND s.current_balance <= s.low_balance_threshold")
	case LowBalanceStateOnlyHealth:
		clauses = append(clauses, "(s.low_balance_threshold IS NULL OR s.current_balance > s.low_balance_threshold)")
	}

	if filter.CreatedFrom != nil {
		args = append(args, *filter.CreatedFrom)
		clauses = append(clauses, fmt.Sprintf("s.created_at >= $%d", next))
		next++
	}

	if filter.CreatedTo != nil {
		args = append(args, *filter.CreatedTo)
		clauses = append(clauses, fmt.Sprintf("s.created_at <= $%d", next))
	}

	return strings.Join(clauses, " AND "), args
}

func buildEmployeeDirectoryWhere(ownerUserID string, filter ListEmployeesFilter) (string, []any) {
	clauses := []string{
		"role = 'karyawan'",
		"created_by_user_id = $1::uuid",
	}
	args := []any{ownerUserID}
	next := 2

	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		placeholder := fmt.Sprintf("$%d", next)
		clauses = append(clauses, "(username::text ILIKE "+placeholder+" OR email::text ILIKE "+placeholder+")")
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

func buildStoreStaffWhere(filter ListStoreStaffFilter) (string, []any) {
	clauses := []string{
		"ss.store_id = $1::uuid",
	}
	args := []any{filter.StoreID}
	next := 2

	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		placeholder := fmt.Sprintf("$%d", next)
		clauses = append(clauses, "(u.username::text ILIKE "+placeholder+" OR u.email::text ILIKE "+placeholder+")")
		next++
	}

	if filter.AssignedFrom != nil {
		args = append(args, *filter.AssignedFrom)
		clauses = append(clauses, fmt.Sprintf("ss.created_at >= $%d", next))
		next++
	}

	if filter.AssignedTo != nil {
		args = append(args, *filter.AssignedTo)
		clauses = append(clauses, fmt.Sprintf("ss.created_at <= $%d", next))
	}

	return strings.Join(clauses, " AND "), args
}

func duplicateStaff(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "store_staff_store_id_user_id_unique"
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}

	return nullableStringValue(*value)
}

func nullableStringValue(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return trimmed
}
