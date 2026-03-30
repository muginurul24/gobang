package realtime

import "testing"

func TestConnectionCountTracksUniqueSubscribers(t *testing.T) {
	hub := &Hub{
		subscribers: make(map[string]map[*localSubscriber]struct{}),
	}

	first := &localSubscriber{hub: hub, channels: []string{"global_chat", "role:dev"}}
	second := &localSubscriber{hub: hub, channels: []string{"global_chat"}}

	hub.subscribers["global_chat"] = map[*localSubscriber]struct{}{
		first:  {},
		second: {},
	}
	hub.subscribers["role:dev"] = map[*localSubscriber]struct{}{
		first: {},
	}

	if count := hub.ConnectionCount(); count != 2 {
		t.Fatalf("ConnectionCount() = %d, want 2", count)
	}
}
