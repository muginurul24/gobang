package storemembers

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

type Handler struct {
	service     Service
	authService auth.Service
}

func NewHandler(service Service, authService auth.Service) *Handler {
	return &Handler{service: service, authService: authService}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /v1/stores/{storeID}/members", auth.RequireAuth(h.authService, h.handleListStoreMembers()))
	mux.Handle("POST /v1/stores/{storeID}/members", auth.RequireAuth(h.authService, h.handleCreateStoreMember()))
}

func (h *Handler) handleListStoreMembers() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		filter := ListStoreMembersFilter{
			StoreID: r.PathValue("storeID"),
			Query:   strings.TrimSpace(r.URL.Query().Get("query")),
			Limit:   25,
		}
		if rawStatus := strings.TrimSpace(r.URL.Query().Get("status")); rawStatus != "" {
			status, ok := parseOptionalMemberStatus(rawStatus)
			if !ok {
				writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
				return
			}
			filter.Status = status
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			limit, err := strconv.Atoi(raw)
			if err != nil {
				writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
				return
			}
			filter.Limit = limit
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
			offset, err := strconv.Atoi(raw)
			if err != nil {
				writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
				return
			}
			filter.Offset = offset
		}
		createdFrom, err := parseFilterTime(r.URL.Query().Get("created_from"))
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		createdTo, err := parseFilterTime(r.URL.Query().Get("created_to"))
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		filter.CreatedFrom = createdFrom
		filter.CreatedTo = createdTo

		members, err := h.service.ListStoreMembers(r.Context(), subject, filter)
		if err != nil {
			writeStoreMemberError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", members)
	})
}

func (h *Handler) handleCreateStoreMember() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input CreateStoreMemberInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		member, err := h.service.CreateStoreMember(r.Context(), subject, r.PathValue("storeID"), input, requestMetadata(r))
		if err != nil {
			writeStoreMemberError(w, err)
			return
		}

		writeEnvelope(w, http.StatusCreated, true, "SUCCESS", member)
	})
}

type envelope struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func writeEnvelope(w http.ResponseWriter, status int, ok bool, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Status:  ok,
		Message: message,
		Data:    data,
	})
}

func writeStoreMemberError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
	case errors.Is(err, ErrInvalidRealUsername):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REAL_USERNAME", nil)
	case errors.Is(err, ErrDuplicateRealUsername):
		writeEnvelope(w, http.StatusConflict, false, "DUPLICATE_REAL_USERNAME", nil)
	default:
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	return decoder.Decode(target)
}

func requestMetadata(r *http.Request) auth.RequestMetadata {
	return auth.RequestMetadata{
		IPAddress: clientIP(r),
		UserAgent: r.UserAgent(),
	}
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		ip, _, _ := strings.Cut(forwarded, ",")
		parsed := net.ParseIP(strings.TrimSpace(ip))
		if parsed != nil {
			return parsed.String()
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		parsed := net.ParseIP(host)
		if parsed != nil {
			return parsed.String()
		}
	}

	parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr))
	if parsed != nil {
		return parsed.String()
	}

	return "0.0.0.0"
}

func parseOptionalMemberStatus(raw string) (*MemberStatus, bool) {
	switch MemberStatus(strings.TrimSpace(raw)) {
	case StatusActive:
		value := StatusActive
		return &value, true
	case StatusInactive:
		value := StatusInactive
		return &value, true
	default:
		return nil, false
	}
}

func parseFilterTime(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}

	parsed = parsed.UTC()
	return &parsed, nil
}
