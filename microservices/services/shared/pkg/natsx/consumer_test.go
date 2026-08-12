package natsx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestConsumerProcessesMessage(t *testing.T) {
	nc, js := setupJetStream(t)
	defer nc.Close()
	subject := "test.audit.log.create"
	_, err := js.AddStream(&nats.StreamConfig{
		Name:     "test_stream",
		Subjects: []string{subject},
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("add stream: %v", err)
	}
	var processed int32
	cleanup, err := StartConsumer(context.Background(), nc.ConnectedUrl(), ConsumerConfig{
		StreamName:   "test_stream",
		Subject:      subject,
		ConsumerName: "test_consumer",
		MaxDeliver:   3,
		AckWait:      5 * time.Second,
		DLQ:          false,
		ProcessFunc: func(ctx context.Context, data []byte, meta *MsgMeta) error {
			atomic.AddInt32(&processed, 1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("start consumer: %v", err)
	}
	defer cleanup()
	_, err = js.Publish(subject, []byte(`{"action":"test"}`))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for atomic.LoadInt32(&processed) < 1 {
		select {
		case <-deadline:
			t.Fatalf("timeout: processed = %d, want >= 1", atomic.LoadInt32(&processed))
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestConsumerDLQ(t *testing.T) {
	nc, js := setupJetStream(t)
	defer nc.Close()
	subject := "test.audit.log.fail"
	dlqSubject := "dlq.test.audit.log.fail"
	_, err := js.AddStream(&nats.StreamConfig{
		Name:     "test_dlq_stream",
		Subjects: []string{subject},
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("add stream: %v", err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "test_dlq_stream_dlq",
		Subjects: []string{dlqSubject},
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("add DLQ stream: %v", err)
	}
	var attempts int32
	cleanup, err := StartConsumer(context.Background(), nc.ConnectedUrl(), ConsumerConfig{
		StreamName:   "test_dlq_stream",
		Subject:      subject,
		ConsumerName: "test_dlq_consumer",
		MaxDeliver:   2,
		AckWait:      1 * time.Second,
		DLQ:          true,
		DLQSubject:   dlqSubject,
		ProcessFunc: func(ctx context.Context, data []byte, meta *MsgMeta) error {
			atomic.AddInt32(&attempts, 1)
			return errors.New("permanent failure")
		},
	})
	if err != nil {
		t.Fatalf("start consumer: %v", err)
	}
	defer cleanup()
	_, err = js.Publish(subject, []byte(`{"action":"fail"}`))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	deadline := time.After(10 * time.Second)
	for atomic.LoadInt32(&attempts) < 2 {
		select {
		case <-deadline:
			t.Fatalf("timeout: attempts = %d, want >= 2", atomic.LoadInt32(&attempts))
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	time.Sleep(500 * time.Millisecond)
	dlqInfo, err := js.StreamInfo("test_dlq_stream_dlq")
	if err != nil {
		t.Fatalf("DLQ stream info: %v", err)
	}
	if dlqInfo.State.Msgs < 1 {
		t.Fatalf("DLQ messages = %d, want >= 1", dlqInfo.State.Msgs)
	}
}

func setupJetStream(t *testing.T) (*nats.Conn, nats.JetStreamContext) {
	t.Helper()
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		t.Fatalf("jetstream: %v", err)
	}
	return nc, js
}
