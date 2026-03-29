package security

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mugiew/onixggr/internal/platform/clock"
)

type AccessTokenClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type AccessTokenManager struct {
	secret []byte
	ttl    time.Duration
	issuer string
	clock  clock.Clock
}

func NewAccessTokenManager(secret string, ttl time.Duration, issuer string, now clock.Clock) *AccessTokenManager {
	if now == nil {
		now = clock.SystemClock{}
	}

	return &AccessTokenManager{
		secret: []byte(secret),
		ttl:    ttl,
		issuer: issuer,
		clock:  now,
	}
}

func (m *AccessTokenManager) Issue(userID string, role string, sessionJTI string) (string, time.Time, error) {
	issuedAt := m.clock.Now().UTC()
	expiresAt := issuedAt.Add(m.ttl)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, AccessTokenClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			ID:        sessionJTI,
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}

	return signed, expiresAt, nil
}

func (m *AccessTokenManager) Parse(raw string) (AccessTokenClaims, error) {
	var claims AccessTokenClaims

	_, err := jwt.ParseWithClaims(
		raw,
		&claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
			}

			return m.secret, nil
		},
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("parse access token: %w", err)
	}

	return claims, nil
}
