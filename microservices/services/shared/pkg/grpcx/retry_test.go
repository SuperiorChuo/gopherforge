package grpcx

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unavailable", status.Error(codes.Unavailable, "upstream down"), true},
		{"aborted", status.Error(codes.Aborted, "aborted"), true},
		{"resource-exhausted", status.Error(codes.ResourceExhausted, "overloaded"), true},
		{"unimplemented", status.Error(codes.Unimplemented, "no method"), false},
		{"invalid-argument", status.Error(codes.InvalidArgument, "bad"), false},
		{"not-found", status.Error(codes.NotFound, "missing"), false},
		{"permission-denied", status.Error(codes.PermissionDenied, "nope"), false},
		{"internal", status.Error(codes.Internal, "bug"), false},
		{"deadline-exceeded", status.Error(codes.DeadlineExceeded, "slow"), false},
		{"context-canceled", context.Canceled, false},
		{"context-deadline", context.DeadlineExceeded, false},
		{"raw-network-error", errors.New("connection refused"), true},
		{"wrapped-raw-error", errors.New("wrap: connection refused"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Retryable(c.err); got != c.want {
				t.Fatalf("Retryable(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline-exceeded", status.Error(codes.DeadlineExceeded, "slow"), true},
		{"unavailable", status.Error(codes.Unavailable, "down"), true},
		{"raw-network-error", errors.New("connection refused"), true},
		{"canceled", status.Error(codes.Canceled, "stopped"), false},
		{"context-canceled", context.Canceled, false},
		{"unimplemented", status.Error(codes.Unimplemented, "no method"), false},
		{"invalid-argument", status.Error(codes.InvalidArgument, "bad"), false},
		{"not-found", status.Error(codes.NotFound, "missing"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsTransient(c.err); got != c.want {
				t.Fatalf("IsTransient(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
