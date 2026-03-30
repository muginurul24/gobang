package storemembers

import (
	"context"
	"errors"
	"fmt"
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

func (r *Repository) GetStoreScope(ctx context.Context, storeID string) (StoreScope, error) {
	var store StoreScope
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_user_id, name, slug, deleted_at
		FROM stores
		WHERE id = $1
		LIMIT 1
	`, storeID).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.Name,
		&store.Slug,
		&store.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreScope{}, ErrNotFound
		}

		return StoreScope{}, fmt.Errorf("get store scope: %w", err)
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

func (r *Repository) ListStoreMembers(ctx context.Context, storeID string) ([]StoreMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			store_id,
			real_username,
			upstream_user_code,
			status,
			created_at,
			updated_at
		FROM store_members
		WHERE store_id = $1
		ORDER BY created_at DESC
	`, storeID)
	if err != nil {
		return nil, fmt.Errorf("list store members: %w", err)
	}
	defer rows.Close()

	var members []StoreMember
	for rows.Next() {
		var member StoreMember
		if err := rows.Scan(
			&member.ID,
			&member.StoreID,
			&member.RealUsername,
			&member.UpstreamUserCode,
			&member.Status,
			&member.CreatedAt,
			&member.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan store member: %w", err)
		}

		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate store members: %w", err)
	}

	return members, nil
}

func (r *Repository) CreateStoreMember(ctx context.Context, params CreateStoreMemberParams) (StoreMember, error) {
	var member StoreMember
	err := r.pool.QueryRow(ctx, `
		INSERT INTO store_members (
			store_id,
			real_username,
			upstream_user_code,
			status,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING
			id,
			store_id,
			real_username,
			upstream_user_code,
			status,
			created_at,
			updated_at
	`, params.StoreID, params.RealUsername, params.UpstreamUserCode, params.Status, params.OccurredAt).Scan(
		&member.ID,
		&member.StoreID,
		&member.RealUsername,
		&member.UpstreamUserCode,
		&member.Status,
		&member.CreatedAt,
		&member.UpdatedAt,
	)
	if err != nil {
		switch {
		case duplicateConstraint(err, "store_members_store_id_real_username_unique"):
			return StoreMember{}, ErrDuplicateRealUsername
		case duplicateConstraint(err, "store_members_upstream_user_code_unique"):
			return StoreMember{}, ErrDuplicateUpstreamUserCode
		}

		return StoreMember{}, fmt.Errorf("create store member: %w", err)
	}

	return member, nil
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
	_, err := r.pool.Exec(ctx, `
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
	`, actorUserID, actorRole, storeID, action, targetType, targetID, toJSON(payload), nullableString(ipAddress), nullableString(userAgent), occurredAt)
	if err != nil {
		return fmt.Errorf("insert store member audit log: %w", err)
	}

	return nil
}

func duplicateConstraint(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
