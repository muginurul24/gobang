package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TOTPManager struct {
	issuer string
	period time.Duration
	digits int
	skew   int64
}

func NewTOTPManager(issuer string) *TOTPManager {
	return &TOTPManager{
		issuer: issuer,
		period: 30 * time.Second,
		digits: 6,
		skew:   1,
	}
}

func (m *TOTPManager) GenerateSecret() (string, error) {
	buffer := make([]byte, 20)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}

	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buffer), nil
}

func (m *TOTPManager) Verify(secret string, code string, at time.Time) bool {
	normalized := strings.TrimSpace(code)
	if len(normalized) != m.digits {
		return false
	}

	for _, char := range normalized {
		if char < '0' || char > '9' {
			return false
		}
	}

	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return false
	}

	counter := at.UTC().Unix() / int64(m.period.Seconds())
	for offset := -m.skew; offset <= m.skew; offset++ {
		if oneTimePassword(key, counter+offset, m.digits) == normalized {
			return true
		}
	}

	return false
}

func (m *TOTPManager) OtpauthURL(accountName string, secret string) string {
	label := url.PathEscape(fmt.Sprintf("%s:%s", m.issuer, accountName))
	values := url.Values{}
	values.Set("secret", secret)
	values.Set("issuer", m.issuer)
	values.Set("algorithm", "SHA1")
	values.Set("digits", strconv.Itoa(m.digits))
	values.Set("period", strconv.Itoa(int(m.period.Seconds())))

	return fmt.Sprintf("otpauth://totp/%s?%s", label, values.Encode())
}

func oneTimePassword(key []byte, counter int64, digits int) string {
	var counterBytes [8]byte
	for index := 7; index >= 0; index-- {
		counterBytes[index] = byte(counter & 0xff)
		counter >>= 8
	}

	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counterBytes[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binary := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)

	mod := 1
	for index := 0; index < digits; index++ {
		mod *= 10
	}

	otp := binary % mod
	format := "%0" + strconv.Itoa(digits) + "d"
	return fmt.Sprintf(format, otp)
}
