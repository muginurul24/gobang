package realtime

import (
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	platformrealtime "github.com/mugiew/onixggr/internal/platform/realtime"
)

const globalChatChannel = "global_chat"

type ConnectionSession struct {
	Subject          auth.Subject
	Channels         []string
	HeartbeatSeconds int
}

type HelloFrame struct {
	Kind                     string    `json:"kind"`
	ConnectionID             string    `json:"connection_id"`
	UserID                   string    `json:"user_id"`
	Role                     string    `json:"role"`
	Channels                 []string  `json:"channels"`
	HeartbeatIntervalSeconds int       `json:"heartbeat_interval_seconds"`
	ConnectedAt              time.Time `json:"connected_at"`
}

type EventFrame struct {
	Kind  string                 `json:"kind"`
	Event platformrealtime.Event `json:"event"`
}

type HeartbeatFrame struct {
	Kind   string    `json:"kind"`
	SentAt time.Time `json:"sent_at"`
}
