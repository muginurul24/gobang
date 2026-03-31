package auth

import (
	"context"
	"errors"
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

func (r *Repository) FindUserByLogin(ctx context.Context, login string) (User, error) {
	const query = `
		SELECT id, email, username, password_hash, role, is_active, totp_enabled, COALESCE(totp_secret_encrypted, ''), host(ip_allowlist), last_login_at
		FROM users
		WHERE email = $1 OR username = $1
		LIMIT 1
	`

	var user User
	err := r.pool.QueryRow(ctx, query, login).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.IsActive,
		&user.TOTPEnabled,
		&user.TOTPSecretEncrypted,
		&user.IPAllowlist,
		&user.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}

		return User{}, fmt.Errorf("find user by login: %w", err)
	}

	return user, nil
}

func (r *Repository) FindUserByID(ctx context.Context, userID string) (User, error) {
	const query = `
		SELECT id, email, username, password_hash, role, is_active, totp_enabled, COALESCE(totp_secret_encrypted, ''), host(ip_allowlist), last_login_at
		FROM users
		WHERE id = $1
		LIMIT 1
	`

	var user User
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.IsActive,
		&user.TOTPEnabled,
		&user.TOTPSecretEncrypted,
		&user.IPAllowlist,
		&user.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}

		return User{}, fmt.Errorf("find user by id: %w", err)
	}

	return user, nil
}

func (r *Repository) ReplaceUserSessions(ctx context.Context, params ReplaceUserSessionsParams) (ReplaceUserSessionsResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReplaceUserSessionsResult{}, fmt.Errorf("begin login transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		UPDATE user_sessions
		SET revoked_at = $2, updated_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL
		RETURNING session_jti
	`, params.UserID, params.OccurredAt)
	if err != nil {
		return ReplaceUserSessionsResult{}, fmt.Errorf("revoke active sessions: %w", err)
	}

	var revokedJTIs []string
	for rows.Next() {
		var sessionJTI string
		if scanErr := rows.Scan(&sessionJTI); scanErr != nil {
			rows.Close()
			return ReplaceUserSessionsResult{}, fmt.Errorf("scan revoked session jti: %w", scanErr)
		}

		revokedJTIs = append(revokedJTIs, sessionJTI)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ReplaceUserSessionsResult{}, fmt.Errorf("iterate revoked sessions: %w", err)
	}

	var session SessionRecord
	err = tx.QueryRow(ctx, `
		INSERT INTO user_sessions (
			user_id,
			session_jti,
			refresh_hash,
			ip_address,
			user_agent,
			expires_at,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING id, user_id, session_jti, refresh_hash, expires_at, revoked_at, created_at, updated_at
	`,
		params.UserID,
		params.SessionJTI,
		params.RefreshHash,
		requiredIPAddress(params.IPAddress),
		requiredUserAgent(params.UserAgent),
		params.ExpiresAt,
		params.OccurredAt,
	).Scan(
		&session.ID,
		&session.UserID,
		&session.SessionJTI,
		&session.RefreshHash,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return ReplaceUserSessionsResult{}, fmt.Errorf("insert user session: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET last_login_at = $2, updated_at = $2
		WHERE id = $1
	`, params.UserID, params.OccurredAt); err != nil {
		return ReplaceUserSessionsResult{}, fmt.Errorf("update last_login_at: %w", err)
	}

	if _, err := tx.Exec(ctx, `
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
		VALUES (
			$1,
			$2,
			'user',
			$1,
			'auth.login_success',
			jsonb_build_object('source', 'dashboard'),
			$3,
			$4,
			$5
		)
	`, params.UserID, string(params.ActorRole), nullableString(params.IPAddress), nullableString(params.UserAgent), params.OccurredAt); err != nil {
		return ReplaceUserSessionsResult{}, fmt.Errorf("insert login audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ReplaceUserSessionsResult{}, fmt.Errorf("commit login transaction: %w", err)
	}

	return ReplaceUserSessionsResult{
		Session:     session,
		RevokedJTIs: revokedJTIs,
	}, nil
}

func (r *Repository) GetSessionForRefresh(ctx context.Context, sessionJTI string) (SessionWithUser, error) {
	const query = `
		SELECT
			s.id,
			s.user_id,
			s.session_jti,
			s.refresh_hash,
			s.expires_at,
			s.revoked_at,
			s.created_at,
			s.updated_at,
			u.id,
			u.email,
			u.username,
			u.password_hash,
			u.role,
			u.is_active,
			u.totp_enabled,
			COALESCE(u.totp_secret_encrypted, ''),
			host(u.ip_allowlist),
			u.last_login_at
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.session_jti = $1
		LIMIT 1
	`

	var result SessionWithUser
	err := r.pool.QueryRow(ctx, query, sessionJTI).Scan(
		&result.Session.ID,
		&result.Session.UserID,
		&result.Session.SessionJTI,
		&result.Session.RefreshHash,
		&result.Session.ExpiresAt,
		&result.Session.RevokedAt,
		&result.Session.CreatedAt,
		&result.Session.UpdatedAt,
		&result.User.ID,
		&result.User.Email,
		&result.User.Username,
		&result.User.PasswordHash,
		&result.User.Role,
		&result.User.IsActive,
		&result.User.TOTPEnabled,
		&result.User.TOTPSecretEncrypted,
		&result.User.IPAllowlist,
		&result.User.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SessionWithUser{}, ErrNotFound
		}

		return SessionWithUser{}, fmt.Errorf("get session for refresh: %w", err)
	}

	return result, nil
}

func (r *Repository) RotateSession(ctx context.Context, params RotateSessionParams) (SessionRecord, error) {
	const query = `
		UPDATE user_sessions
		SET
			session_jti = $2,
			refresh_hash = $3,
			ip_address = $4,
			user_agent = $5,
			expires_at = $6,
			updated_at = $7
		WHERE session_jti = $1 AND revoked_at IS NULL
		RETURNING id, user_id, session_jti, refresh_hash, expires_at, revoked_at, created_at, updated_at
	`

	var session SessionRecord
	err := r.pool.QueryRow(
		ctx,
		query,
		params.OldSessionJTI,
		params.NewSessionJTI,
		params.RefreshHash,
		requiredIPAddress(params.IPAddress),
		requiredUserAgent(params.UserAgent),
		params.ExpiresAt,
		params.OccurredAt,
	).Scan(
		&session.ID,
		&session.UserID,
		&session.SessionJTI,
		&session.RefreshHash,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SessionRecord{}, ErrNotFound
		}

		return SessionRecord{}, fmt.Errorf("rotate session: %w", err)
	}

	return session, nil
}

func (r *Repository) RevokeSession(ctx context.Context, params RevokeSessionParams) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin logout transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	commandTag, err := tx.Exec(ctx, `
		UPDATE user_sessions
		SET revoked_at = $3, updated_at = $3
		WHERE user_id = $1 AND session_jti = $2 AND revoked_at IS NULL
	`, params.UserID, params.SessionJTI, params.OccurredAt)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	if commandTag.RowsAffected() > 0 {
		if _, err := tx.Exec(ctx, `
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
			VALUES (
				$1,
				$2,
				'user',
				$1,
				'auth.logout',
				jsonb_build_object('scope', 'current_session'),
				$3,
				$4,
				$5
			)
		`, params.UserID, string(params.ActorRole), nullableString(params.IPAddress), nullableString(params.UserAgent), params.OccurredAt); err != nil {
			return fmt.Errorf("insert logout audit log: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit logout transaction: %w", err)
	}

	return nil
}

func (r *Repository) RevokeAllSessions(ctx context.Context, params RevokeAllSessionsParams) ([]string, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin logout all transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		UPDATE user_sessions
		SET revoked_at = $2, updated_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL
		RETURNING session_jti
	`, params.UserID, params.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("revoke all sessions: %w", err)
	}

	var revokedJTIs []string
	for rows.Next() {
		var sessionJTI string
		if scanErr := rows.Scan(&sessionJTI); scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("scan revoked session from logout all: %w", scanErr)
		}

		revokedJTIs = append(revokedJTIs, sessionJTI)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate revoked sessions from logout all: %w", err)
	}

	if len(revokedJTIs) > 0 {
		if _, err := tx.Exec(ctx, `
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
			VALUES (
				$1,
				$2,
				'user',
				$1,
				'auth.logout_all',
				jsonb_build_object('scope', 'all_sessions'),
				$3,
				$4,
				$5
			)
		`, params.UserID, string(params.ActorRole), nullableString(params.IPAddress), nullableString(params.UserAgent), params.OccurredAt); err != nil {
			return nil, fmt.Errorf("insert logout all audit log: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit logout all transaction: %w", err)
	}

	return revokedJTIs, nil
}

func (r *Repository) PruneSessionsExpiredBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	commandTag, err := r.pool.Exec(ctx, `
		DELETE FROM user_sessions
		WHERE expires_at <= $1
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune expired sessions: %w", err)
	}

	return commandTag.RowsAffected(), nil
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return trimmed
}

func requiredIPAddress(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "0.0.0.0"
	}

	return trimmed
}

func requiredUserAgent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}

	return trimmed
}
