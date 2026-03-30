package callbacks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const (
	maxRetries = 5
)

type signer struct {
	secret []byte
}

func newSigner(secret string) signer {
	return signer{secret: []byte(strings.TrimSpace(secret))}
}

func (s signer) Sign(payload []byte) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func nextRetryAt(now time.Time, attemptNo int) *time.Time {
	if attemptNo <= 0 || attemptNo > maxRetries {
		return nil
	}

	backoff := time.Minute << (attemptNo - 1)
	resolved := now.Add(backoff).UTC()
	return &resolved
}

func maskResponseBody(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if len(trimmed) > 2048 {
		trimmed = trimmed[:2048]
	}

	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}

	masked := maskValue(payload)
	encoded, err := json.Marshal(masked)
	if err != nil {
		return trimmed
	}

	return string(encoded)
}

func maskValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if isSensitiveKey(key) {
				result[key] = "***"
				continue
			}

			result[key] = maskValue(child)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			result = append(result, maskValue(child))
		}
		return result
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "token", "secret", "password", "authorization", "signature", "api_key", "client_key":
		return true
	default:
		return false
	}
}
