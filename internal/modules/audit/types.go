package audit

import (
	"encoding/json"
	"time"
)

type LogEntry struct {
	ID          string          `json:"id"`
	ActorUserID *string         `json:"actor_user_id"`
	ActorRole   string          `json:"actor_role"`
	StoreID     *string         `json:"store_id"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    *string         `json:"target_id"`
	Payload     json.RawMessage `json:"payload_masked"`
	IPAddress   *string         `json:"ip_address"`
	UserAgent   *string         `json:"user_agent"`
	CreatedAt   time.Time       `json:"created_at"`
}

type Filter struct {
	StoreID    *string
	Action     *string
	ActorRole  *string
	TargetType *string
	Limit      int
}
