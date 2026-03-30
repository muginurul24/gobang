package chat

import "time"

type Message struct {
	ID                 string     `json:"id"`
	SenderUserID       string     `json:"sender_user_id"`
	SenderUsername     string     `json:"sender_username"`
	SenderRole         string     `json:"sender_role"`
	Body               string     `json:"body"`
	DeletedByDevUserID *string    `json:"deleted_by_dev_user_id,omitempty"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type SendMessageInput struct {
	Body string `json:"body"`
}

type CreateMessageParams struct {
	SenderUserID string
	Body         string
	CreatedAt    time.Time
}

type DeleteMessageParams struct {
	MessageID          string
	DeletedByDevUserID string
	DeletedAt          time.Time
}
