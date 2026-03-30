package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListActiveRecoveryCodes(ctx context.Context, userID string) ([]RecoveryCodeRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, code_hash
		FROM user_recovery_codes
		WHERE user_id = $1 AND used_at IS NULL
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list active recovery codes: %w", err)
	}
	defer rows.Close()

	var records []RecoveryCodeRecord
	for rows.Next() {
		var record RecoveryCodeRecord
		if err := rows.Scan(&record.ID, &record.CodeHash); err != nil {
			return nil, fmt.Errorf("scan recovery code: %w", err)
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recovery codes: %w", err)
	}

	return records, nil
}

func (r *Repository) UseRecoveryCode(ctx context.Context, recoveryCodeID string, occurredAt time.Time) error {
	commandTag, err := r.pool.Exec(ctx, `
		UPDATE user_recovery_codes
		SET used_at = $2
		WHERE id = $1 AND used_at IS NULL
	`, recoveryCodeID, occurredAt)
	if err != nil {
		return fmt.Errorf("use recovery code: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) EnableTOTP(ctx context.Context, params EnableTOTPParams) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin enable totp transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET totp_enabled = TRUE, totp_secret_encrypted = $2, updated_at = $3
		WHERE id = $1
	`, params.UserID, params.EncryptedSecret, params.OccurredAt); err != nil {
		return fmt.Errorf("update totp state: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM user_recovery_codes WHERE user_id = $1`, params.UserID); err != nil {
		return fmt.Errorf("delete old recovery codes: %w", err)
	}

	for _, recoveryCodeHash := range params.RecoveryCodeHashes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_recovery_codes (user_id, code_hash, created_at)
			VALUES ($1, $2, $3)
		`, params.UserID, recoveryCodeHash, params.OccurredAt); err != nil {
			return fmt.Errorf("insert recovery code: %w", err)
		}
	}

	if err := insertAuditLogTx(ctx, tx, AuditLogParams{
		ActorUserID: &params.UserID,
		ActorRole:   string(params.ActorRole),
		TargetType:  "user",
		TargetID:    &params.UserID,
		Action:      "auth.2fa_enabled",
		Payload: map[string]any{
			"recovery_code_count": len(params.RecoveryCodeHashes),
		},
		IPAddress:  params.IPAddress,
		UserAgent:  params.UserAgent,
		OccurredAt: params.OccurredAt,
	}); err != nil {
		return fmt.Errorf("insert 2fa enable audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit enable totp transaction: %w", err)
	}

	return nil
}

func (r *Repository) DisableTOTP(ctx context.Context, params DisableTOTPParams) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin disable totp transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET totp_enabled = FALSE, totp_secret_encrypted = NULL, updated_at = $2
		WHERE id = $1
	`, params.UserID, params.OccurredAt); err != nil {
		return fmt.Errorf("clear totp state: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM user_recovery_codes WHERE user_id = $1`, params.UserID); err != nil {
		return fmt.Errorf("delete recovery codes: %w", err)
	}

	if err := insertAuditLogTx(ctx, tx, AuditLogParams{
		ActorUserID: &params.UserID,
		ActorRole:   string(params.ActorRole),
		TargetType:  "user",
		TargetID:    &params.UserID,
		Action:      "auth.2fa_disabled",
		Payload: map[string]any{
			"source": "self_service",
		},
		IPAddress:  params.IPAddress,
		UserAgent:  params.UserAgent,
		OccurredAt: params.OccurredAt,
	}); err != nil {
		return fmt.Errorf("insert 2fa disable audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit disable totp transaction: %w", err)
	}

	return nil
}

func (r *Repository) UpdateIPAllowlist(ctx context.Context, params UpdateIPAllowlistParams) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE users
		SET ip_allowlist = $2, updated_at = $3
		WHERE id = $1
	`, params.UserID, nullableStringPtr(params.IPAllowlist), params.OccurredAt); err != nil {
		return fmt.Errorf("update ip allowlist: %w", err)
	}

	action := "auth.ip_allowlist_cleared"
	if params.IPAllowlist != nil {
		action = "auth.ip_allowlist_updated"
	}

	if err := r.InsertAuditLog(ctx, AuditLogParams{
		ActorUserID: &params.UserID,
		ActorRole:   string(params.ActorRole),
		TargetType:  "user",
		TargetID:    &params.UserID,
		Action:      action,
		Payload: map[string]any{
			"allowlist": params.IPAllowlist,
		},
		IPAddress:  params.IPAddress,
		UserAgent:  params.UserAgent,
		OccurredAt: params.OccurredAt,
	}); err != nil {
		return fmt.Errorf("insert ip allowlist audit log: %w", err)
	}

	return nil
}

func (r *Repository) InsertAuditLog(ctx context.Context, params AuditLogParams) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (
			actor_user_id,
			actor_role,
			target_type,
			target_id,
			action,
			payload_masked,
			ip_address,
			user_agent,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
	`, params.ActorUserID, params.ActorRole, params.TargetType, params.TargetID, params.Action, toJSONPayload(params.Payload), nullableString(params.IPAddress), nullableString(params.UserAgent), params.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

func insertAuditLogTx(ctx context.Context, tx pgx.Tx, params AuditLogParams) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			actor_user_id,
			actor_role,
			target_type,
			target_id,
			action,
			payload_masked,
			ip_address,
			user_agent,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
	`, params.ActorUserID, params.ActorRole, params.TargetType, params.TargetID, params.Action, toJSONPayload(params.Payload), nullableString(params.IPAddress), nullableString(params.UserAgent), params.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert audit log in transaction: %w", err)
	}

	return nil
}

func nullableStringPtr(value *string) any {
	if value == nil {
		return nil
	}

	return nullableString(*value)
}

func toJSONPayload(payload map[string]any) string {
	if payload == nil {
		return "{}"
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}

	return string(encoded)
}
