package game

import "time"

type StoreStatus string

const (
	StoreStatusActive StoreStatus = "active"
)

type MemberStatus string

const (
	MemberStatusActive MemberStatus = "active"
)

type RequestMetadata struct {
	IPAddress string
	UserAgent string
}

type StoreScope struct {
	ID          string
	OwnerUserID string
	Name        string
	Slug        string
	Status      StoreStatus
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

type CreateUserInput struct {
	Username string `json:"username"`
}

type CreateStoreMemberParams struct {
	StoreID          string
	RealUsername     string
	UpstreamUserCode string
	Status           MemberStatus
	OccurredAt       time.Time
}
