package users

import (
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Username        string     `json:"username"`
	Role            auth.Role  `json:"role"`
	IsActive        bool       `json:"is_active"`
	CreatedByUserID *string    `json:"created_by_user_id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastLoginAt     *time.Time `json:"last_login_at"`
}

type CreateUserInput struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type UpdateUserStatusInput struct {
	IsActive *bool `json:"is_active"`
}

type ListFilter struct {
	Query       string
	Role        *auth.Role
	IsActive    *bool
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Limit       int
	Offset      int
}

type DirectorySummary struct {
	TotalCount      int `json:"total_count"`
	OwnerCount      int `json:"owner_count"`
	SuperadminCount int `json:"superadmin_count"`
	DevCount        int `json:"dev_count"`
	ActiveCount     int `json:"active_count"`
	InactiveCount   int `json:"inactive_count"`
}

type Page struct {
	Items   []User           `json:"items"`
	Summary DirectorySummary `json:"summary"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

type CreateUserParams struct {
	Email           string
	Username        string
	PasswordHash    string
	Role            auth.Role
	CreatedByUserID *string
	OccurredAt      time.Time
}

type UpdateUserStatusParams struct {
	UserID     string
	IsActive   bool
	OccurredAt time.Time
}
