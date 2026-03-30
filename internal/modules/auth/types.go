package auth

import "time"

type Role string

const (
	RoleDev        Role = "dev"
	RoleSuperadmin Role = "superadmin"
	RoleOwner      Role = "owner"
	RoleKaryawan   Role = "karyawan"
)

type User struct {
	ID                  string
	Email               string
	Username            string
	PasswordHash        string
	Role                Role
	IsActive            bool
	TOTPEnabled         bool
	TOTPSecretEncrypted string
	IPAllowlist         *string
	LastLoginAt         *time.Time
}

type SessionRecord struct {
	ID          string
	UserID      string
	SessionJTI  string
	RefreshHash string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SessionWithUser struct {
	Session SessionRecord
	User    User
}

type SessionState struct {
	SessionID  string    `json:"session_id"`
	UserID     string    `json:"user_id"`
	Role       Role      `json:"role"`
	SessionJTI string    `json:"session_jti"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type Subject struct {
	UserID     string
	Role       Role
	SessionJTI string
}

type UserProfile struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Username    string     `json:"username"`
	Role        Role       `json:"role"`
	TOTPEnabled bool       `json:"totp_enabled"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

type AuthSession struct {
	User                  UserProfile `json:"user"`
	TokenType             string      `json:"token_type"`
	AccessToken           string      `json:"access_token"`
	AccessTokenExpiresAt  time.Time   `json:"access_token_expires_at"`
	RefreshToken          string      `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time   `json:"refresh_token_expires_at"`
	SessionJTI            string      `json:"session_jti"`
}

type RequestMetadata struct {
	IPAddress string
	UserAgent string
}

type LoginInput struct {
	Login        string
	Password     string
	TOTPCode     string
	RecoveryCode string
	RequestMetadata
}

type SecuritySettings struct {
	UserID         string  `json:"user_id"`
	TOTPEnabled    bool    `json:"totp_enabled"`
	IPAllowlist    *string `json:"ip_allowlist"`
	Recommended2FA bool    `json:"recommended_2fa"`
}

type TOTPEnrollment struct {
	Secret     string    `json:"secret"`
	OtpAuthURL string    `json:"otpauth_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type RecoveryCodesPayload struct {
	Codes []string `json:"codes"`
}

type RecoveryCodeRecord struct {
	ID       string
	CodeHash string
}

type EnableTOTPInput struct {
	Code string
	RequestMetadata
}

type DisableTOTPInput struct {
	TOTPCode     string
	RecoveryCode string
	RequestMetadata
}

type UpdateIPAllowlistInput struct {
	IPAllowlist *string
	RequestMetadata
}

type ReplaceUserSessionsParams struct {
	UserID      string
	ActorRole   Role
	SessionJTI  string
	RefreshHash string
	IPAddress   string
	UserAgent   string
	ExpiresAt   time.Time
	OccurredAt  time.Time
}

type ReplaceUserSessionsResult struct {
	Session     SessionRecord
	RevokedJTIs []string
}

type RotateSessionParams struct {
	OldSessionJTI string
	NewSessionJTI string
	RefreshHash   string
	IPAddress     string
	UserAgent     string
	ExpiresAt     time.Time
	OccurredAt    time.Time
}

type RevokeSessionParams struct {
	UserID     string
	SessionJTI string
	ActorRole  Role
	IPAddress  string
	UserAgent  string
	OccurredAt time.Time
}

type RevokeAllSessionsParams struct {
	UserID     string
	ActorRole  Role
	IPAddress  string
	UserAgent  string
	OccurredAt time.Time
}

type EnableTOTPParams struct {
	UserID             string
	ActorRole          Role
	EncryptedSecret    string
	RecoveryCodeHashes []string
	IPAddress          string
	UserAgent          string
	OccurredAt         time.Time
}

type DisableTOTPParams struct {
	UserID     string
	ActorRole  Role
	IPAddress  string
	UserAgent  string
	OccurredAt time.Time
}

type UpdateIPAllowlistParams struct {
	UserID      string
	ActorRole   Role
	IPAllowlist *string
	IPAddress   string
	UserAgent   string
	OccurredAt  time.Time
}

type AuditLogParams struct {
	ActorUserID *string
	ActorRole   string
	TargetType  string
	TargetID    *string
	Action      string
	Payload     map[string]any
	IPAddress   string
	UserAgent   string
	OccurredAt  time.Time
}
