package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/middleware"
	platformrealtime "github.com/mugiew/onixggr/internal/platform/realtime"
)

type HubContract interface {
	Subscribe(channels []string) *platformrealtime.Subscription
	Publish(ctx context.Context, event platformrealtime.Event) error
}

type Handler struct {
	service       Service
	hub           HubContract
	allowedOrigin string
	upgrader      websocket.Upgrader
}

func NewHandler(service Service, hub HubContract, allowedOrigin string) *Handler {
	handler := &Handler{
		service:       service,
		hub:           hub,
		allowedOrigin: strings.TrimSpace(allowedOrigin),
	}
	handler.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     handler.checkOrigin,
	}

	return handler
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /v1/realtime/ws", h.handleWebSocket())
}

func (h *Handler) handleWebSocket() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken, ok := websocketToken(r)
		if !ok {
			http.Error(w, "UNAUTHORIZED", http.StatusUnauthorized)
			return
		}

		session, err := h.service.AuthorizeConnection(r.Context(), accessToken)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrUnauthorized):
				http.Error(w, "UNAUTHORIZED", http.StatusUnauthorized)
			default:
				http.Error(w, "INTERNAL_ERROR", http.StatusInternalServerError)
			}
			return
		}

		conn, err := h.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		subscription := h.hub.Subscribe(session.Channels)
		defer subscription.Close()

		connectionID := requestID(r)
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteJSON(HelloFrame{
			Kind:                     "hello",
			ConnectionID:             connectionID,
			UserID:                   session.Subject.UserID,
			Role:                     string(session.Subject.Role),
			Channels:                 session.Channels,
			HeartbeatIntervalSeconds: session.HeartbeatSeconds,
			ConnectedAt:              time.Now().UTC(),
		}); err != nil {
			return
		}

		writeDone := make(chan struct{})
		go h.writeLoop(conn, subscription.Events, session.HeartbeatSeconds, writeDone)
		defer close(writeDone)

		_ = h.hub.Publish(r.Context(), platformrealtime.Event{
			Channel: userChannel(session.Subject.UserID),
			Type:    "realtime.connection.ready",
			Payload: map[string]any{
				"connection_id": connectionID,
				"role":          string(session.Subject.Role),
				"channels":      session.Channels,
			},
			CreatedAt: time.Now().UTC(),
		})

		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var frame struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(payload, &frame); err != nil {
				continue
			}

			if strings.EqualFold(strings.TrimSpace(frame.Type), "ping") {
				_ = h.hub.Publish(r.Context(), platformrealtime.Event{
					Channel: userChannel(session.Subject.UserID),
					Type:    "realtime.pong",
					Payload: map[string]any{
						"connection_id": connectionID,
						"source":        "client_ping",
					},
					CreatedAt: time.Now().UTC(),
				})
			}
		}
	})
}

func (h *Handler) writeLoop(conn *websocket.Conn, events <-chan platformrealtime.Event, heartbeatSeconds int, done <-chan struct{}) {
	interval := time.Duration(heartbeatSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case event, ok := <-events:
			if !ok {
				_ = conn.Close()
				return
			}

			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(EventFrame{
				Kind:  "event",
				Event: event,
			}); err != nil {
				_ = conn.Close()
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(HeartbeatFrame{
				Kind:   "heartbeat",
				SentAt: time.Now().UTC(),
			}); err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

func (h *Handler) checkOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	parsedOrigin, err := url.Parse(origin)
	if err != nil {
		return false
	}

	if strings.EqualFold(parsedOrigin.Host, r.Host) {
		return true
	}

	if strings.TrimSpace(h.allowedOrigin) == "" {
		return false
	}

	allowedOrigin, err := url.Parse(h.allowedOrigin)
	if err != nil {
		return false
	}

	return strings.EqualFold(parsedOrigin.Host, allowedOrigin.Host)
}

func websocketToken(r *http.Request) (string, bool) {
	queryToken := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if queryToken != "" {
		return queryToken, true
	}

	header := strings.TrimSpace(r.Header.Get("Authorization"))
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

func requestID(r *http.Request) string {
	requestID := strings.TrimSpace(middleware.GetRequestID(r.Context()))
	if requestID != "" {
		return requestID
	}

	return "realtime-session"
}
