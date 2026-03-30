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
		WHERE s.owner_user_id = $1 AND s.deleted_at IS NULL
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
		WHERE ss.user_id = $1 AND s.deleted_at IS NULL
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
		WHERE s.id = $1
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
			WHERE store_id = $1 AND user_id = $2
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
		VALUES ($1, $2, $3, 'active', $4, $5, $6, $6)
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
		WHERE id = $1 AND deleted_at IS NULL
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
		WHERE id = $1 AND deleted_at IS NULL
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
		WHERE id = $1 AND deleted_at IS NULL
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
		WHERE id = $1 AND deleted_at IS NULL
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
		VALUES ($1, $2, $3, 'karyawan', TRUE, $4, $5, $5)
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
		WHERE role = 'karyawan' AND created_by_user_id = $1
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
		WHERE id = $1
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
		WHERE ss.store_id = $1
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

func (r *Repository) AssignStaff(ctx context.Context, params AssignStaffParams) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO store_staff (
			store_id,
			user_id,
			created_by_owner_id,
			created_at
		)
		VALUES ($1, $2, $3, $4)
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
		WHERE store_id = $1 AND user_id = $2
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
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)
	`, actorUserID, actorRole, storeID, action, targetType, targetID, string(encoded), nullableStringValue(ipAddress), nullableStringValue(userAgent), occurredAt)
	if err != nil {
		return fmt.Errorf("insert store audit log: %w", err)
	}

	return nil
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
