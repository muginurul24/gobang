package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	channelPatternUser       = "user:*"
	channelPatternStore      = "store:*"
	channelPatternRole       = "role:*"
	channelPatternGlobalChat = "global_chat"
)

type Event struct {
	Channel   string         `json:"channel"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type Subscription struct {
	Events <-chan Event
	Close  func()
}

type Hub struct {
	client *goredis.Client
	pubsub *goredis.PubSub

	mu          sync.RWMutex
	subscribers map[string]map[*localSubscriber]struct{}

	closeOnce sync.Once
	closed    chan struct{}
}

type localSubscriber struct {
	hub      *Hub
	channels []string
	events   chan Event

	closeOnce sync.Once
}

func NewHub(ctx context.Context, client *goredis.Client) (*Hub, error) {
	pubsub := client.PSubscribe(ctx, channelPatternUser, channelPatternStore, channelPatternRole, channelPatternGlobalChat)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("subscribe realtime pubsub: %w", err)
	}

	hub := &Hub{
		client:      client,
		pubsub:      pubsub,
		subscribers: make(map[string]map[*localSubscriber]struct{}),
		closed:      make(chan struct{}),
	}

	go hub.run(ctx)

	return hub, nil
}

func (h *Hub) Subscribe(channels []string) *Subscription {
	normalized := normalizeChannels(channels)
	events := make(chan Event, 32)
	subscriber := &localSubscriber{
		hub:      h,
		channels: normalized,
		events:   events,
	}

	h.mu.Lock()
	for _, channel := range normalized {
		if h.subscribers[channel] == nil {
			h.subscribers[channel] = make(map[*localSubscriber]struct{})
		}
		h.subscribers[channel][subscriber] = struct{}{}
	}
	h.mu.Unlock()

	return &Subscription{
		Events: events,
		Close: func() {
			subscriber.close()
		},
	}
}

func (h *Hub) Publish(ctx context.Context, event Event) error {
	channel := strings.TrimSpace(event.Channel)
	if channel == "" {
		return fmt.Errorf("publish realtime event: empty channel")
	}
	if strings.TrimSpace(event.Type) == "" {
		return fmt.Errorf("publish realtime event: empty type")
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	} else {
		event.CreatedAt = event.CreatedAt.UTC()
	}
	event.Channel = channel

	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal realtime event: %w", err)
	}

	if err := h.client.Publish(ctx, channel, encoded).Err(); err != nil {
		return fmt.Errorf("publish realtime event: %w", err)
	}

	return nil
}

func (h *Hub) Close() error {
	var closeErr error
	h.closeOnce.Do(func() {
		close(h.closed)
		if h.pubsub != nil {
			closeErr = h.pubsub.Close()
		}

		h.mu.RLock()
		subscribers := make([]*localSubscriber, 0)
		for _, channelSubscribers := range h.subscribers {
			for subscriber := range channelSubscribers {
				subscribers = append(subscribers, subscriber)
			}
		}
		h.mu.RUnlock()

		for _, subscriber := range subscribers {
			subscriber.close()
		}
	})

	return closeErr
}

func (h *Hub) run(ctx context.Context) {
	messages := h.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			_ = h.Close()
			return
		case <-h.closed:
			return
		case message, ok := <-messages:
			if !ok {
				return
			}

			var event Event
			if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
				continue
			}

			if strings.TrimSpace(event.Channel) == "" {
				event.Channel = strings.TrimSpace(message.Channel)
			}
			if event.CreatedAt.IsZero() {
				event.CreatedAt = time.Now().UTC()
			}

			h.dispatch(event)
		}
	}
}

func (h *Hub) dispatch(event Event) {
	channel := strings.TrimSpace(event.Channel)
	if channel == "" {
		return
	}

	h.mu.RLock()
	subscribers := h.subscribers[channel]
	copied := make([]*localSubscriber, 0, len(subscribers))
	for subscriber := range subscribers {
		copied = append(copied, subscriber)
	}
	h.mu.RUnlock()

	for _, subscriber := range copied {
		subscriber.push(event)
	}
}

func (s *localSubscriber) push(event Event) {
	select {
	case s.events <- event:
	default:
		select {
		case <-s.events:
		default:
		}

		select {
		case s.events <- event:
		default:
		}
	}
}

func (s *localSubscriber) close() {
	s.closeOnce.Do(func() {
		s.hub.mu.Lock()
		for _, channel := range s.channels {
			subscribers := s.hub.subscribers[channel]
			delete(subscribers, s)
			if len(subscribers) == 0 {
				delete(s.hub.subscribers, channel)
			}
		}
		s.hub.mu.Unlock()
		close(s.events)
	})
}

func normalizeChannels(channels []string) []string {
	seen := make(map[string]struct{}, len(channels))
	normalized := make([]string, 0, len(channels))

	for _, channel := range channels {
		trimmed := strings.TrimSpace(channel)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}

		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	return normalized
}
