package game

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

func (r *Repository) AuthenticateStore(ctx context.Context, tokenHash string) (StoreScope, error) {
	var store StoreScope
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			owner_user_id,
			name,
			slug,
			status,
			deleted_at
		FROM stores
		WHERE api_token_hash = $1
		LIMIT 1
	`, tokenHash).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.Name,
		&store.Slug,
		&store.Status,
		&store.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreScope{}, ErrUnauthorized
		}

		return StoreScope{}, fmt.Errorf("authenticate store token: %w", err)
	}

	return store, nil
}

func (r *Repository) FindStoreMemberByUsername(ctx context.Context, storeID string, username string) (StoreMember, error) {
	var member StoreMember
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			store_id,
			real_username,
			upstream_user_code,
			status,
			created_at,
			updated_at
		FROM store_members
		WHERE store_id = $1 AND real_username = $2
		LIMIT 1
	`, storeID, username).Scan(
		&member.ID,
		&member.StoreID,
		&member.RealUsername,
		&member.UpstreamUserCode,
		&member.Status,
		&member.CreatedAt,
		&member.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreMember{}, ErrNotFound
		}

		return StoreMember{}, fmt.Errorf("find store member by username: %w", err)
	}

	return member, nil
}

func (r *Repository) HasUpstreamUserCode(ctx context.Context, upstreamUserCode string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM store_members
			WHERE upstream_user_code = $1
		)
	`, upstreamUserCode).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check upstream user code: %w", err)
	}

	return exists, nil
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
			return StoreMember{}, ErrDuplicateUsername
		case duplicateConstraint(err, "store_members_upstream_user_code_unique"):
			return StoreMember{}, ErrDuplicateUpstreamUserCode
		}

		return StoreMember{}, fmt.Errorf("create game user mapping: %w", err)
	}

	return member, nil
}

func (r *Repository) InsertAuditLog(
	ctx context.Context,
	storeID string,
	action string,
	targetID *string,
	payload map[string]any,
	ipAddress string,
	userAgent string,
	occurredAt time.Time,
) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (
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
		VALUES ('store_api', $1, $2, 'store_member', $3, $4::jsonb, $5, $6, $7)
	`, storeID, action, targetID, toJSON(payload), nullableString(ipAddress), nullableString(userAgent), occurredAt)
	if err != nil {
		return fmt.Errorf("insert game audit log: %w", err)
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
