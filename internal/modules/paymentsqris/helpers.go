package paymentsqris

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
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

func formatAmountFromCents(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}

	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

func newCustomRef() (string, error) {
	return newCustomRefWithPrefix("TOPUP")
}

func newMemberPaymentRef() (string, error) {
	return newCustomRefWithPrefix("MPAY")
}

func newCustomRefWithPrefix(prefix string) (string, error) {
	buffer := make([]byte, customRefLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	for index := range buffer {
		buffer[index] = alphabet[int(buffer[index])%len(alphabet)]
	}

	return prefix + string(buffer), nil
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

func normalizeUsername(value string) string {
	return strings.TrimSpace(value)
}

func resolvePaymentStatus(raw string) (TransactionStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "success":
		return TransactionStatusSuccess, true
	case "failed":
		return TransactionStatusFailed, true
	case "expired":
		return TransactionStatusExpired, true
	default:
		return "", false
	}
}

func providerStateForStatus(status TransactionStatus) ProviderState {
	switch status {
	case TransactionStatusSuccess:
		return ProviderStateWebhookSuccess
	case TransactionStatusFailed:
		return ProviderStateWebhookFailed
	case TransactionStatusExpired:
		return ProviderStateWebhookExpired
	default:
		return ProviderStatePendingProviderAnswer
	}
}

func computeMemberPaymentAmounts(grossAmount int64, feePercent float64) (string, string) {
	if grossAmount <= 0 {
		return formatAmount(0), formatAmount(0)
	}

	basisPoints := int64(math.Round(feePercent * 100))
	if basisPoints < 0 {
		basisPoints = 0
	}

	grossCents := grossAmount * 100
	feeCents := grossAmount * basisPoints / 100
	if feeCents < 0 {
		feeCents = 0
	}
	if feeCents > grossCents {
		feeCents = grossCents
	}

	return formatAmountFromCents(feeCents), formatAmountFromCents(grossCents - feeCents)
}
