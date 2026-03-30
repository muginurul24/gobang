package bankaccounts

import (
	"regexp"
	"strings"
)

var accountNumberPattern = regexp.MustCompile(`^[0-9]{6,24}$`)

func normalizeBankCode(code string) string {
	return strings.TrimSpace(code)
}

func normalizeAccountNumber(number string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", ".", "")
	return strings.TrimSpace(replacer.Replace(number))
}

func validAccountNumber(number string) bool {
	return accountNumberPattern.MatchString(number)
}

func maskAccountNumber(number string) string {
	if len(number) <= 4 {
		return strings.Repeat("*", len(number))
	}

	last4 := number[len(number)-4:]
	return strings.Repeat("*", len(number)-4) + last4
}
