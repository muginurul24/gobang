package stores

import "time"

type StoreStatus string

const (
	StatusActive   StoreStatus = "active"
	StatusInactive StoreStatus = "inactive"
	StatusBanned   StoreStatus = "banned"
	StatusDeleted  StoreStatus = "deleted"
)

type Store struct {
	ID                  string      `json:"id"`
	OwnerUserID         string      `json:"owner_user_id"`
	Name                string      `json:"name"`
	Slug                string      `json:"slug"`
	Status              StoreStatus `json:"status"`
	APIToken            *string     `json:"api_token,omitempty"`
	CallbackURL         string      `json:"callback_url"`
	CurrentBalance      string      `json:"current_balance"`
	LowBalanceThreshold *string     `json:"low_balance_threshold"`
	StaffCount          int         `json:"staff_count"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	DeletedAt           *time.Time  `json:"deleted_at"`
}

type StaffUser struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Username        string     `json:"username"`
	Role            string     `json:"role"`
	CreatedByUserID *string    `json:"created_by_user_id"`
	CreatedAt       time.Time  `json:"created_at"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	AssignedAt      *time.Time `json:"assigned_at,omitempty"`
}

type StoreToken struct {
	Token string `json:"token"`
}

type LowBalanceState string

const (
	LowBalanceStateAll        LowBalanceState = ""
	LowBalanceStateOnlyLow    LowBalanceState = "low_balance"
	LowBalanceStateOnlyHealth LowBalanceState = "healthy"
)

type AuditLog struct {
	ID          string    `json:"id"`
	ActorUserID *string   `json:"actor_user_id"`
	ActorRole   string    `json:"actor_role"`
	StoreID     *string   `json:"store_id"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    *string   `json:"target_id"`
	Payload     []byte    `json:"payload_masked"`
	IPAddress   *string   `json:"ip_address"`
	UserAgent   *string   `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateStoreInput struct {
	Name                string  `json:"name"`
	Slug                string  `json:"slug"`
	LowBalanceThreshold *string `json:"low_balance_threshold"`
}

type UpdateStoreInput struct {
	Name                string  `json:"name"`
	Status              string  `json:"status"`
	LowBalanceThreshold *string `json:"low_balance_threshold"`
}

type UpdateCallbackInput struct {
	CallbackURL string `json:"callback_url"`
}

type CreateEmployeeInput struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type AssignStaffInput struct {
	UserID string `json:"user_id"`
}

type ListStoreDirectoryFilter struct {
	Query           string
	Status          *StoreStatus
	LowBalanceState LowBalanceState
	Limit           int
	Offset          int
	CreatedFrom     *time.Time
	CreatedTo       *time.Time
}

type StoreDirectorySummary struct {
	TotalCount      int `json:"total_count"`
	ActiveCount     int `json:"active_count"`
	InactiveCount   int `json:"inactive_count"`
	BannedCount     int `json:"banned_count"`
	DeletedCount    int `json:"deleted_count"`
	LowBalanceCount int `json:"low_balance_count"`
}

type StorePage struct {
	Items   []Store               `json:"items"`
	Summary StoreDirectorySummary `json:"summary"`
	Limit   int                   `json:"limit"`
	Offset  int                   `json:"offset"`
}

type ListEmployeesFilter struct {
	Query       string
	Limit       int
	Offset      int
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

type StaffUserPage struct {
	Items      []StaffUser `json:"items"`
	TotalCount int         `json:"total_count"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
}

type ListStoreStaffFilter struct {
	StoreID      string
	Query        string
	Limit        int
	Offset       int
	AssignedFrom *time.Time
	AssignedTo   *time.Time
}

type AuditFilter struct {
	StoreID *string
	Limit   int
}

type CreateStoreParams struct {
	OwnerUserID         string
	Name                string
	Slug                string
	APITokenHash        string
	LowBalanceThreshold *string
	OccurredAt          time.Time
}

type UpdateStoreParams struct {
	StoreID             string
	Name                string
	Status              StoreStatus
	LowBalanceThreshold *string
	OccurredAt          time.Time
}

type SoftDeleteStoreParams struct {
	StoreID    string
	OccurredAt time.Time
}

type RotateTokenParams struct {
	StoreID      string
	APITokenHash string
	OccurredAt   time.Time
}

type UpdateCallbackParams struct {
	StoreID     string
	CallbackURL string
	OccurredAt  time.Time
}

type CreateEmployeeParams struct {
	OwnerUserID  string
	Email        string
	Username     string
	PasswordHash string
	OccurredAt   time.Time
}

type AssignStaffParams struct {
	StoreID          string
	UserID           string
	CreatedByOwnerID string
	OccurredAt       time.Time
}
