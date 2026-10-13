package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
)

const earlyTestUUID = "11111111-2222-3333-4444-555555555555"

func earlyTestWAV() []byte {
	b := make([]byte, 80)
	copy(b, "RIFF")
	copy(b[8:], "WAVEfmt ")
	return b
}

func earlyEvent(name string, marked bool) map[string]string {
	ev := map[string]string{
		"Event-Name": name, "Unique-ID": earlyTestUUID,
		"Channel-Call-UUID":    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"variable_cc_voice_id": "7", "variable_cc_tenant_id": "9",
	}
	if marked {
		ev["variable_cc_early_media"] = "1"
	}
	return ev
}

func TestHandleEarlyMediaMarkedBlocksAndSigns(t *testing.T) {
	const secret = "secret"
	wav := earlyTestWAV()
	classifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(raw)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if r.Header.Get("X-CC-Signature") != want || !bytes.Equal(raw, wav) {
			t.Fatalf("bad signed sample")
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"blocked":true,"class":"empty"}}`))
	}))
	defer classifier.Close()

	dir := t.TempDir()
	killed := make(chan string, 1)
	var commands []string
	s := &Server{Cfg: config.Config{
		EarlyMediaURL: classifier.URL, EarlyMediaDir: dir, EarlyMediaSeconds: 4,
		WebhookSecret: secret,
	}, earlySampleDuration: 10 * time.Millisecond}
	s.earlyCommand = func(cmd string) error {
		commands = append(commands, cmd)
		if strings.Contains(cmd, " start ") {
			return os.WriteFile(filepath.Join(dir, "early-"+earlyTestUUID+".wav"), wav, 0o600)
		}
		return nil
	}
	s.killChannel = func(uuid string) error { killed <- uuid; return nil }
	s.HandleEarlyMedia(earlyEvent("CHANNEL_PROGRESS_MEDIA", true))
	select {
	case got := <-killed:
		if got != earlyTestUUID {
			t.Fatalf("kill=%s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected kill")
	}
	if len(commands) != 2 || !strings.Contains(commands[0], " start ") || !strings.Contains(commands[1], " stop ") {
		t.Fatalf("commands=%v", commands)
	}
	if _, err := os.Stat(filepath.Join(dir, "early-"+earlyTestUUID+".wav")); !os.IsNotExist(err) {
		t.Fatalf("sample not cleaned: %v", err)
	}
}

func TestHandleEarlyMediaAnswerCancelsWithoutClassification(t *testing.T) {
	var calls atomic.Int32
	classifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"code":200,"data":{"blocked":true,"class":"empty"}}`))
	}))
	defer classifier.Close()
	dir := t.TempDir()
	stopped := make(chan struct{}, 1)
	s := &Server{Cfg: config.Config{EarlyMediaURL: classifier.URL, EarlyMediaDir: dir}, earlySampleDuration: time.Second}
	s.earlyCommand = func(cmd string) error {
		if strings.Contains(cmd, " start ") {
			_ = os.WriteFile(filepath.Join(dir, "early-"+earlyTestUUID+".wav"), earlyTestWAV(), 0o600)
		}
		if strings.Contains(cmd, " stop ") {
			stopped <- struct{}{}
		}
		return nil
	}
	s.HandleEarlyMedia(earlyEvent("CHANNEL_PROGRESS_MEDIA", true))
	s.HandleEarlyMedia(earlyEvent("CHANNEL_ANSWER", false))
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("recording was not stopped")
	}
	if calls.Load() != 0 {
		t.Fatalf("classification calls=%d", calls.Load())
	}
}

func TestHandleEarlyMediaUnmarkedIgnored(t *testing.T) {
	s := &Server{Cfg: config.Config{EarlyMediaURL: "http://unused", EarlyMediaDir: t.TempDir()}}
	called := false
	s.earlyCommand = func(string) error { called = true; return nil }
	s.HandleEarlyMedia(earlyEvent("CHANNEL_PROGRESS_MEDIA", false))
	time.Sleep(20 * time.Millisecond)
	if called {
		t.Fatal("unmarked channel was recorded")
	}
}

func TestHandleEarlyMediaClassifierFailureFailsOpen(t *testing.T) {
	classifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer classifier.Close()
	dir := t.TempDir()
	var killed atomic.Bool
	s := &Server{Cfg: config.Config{EarlyMediaURL: classifier.URL, EarlyMediaDir: dir}, earlySampleDuration: 10 * time.Millisecond}
	s.earlyCommand = func(cmd string) error {
		if strings.Contains(cmd, " start ") {
			return os.WriteFile(filepath.Join(dir, "early-"+earlyTestUUID+".wav"), earlyTestWAV(), 0o600)
		}
		return nil
	}
	s.killChannel = func(string) error { killed.Store(true); return nil }
	s.HandleEarlyMedia(earlyEvent("CHANNEL_PROGRESS_MEDIA", true))
	time.Sleep(100 * time.Millisecond)
	if killed.Load() {
		t.Fatal("classifier failure must fail open")
	}
}
