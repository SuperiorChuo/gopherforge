package ipinfo

import (
	"context"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestIPInfoPackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("ipinfo.go")
	if err != nil {
		t.Fatalf("read ipinfo.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("ipinfo.go contains non-English source text")
	}
}

func TestPrivateIPInfoUsesEnglishLocation(t *testing.T) {
	client := NewIPInfoClient(time.Second, time.Minute)

	info, err := client.GetIPInfoContext(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatalf("GetIPInfoContext(private): %v", err)
	}
	if info.Country != "Private Network" || info.City != "Private Network" {
		t.Fatalf("private location = country %q city %q, want Private Network", info.Country, info.City)
	}

	location := client.GetLocationContext(context.Background(), "127.0.0.1")
	if location != "Private Network" {
		t.Fatalf("private location string = %q, want Private Network", location)
	}
}

func TestIPAPIRequestUsesEnglishLanguage(t *testing.T) {
	content, err := os.ReadFile("ipinfo.go")
	if err != nil {
		t.Fatalf("read ipinfo.go: %v", err)
	}

	if strings.Contains(string(content), "lang=zh-CN") {
		t.Fatal("ip-api request still asks for Chinese responses")
	}
}

func TestGetIPInfoContextHonorsCancellation(t *testing.T) {
	client := NewIPInfoClient(time.Minute, time.Minute)
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !errors.Is(req.Context().Err(), context.Canceled) {
			t.Fatalf("request context error = %v, want context.Canceled", req.Context().Err())
		}
		return nil, req.Context().Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetIPInfoContext(ctx, "8.8.8.8")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetIPInfoContext() error = %v, want context.Canceled", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
