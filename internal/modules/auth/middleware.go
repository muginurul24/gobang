package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type subjectContextKey string

const subjectKey subjectContextKey = "auth_subject"

func RequireAuth(service Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		subject, err := service.AuthenticateAccessToken(r.Context(), token)
		if err != nil {
			switch {
			case errors.Is(err, ErrUnauthorized):
				writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			default:
				writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
			}
			return
		}

		next.ServeHTTP(w, r.WithContext(WithSubject(r.Context(), subject)))
	})
}

func RequireRoles(next http.Handler, roles ...Role) http.Handler {
	allowed := make(map[Role]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		if _, found := allowed[subject.Role]; !found {
			writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func WithSubject(ctx context.Context, subject Subject) context.Context {
	return context.WithValue(ctx, subjectKey, subject)
}

func SubjectFromContext(ctx context.Context) (Subject, bool) {
	subject, ok := ctx.Value(subjectKey).(Subject)
	return subject, ok
}

func bearerToken(header string) (string, bool) {
	if header == "" {
		return "", false
	}

	prefix, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(prefix, "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	return token, true
}
