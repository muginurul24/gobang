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

type CreateStoreMemberParams struct {
	StoreID          string
	RealUsername     string
	UpstreamUserCode string
	Status           MemberStatus
	OccurredAt       time.Time
}
