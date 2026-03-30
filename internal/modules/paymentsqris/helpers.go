package paymentsqris

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const customRefLength = 18

var alphabet = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")

func parseAmount(value json.Number) (int64, error) {
	raw := strings.TrimSpace(value.String())
	if raw == "" {
		return 0, ErrInvalidAmount
	}

	amount, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || amount <= 0 {
		return 0, ErrInvalidAmount
	}

	return amount, nil
}

func formatAmount(value int64) string {
	return fmt.Sprintf("%d.00", value)
}

func newCustomRef() (string, error) {
	buffer := make([]byte, customRefLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	for index := range buffer {
		buffer[index] = alphabet[int(buffer[index])%len(alphabet)]
	}

	return "TOPUP" + string(buffer), nil
}

func toJSON(payload map[string]any) string {
	if payload == nil {
		return "{}"
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}

	return string(encoded)
}

func providerStatePtr(value ProviderState) *ProviderState {
	if value == "" {
		return nil
	}

	result := value
	return &result
}

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func resolveExpiresAt(now time.Time, providerExpiry *int, fallbackSeconds int) *time.Time {
	if providerExpiry != nil {
		seconds := int64(*providerExpiry)
		if seconds > 9999999999 {
			resolved := time.UnixMilli(seconds).UTC()
			return &resolved
		}

		resolved := time.Unix(seconds, 0).UTC()
		return &resolved
	}

	if fallbackSeconds <= 0 {
		return nil
	}

	resolved := now.Add(time.Duration(fallbackSeconds) * time.Second).UTC()
	return &resolved
}

func payloadFieldString(payload map[string]any, key string) *string {
	if payload == nil {
		return nil
	}

	value, ok := payload[key].(string)
	if !ok {
		return nil
	}

	return stringPtr(value)
}

func payloadFieldProviderState(payload map[string]any) *ProviderState {
	value := payloadFieldString(payload, "provider_state")
	if value == nil {
		return nil
	}

	state := ProviderState(*value)
	return &state
}
