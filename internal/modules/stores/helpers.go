package stores

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func normalizeSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validSlug(value string) bool {
	return slugPattern.MatchString(value)
}

func normalizeThreshold(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || parsed < 0 {
		return nil, ErrInvalidThreshold
	}

	normalized := strconv.FormatFloat(parsed, 'f', -1, 64)
	return &normalized, nil
}

func parseStatus(raw string) (StoreStatus, error) {
	status := StoreStatus(strings.TrimSpace(strings.ToLower(raw)))
	switch status {
	case StatusActive, StatusInactive, StatusBanned, StatusDeleted:
		return status, nil
	default:
		return "", ErrInvalidStatus
	}
}

func validateCallbackURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidCallbackURL
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidCallbackURL
	}

	return parsed.String(), nil
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
