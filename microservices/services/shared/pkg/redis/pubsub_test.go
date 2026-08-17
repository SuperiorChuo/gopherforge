package redis

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func setupPubSubTestClient(t *testing.T) {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	previousClient := Client
	Client = goredis.NewClient(&goredis.Options{Addr: store.Addr()})

	t.Cleanup(func() {
		_ = Client.Close()
		Client = previousClient
		store.Close()
	})
}

func TestPublishStringDeliversMessageToStartSubscriber(t *testing.T) {
	setupPubSubTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	messages := make(chan string, 1)
	subscriber, err := StartSubscriber(ctx, "department-tree", func(_ context.Context, payload string) {
		messages <- payload
	})
	if err != nil {
		t.Fatalf("start subscriber: %v", err)
	}
	defer func() {
		if err := subscriber.Close(); err != nil {
			t.Fatalf("close subscriber: %v", err)
		}
	}()

	if err := PublishString(ctx, "department-tree", "invalidate"); err != nil {
		t.Fatalf("publish string: %v", err)
	}

	select {
	case got := <-messages:
		if got != "invalidate" {
			t.Fatalf("message = %q, want %q", got, "invalidate")
		}
	case <-ctx.Done():
		t.Fatal("subscriber did not receive published message")
	}
}

func TestStartSubscriberReturnsErrorWhenClientIsNil(t *testing.T) {
	previousClient := Client
	Client = nil
	t.Cleanup(func() {
		Client = previousClient
	})

	subscriber, err := StartSubscriber(context.Background(), "department-tree", func(context.Context, string) {})
	if err == nil {
		t.Fatal("expected error when Client is nil")
	}
	if subscriber != nil {
		t.Fatal("expected nil subscriber when Client is nil")
	}
}

func TestStringSubscriberCloseIsIdempotent(t *testing.T) {
	setupPubSubTestClient(t)

	subscriber, err := StartSubscriber(context.Background(), "department-tree", func(context.Context, string) {})
	if err != nil {
		t.Fatalf("start subscriber: %v", err)
	}

	if err := subscriber.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := subscriber.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
