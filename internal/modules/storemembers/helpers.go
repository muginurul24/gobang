package storemembers

import (
	"crypto/rand"
	"encoding/json"
	"strings"
)

const upstreamUserCodeLength = 12

var upstreamAlphabet = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")

func normalizeRealUsername(value string) string {
	return strings.TrimSpace(value)
}

func validRealUsername(value string) bool {
	return strings.TrimSpace(value) != ""
}

func NewUpstreamUserCode() (string, error) {
	buffer := make([]byte, upstreamUserCodeLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	for index := range buffer {
		buffer[index] = upstreamAlphabet[int(buffer[index])%len(upstreamAlphabet)]
	}

	return string(buffer), nil
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
