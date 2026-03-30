package withdrawals

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type money int64

func parseAmount(value json.Number) (money, error) {
	raw := strings.TrimSpace(value.String())
	if raw == "" {
		return 0, ErrInvalidAmount
	}

	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, ErrInvalidAmount
	}

	return money(parsed * 100), nil
}

func parseMoneyString(raw string) (money, error) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, ErrInvalidAmount
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || !allDigits(parts[0]) {
		return 0, ErrInvalidAmount
	}

	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) == 0 || len(parts[1]) > 2 || !allDigits(parts[1]) {
			return 0, ErrInvalidAmount
		}

		fraction = parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
	}

	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}

	frac, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}

	return money(whole*100 + frac), nil
}

func formatAmount(value money) string {
	sign := ""
	raw := int64(value)
	if raw < 0 {
		sign = "-"
		raw = -raw
	}

	return fmt.Sprintf("%s%d.%02d", sign, raw/100, raw%100)
}

func (m money) LessThan(other money) bool {
	return m < other
}

func validIdempotencyKey(value string) bool {
	return idempotencyKeyPattern.MatchString(strings.TrimSpace(value))
}

func normalizeIdempotencyKey(value string) string {
	return strings.TrimSpace(value)
}

func computePlatformFee(amount money, feePercent float64) money {
	if amount <= 0 {
		return 0
	}

	basisPoints := int64(math.Round(feePercent * 100))
	if basisPoints < 0 {
		basisPoints = 0
	}

	return money((int64(amount) * basisPoints) / 10000)
}

func sanitizeClientRefID(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return randomClientRefID()
	}

	builder := strings.Builder{}
	for _, char := range normalized {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		}
	}

	sanitized := builder.String()
	if sanitized == "" {
		return randomClientRefID()
	}
	if len(sanitized) > 64 {
		return sanitized[:64]
	}

	return sanitized
}

func randomClientRefID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "WITHDRAWALREQUEST"
	}

	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for index := range buffer {
		buffer[index] = alphabet[int(buffer[index])%len(alphabet)]
	}

	return string(buffer)
}

func sameIntent(existing StoreWithdrawal, bankAccountID string, amount money) bool {
	return existing.StoreBankAccountID == strings.TrimSpace(bankAccountID) &&
		existing.NetRequestedAmount == formatAmount(amount)
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

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func allDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
