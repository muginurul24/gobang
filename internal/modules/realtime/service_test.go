package realtime

import (
	"context"
	"errors"
	"testing"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestAuthorizeConnectionOwnerIncludesStoreChannels(t *testing.T) {
	service := NewService(Options{
		Repository:       stubRepository{storeIDs: []string{"store-b", "store-a"}},
		Authenticator:    stubAuthenticator{subject: auth.Subject{UserID: "user-1", Role: auth.RoleOwner}},
		HeartbeatSeconds: 45,
	})

	session, err := service.AuthorizeConnection(context.Background(), "token")
	if err != nil {
		t.Fatalf("AuthorizeConnection error = %v", err)
	}

	want := []string{"global_chat", "store:store-a", "store:store-b", "user:user-1"}
	if len(session.Channels) != len(want) {
		t.Fatalf("channels len = %d, want %d (%v)", len(session.Channels), len(want), session.Channels)
	}
	for index, channel := range want {
		if session.Channels[index] != channel {
			t.Fatalf("channel[%d] = %q, want %q", index, session.Channels[index], channel)
		}
	}
	if session.HeartbeatSeconds != 45 {
		t.Fatalf("HeartbeatSeconds = %d, want 45", session.HeartbeatSeconds)
	}
}

func TestAuthorizeConnectionDevIncludesRoleChannel(t *testing.T) {
	service := NewService(Options{
		Repository:    stubRepository{},
		Authenticator: stubAuthenticator{subject: auth.Subject{UserID: "dev-1", Role: auth.RoleDev}},
	})

	session, err := service.AuthorizeConnection(context.Background(), "token")
	if err != nil {
		t.Fatalf("AuthorizeConnection error = %v", err)
	}

	want := []string{"global_chat", "role:dev", "user:dev-1"}
	for index, channel := range want {
		if session.Channels[index] != channel {
			t.Fatalf("channel[%d] = %q, want %q", index, session.Channels[index], channel)
		}
	}
}

func TestAuthorizeConnectionPropagatesUnauthorized(t *testing.T) {
	service := NewService(Options{
		Repository:    stubRepository{},
		Authenticator: stubAuthenticator{err: auth.ErrUnauthorized},
	})

	_, err := service.AuthorizeConnection(context.Background(), "token")
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("error = %v, want auth.ErrUnauthorized", err)
	}
}

type stubRepository struct {
	storeIDs []string
	err      error
}

func (s stubRepository) ListAccessibleStoreIDs(context.Context, string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.storeIDs, nil
}

type stubAuthenticator struct {
	subject auth.Subject
	err     error
}

func (s stubAuthenticator) AuthenticateAccessToken(context.Context, string) (auth.Subject, error) {
	if s.err != nil {
		return auth.Subject{}, s.err
	}

	return s.subject, nil
}
