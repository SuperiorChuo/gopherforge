package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/config"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestSetupCORSAllowsLocalDevelopmentFrontendPorts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalConfig := config.Cfg
	t.Cleanup(func() {
		config.Cfg = originalConfig
	})

	config.Cfg = config.Config{
		App: config.AppCfg{Env: "development"},
		CORS: config.CORSConfig{
			AllowOrigins:     []string{"http://127.0.0.1:3002"},
			AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
			AllowHeaders:     []string{"Origin", "Content-Type"},
			AllowCredentials: true,
			MaxAge:           12,
		},
	}

	router := gin.New()
	setupCORS(router)
	router.GET("/api/v1/captcha", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/captcha", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "http://127.0.0.1:5173")
	}
}

func TestConfigureGinWritersUsesDiscardInProduction(t *testing.T) {
	originalMode := gin.Mode()
	originalWriter := gin.DefaultWriter
	originalErrorWriter := gin.DefaultErrorWriter
	t.Cleanup(func() {
		gin.SetMode(originalMode)
		gin.DefaultWriter = originalWriter
		gin.DefaultErrorWriter = originalErrorWriter
	})

	configureGinWriters("production")

	if gin.DefaultWriter == nil {
		t.Fatal("production gin DefaultWriter must not be nil")
	}
	if gin.DefaultErrorWriter == nil {
		t.Fatal("production gin DefaultErrorWriter must not be nil")
	}
	if _, err := gin.DefaultWriter.Write([]byte("probe")); err != nil {
		t.Fatalf("production gin DefaultWriter write failed: %v", err)
	}
	if _, err := gin.DefaultErrorWriter.Write([]byte("probe")); err != nil {
		t.Fatalf("production gin DefaultErrorWriter write failed: %v", err)
	}
}

func TestServeHTTPServerGracefullyWaitsForInFlightRequests(t *testing.T) {
	releaseHandler := make(chan struct{})
	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(handlerStarted)
			<-releaseHandler
			w.WriteHeader(http.StatusNoContent)
			close(handlerDone)
		}),
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	shutdown := make(chan os.Signal, 1)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- serveHTTPServer(server, listener, time.Second, shutdown)
	}()

	clientErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			clientErr <- err
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusNoContent {
			clientErr <- fmt.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
			return
		}
		clientErr <- nil
	}()

	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	shutdown <- os.Interrupt

	select {
	case err := <-serverErr:
		t.Fatalf("server returned before in-flight request completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseHandler)

	select {
	case err := <-clientErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("client request did not complete")
	}

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not complete")
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestStartDepartmentTreeInvalidationListenerReturnsErrorWithoutRedis(t *testing.T) {
	oldClient := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldClient
	})

	listener, err := startDepartmentTreeInvalidationListener(context.Background())
	if err == nil {
		t.Fatal("expected error when redis client is nil")
	}
	if listener != nil {
		t.Fatal("expected nil listener when redis client is nil")
	}
}

func TestStartDepartmentTreeInvalidationListenerStartsAndCloses(t *testing.T) {
	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	oldClient := redisstore.Client
	redisstore.Client = goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	t.Cleanup(func() {
		_ = redisstore.Client.Close()
		redisstore.Client = oldClient
		store.Close()
	})

	listener, err := startDepartmentTreeInvalidationListener(context.Background())
	if err != nil {
		t.Fatalf("startDepartmentTreeInvalidationListener() error = %v", err)
	}
	if listener == nil {
		t.Fatal("expected listener")
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close() error = %v", err)
	}
}
