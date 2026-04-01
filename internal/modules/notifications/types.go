package notifications

import "time"

type ScopeType string

const (
	ScopeStore  ScopeType = "store"
	ScopeUser   ScopeType = "user"
	ScopeRole   ScopeType = "role"
	ScopeGlobal ScopeType = "global"
)

type Notification struct {
	ID        string     `json:"id"`
	ScopeType ScopeType  `json:"scope_type"`
	ScopeID   string     `json:"scope_id"`
	EventType string     `json:"event_type"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type CreateParams struct {
	ScopeType ScopeType
	ScopeID   string
	EventType string
	Title     string
	Body      string
}

type MarkReadParams struct {
	ID        string
	ScopeType ScopeType
	ScopeID   string
}

type ListParams struct {
	ScopeType   ScopeType
	ScopeID     string
	Query       string
	ReadState   string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Limit       int
	Offset      int
}

type ListResult struct {
	Items      []Notification `json:"items"`
	TotalCount int            `json:"total_count"`
}

// Emitter is the interface exposed to domain modules for creating notifications.
// Domain services depend on this interface so they do not import the full
// notifications package.
type Emitter interface {
	Emit(params CreateParams) error
}
