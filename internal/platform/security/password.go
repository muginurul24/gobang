package security

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type PasswordHasher struct {
	cost int
}

func NewPasswordHasher(cost int) PasswordHasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}

	return PasswordHasher{cost: cost}
}

func (h PasswordHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("generate bcrypt hash: %w", err)
	}

	return string(hash), nil
}

func (h PasswordHasher) Compare(hash string, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return fmt.Errorf("compare bcrypt hash: %w", err)
	}

	return nil
}
