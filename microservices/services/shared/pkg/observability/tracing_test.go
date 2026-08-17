package observability

import (
	"context"
	"testing"
)

func TestInitTracerDisabledReturnsNoop(t *testing.T) {
	shutdown, err := InitTracer(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("disabled tracing should not fail: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected a shutdown function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("noop shutdown failed: %v", err)
	}
}

func TestNormalizeSampleRatio(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{-0.2, 0},
		{0, 0},
		{0.25, 0.25},
		{1, 1},
		{1.8, 1},
	}
	for _, tc := range cases {
		if got := normalizeSampleRatio(tc.in); got != tc.want {
			t.Fatalf("normalizeSampleRatio(%v)=%v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeOTLPEndpoint(t *testing.T) {
	cases := []struct {
		in        string
		wantHost  string
		wantInsec bool
	}{
		{"", defaultOTLPEndpoint, true},
		{"  ", defaultOTLPEndpoint, true},
		{"otel-collector:4317", "otel-collector:4317", true},
		{"http://otel-collector:4317", "otel-collector:4317", true},
		{"https://otel-collector:4317", "otel-collector:4317", false},
		{"http://otel-collector:4317/v1/traces", "otel-collector:4317", true},
		{"https://", defaultOTLPEndpoint, true},
	}
	for _, tc := range cases {
		host, insecure := normalizeOTLPEndpoint(tc.in)
		if host != tc.wantHost || insecure != tc.wantInsec {
			t.Fatalf("normalizeOTLPEndpoint(%q)=(%q,%v), want (%q,%v)", tc.in, host, insecure, tc.wantHost, tc.wantInsec)
		}
	}
}

func TestFallbackString(t *testing.T) {
	if got := fallbackString("  system  ", "go-admin-kit"); got != "system" {
		t.Fatalf("fallbackString trimmed value = %q", got)
	}
	if got := fallbackString("   ", "go-admin-kit"); got != "go-admin-kit" {
		t.Fatalf("fallbackString empty = %q", got)
	}
}
