package storemembers

import "time"

type MemberStatus string

const (
	StatusActive   MemberStatus = "active"
	StatusInactive MemberStatus = "inactive"
)

type StoreScope struct {
	ID          string
	OwnerUserID string
	Name        string
	Slug        string
	DeletedAt   *time.Time
}

type StoreMember struct {
	ID               string       `json:"id"`
	StoreID          string       `json:"store_id"`
	RealUsername     string       `json:"real_username"`
	UpstreamUserCode string       `json:"upstream_user_code"`
	Status           MemberStatus `json:"status"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

type CreateStoreMemberInput struct {
	RealUsername string `json:"real_username"`
}

type ListStoreMembersFilter struct {
	StoreID     string
	Query       string
	Status      *MemberStatus
	Limit       int
	Offset      int
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

type StoreMemberSummary struct {
	TotalCount    int `json:"total_count"`
	ActiveCount   int `json:"active_count"`
	InactiveCount int `json:"inactive_count"`
}

type StoreMemberPage struct {
	Items   []StoreMember      `json:"items"`
	Summary StoreMemberSummary `json:"summary"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
}

type CreateStoreMemberParams struct {
	StoreID          string
	RealUsername     string
	UpstreamUserCode string
	Status           MemberStatus
	OccurredAt       time.Time
}
