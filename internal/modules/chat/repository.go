package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RepositoryContract interface {
	ListMessages(ctx context.Context, cutoff time.Time, filter ListMessagesFilter) (ListMessagesResult, error)
	CreateMessage(ctx context.Context, params CreateMessageParams) (Message, error)
	DeleteMessage(ctx context.Context, params DeleteMessageParams) (Message, error)
	PruneMessages(ctx context.Context, cutoff time.Time) (int64, error)
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListMessages(ctx context.Context, cutoff time.Time, filter ListMessagesFilter) (ListMessagesResult, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	clauses := []string{
		"m.deleted_at IS NULL",
		"m.created_at >= $1",
	}
	args := []any{cutoff}

	if role := strings.TrimSpace(filter.Role); role != "" {
		args = append(args, role)
		clauses = append(clauses, fmt.Sprintf("u.role = $%d", len(args)))
	}

	if query := strings.TrimSpace(filter.Query); query != "" {
		args = append(args, "%"+query+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		clauses = append(clauses, fmt.Sprintf("(u.username ILIKE %s OR m.body ILIKE %s)", placeholder, placeholder))
	}

	if filter.CreatedFrom != nil {
		args = append(args, *filter.CreatedFrom)
		clauses = append(clauses, fmt.Sprintf("m.created_at >= $%d", len(args)))
	}

	if filter.CreatedTo != nil {
		args = append(args, *filter.CreatedTo)
		clauses = append(clauses, fmt.Sprintf("m.created_at <= $%d", len(args)))
	}

	whereClause := strings.Join(clauses, " AND ")

	var totalCount int
	countQuery := `
		SELECT count(*)
		FROM chat_messages m
		INNER JOIN users u ON u.id = m.sender_user_id
		WHERE ` + whereClause
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return ListMessagesResult{}, fmt.Errorf("count chat messages: %w", err)
	}

	args = append(args, limit, filter.Offset)
	rows, err := r.pool.Query(ctx, `
		SELECT
			m.id::text,
			m.sender_user_id::text,
			u.username AS sender_username,
			u.role AS sender_role,
			m.body,
			m.deleted_by_dev_user_id::text,
			m.deleted_at,
			m.created_at
		FROM chat_messages m
		INNER JOIN users u ON u.id = m.sender_user_id
		WHERE `+whereClause+`
		ORDER BY m.created_at DESC
		LIMIT $`+fmt.Sprintf("%d", len(args)-1)+` OFFSET $`+fmt.Sprintf("%d", len(args)))
	if err != nil {
		return ListMessagesResult{}, fmt.Errorf("list chat messages: %w", err)
	}
	defer rows.Close()

	messages := make([]Message, 0, limit)
	for rows.Next() {
		var message Message
		if err := rows.Scan(
			&message.ID,
			&message.SenderUserID,
			&message.SenderUsername,
			&message.SenderRole,
			&message.Body,
			&message.DeletedByDevUserID,
			&message.DeletedAt,
			&message.CreatedAt,
		); err != nil {
			return ListMessagesResult{}, fmt.Errorf("scan chat message: %w", err)
		}
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return ListMessagesResult{}, fmt.Errorf("iterate chat messages: %w", err)
	}

	return ListMessagesResult{
		Items:      messages,
		TotalCount: totalCount,
	}, nil
}

func (r *Repository) CreateMessage(ctx context.Context, params CreateMessageParams) (Message, error) {
	var message Message
	err := r.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO chat_messages (sender_user_id, body, created_at)
			VALUES ($1, $2, $3)
			RETURNING id, sender_user_id, body, deleted_by_dev_user_id, deleted_at, created_at
		)
		SELECT
			i.id::text,
			i.sender_user_id::text,
			u.username AS sender_username,
			u.role AS sender_role,
			i.body,
			i.deleted_by_dev_user_id::text,
			i.deleted_at,
			i.created_at
		FROM inserted i
		INNER JOIN users u ON u.id = i.sender_user_id
	`, params.SenderUserID, params.Body, params.CreatedAt).Scan(
		&message.ID,
		&message.SenderUserID,
		&message.SenderUsername,
		&message.SenderRole,
		&message.Body,
		&message.DeletedByDevUserID,
		&message.DeletedAt,
		&message.CreatedAt,
	)
	if err != nil {
		return Message{}, fmt.Errorf("create chat message: %w", err)
	}

	return message, nil
}

func (r *Repository) DeleteMessage(ctx context.Context, params DeleteMessageParams) (Message, error) {
	var message Message
	err := r.pool.QueryRow(ctx, `
		WITH updated AS (
			UPDATE chat_messages
			SET deleted_by_dev_user_id = $2::uuid, deleted_at = $3
			WHERE id = $1::uuid
				AND deleted_at IS NULL
			RETURNING id, sender_user_id, body, deleted_by_dev_user_id, deleted_at, created_at
		)
		SELECT
			u2.id::text,
			u2.sender_user_id::text,
			u.username AS sender_username,
			u.role AS sender_role,
			u2.body,
			u2.deleted_by_dev_user_id::text,
			u2.deleted_at,
			u2.created_at
		FROM updated u2
		INNER JOIN users u ON u.id = u2.sender_user_id
	`, strings.TrimSpace(params.MessageID), strings.TrimSpace(params.DeletedByDevUserID), params.DeletedAt).Scan(
		&message.ID,
		&message.SenderUserID,
		&message.SenderUsername,
		&message.SenderRole,
		&message.Body,
		&message.DeletedByDevUserID,
		&message.DeletedAt,
		&message.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrNotFound
		}

		return Message{}, fmt.Errorf("delete chat message: %w", err)
	}

	return message, nil
}

func (r *Repository) PruneMessages(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM chat_messages
		WHERE created_at < $1
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune chat messages: %w", err)
	}

	return tag.RowsAffected(), nil
}
