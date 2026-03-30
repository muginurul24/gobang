package auth

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func normalizeLogin(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeRecoveryCode(value string) string {
	replacer := strings.NewReplacer("-", "", " ", "")
	return strings.ToUpper(replacer.Replace(strings.TrimSpace(value)))
}

func maskIdentifier(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "***"
	}

	if len(trimmed) <= 4 {
		return trimmed[:1] + "***"
	}

	return trimmed[:2] + "***" + trimmed[len(trimmed)-2:]
}

func newRecoveryCode() (string, error) {
	normalized, err := randomRecoveryString(10)
	if err != nil {
		return "", err
	}

	return normalized[:5] + "-" + normalized[5:], nil
}

func randomRecoveryString(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate recovery code: %w", err)
	}

	result := make([]byte, length)
	for index := range buffer {
		result[index] = recoveryAlphabet[int(buffer[index])%len(recoveryAlphabet)]
	}

	return string(result), nil
}
