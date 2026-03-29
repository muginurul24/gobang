package id

import (
	"crypto/rand"
	"encoding/hex"
)

func NewRequestID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "req-fallback"
	}

	return "req_" + hex.EncodeToString(buffer)
}
