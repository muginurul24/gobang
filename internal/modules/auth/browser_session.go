package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	RefreshCookieName = "onixggr_refresh_token"
	CSRFCookieName    = "onixggr_csrf_token"
	CSRFHeaderName    = "X-CSRF-Token"
)

type BrowserSession struct {
	User                 UserProfile `json:"user"`
	TokenType            string      `json:"token_type"`
	AccessToken          string      `json:"access_token"`
	AccessTokenExpiresAt time.Time   `json:"access_token_expires_at"`
	SessionJTI           string      `json:"session_jti"`
}

func sanitizeBrowserSession(session AuthSession) BrowserSession {
	return BrowserSession{
		User:                 session.User,
		TokenType:            session.TokenType,
		AccessToken:          session.AccessToken,
		AccessTokenExpiresAt: session.AccessTokenExpiresAt,
		SessionJTI:           session.SessionJTI,
	}
}

func issueBrowserSessionCookies(w http.ResponseWriter, appURL string, refreshToken string, refreshExpiresAt time.Time) (string, error) {
	csrfToken, err := newCSRFCookieToken()
	if err != nil {
		return "", err
	}

	cookieSettings := newBrowserCookieSettings(appURL)
	refreshTTL := time.Until(refreshExpiresAt.UTC())
	if refreshTTL < 0 {
		refreshTTL = 0
	}
	refreshMaxAge := int(refreshTTL.Seconds())

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    refreshToken,
		Path:     "/v1/auth",
		Domain:   cookieSettings.domain,
		HttpOnly: true,
		Secure:   cookieSettings.secure,
		SameSite: cookieSettings.sameSite,
		MaxAge:   refreshMaxAge,
		Expires:  refreshExpiresAt.UTC(),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		Domain:   cookieSettings.domain,
		HttpOnly: false,
		Secure:   cookieSettings.secure,
		SameSite: cookieSettings.sameSite,
		MaxAge:   refreshMaxAge,
		Expires:  refreshExpiresAt.UTC(),
	})

	return csrfToken, nil
}

func clearBrowserSessionCookies(w http.ResponseWriter, appURL string) {
	cookieSettings := newBrowserCookieSettings(appURL)

	for _, cookie := range []*http.Cookie{
		{
			Name:     RefreshCookieName,
			Value:    "",
			Path:     "/v1/auth",
			Domain:   cookieSettings.domain,
			HttpOnly: true,
			Secure:   cookieSettings.secure,
			SameSite: cookieSettings.sameSite,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0).UTC(),
		},
		{
			Name:     CSRFCookieName,
			Value:    "",
			Path:     "/",
			Domain:   cookieSettings.domain,
			HttpOnly: false,
			Secure:   cookieSettings.secure,
			SameSite: cookieSettings.sameSite,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0).UTC(),
		},
	} {
		http.SetCookie(w, cookie)
	}
}

func refreshTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}

	cookie, err := r.Cookie(RefreshCookieName)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(cookie.Value)
}

func setNoStoreHeaders(w http.ResponseWriter) {
	if w == nil {
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

type browserCookieSettings struct {
	domain   string
	secure   bool
	sameSite http.SameSite
}

func newBrowserCookieSettings(appURL string) browserCookieSettings {
	settings := browserCookieSettings{
		sameSite: http.SameSiteLaxMode,
	}

	parsed, err := url.Parse(strings.TrimSpace(appURL))
	if err != nil {
		return settings
	}

	if strings.EqualFold(parsed.Scheme, "https") {
		settings.secure = true
	}

	host := strings.TrimSpace(parsed.Hostname())
	switch host {
	case "", "localhost", "127.0.0.1":
	default:
		settings.domain = host
	}

	return settings
}

func newCSRFCookieToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}
