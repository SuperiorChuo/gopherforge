package grpcx

import (
	"os"
	"testing"
)

func TestTLSRequiredEnv(t *testing.T) {
	t.Setenv("GRPC_TLS_REQUIRED", "")
	if tlsRequired() {
		t.Fatal("empty should be false")
	}
	t.Setenv("GRPC_TLS_REQUIRED", "1")
	if !tlsRequired() {
		t.Fatal("1 should be true")
	}
	t.Setenv("GRPC_TLS_REQUIRED", "true")
	if !tlsRequired() {
		t.Fatal("true should be true")
	}
	t.Setenv("GRPC_TLS_REQUIRED", "no")
	if tlsRequired() {
		t.Fatal("no should be false")
	}
}

func TestNewServerInsecureWithoutCerts(t *testing.T) {
	t.Setenv("GRPC_TLS_REQUIRED", "")
	t.Setenv("TLS_CERT_PATH", "")
	t.Setenv("TLS_KEY_PATH", "")
	t.Setenv("TLS_CA_PATH", "")
	srv := NewServer()
	if srv == nil {
		t.Fatal("nil server")
	}
	srv.Stop()
}

func TestNewServerExitsWhenTLSRequiredWithoutCerts(t *testing.T) {
	t.Setenv("GRPC_TLS_REQUIRED", "1")
	t.Setenv("TLS_CERT_PATH", "")
	t.Setenv("TLS_KEY_PATH", "")
	t.Setenv("TLS_CA_PATH", "")

	old := exitFunc
	exited := 0
	exitFunc = func(code int) { exited = code }
	t.Cleanup(func() { exitFunc = old })

	// NewServer 在 require 且无证书时调用 exitFunc(1)；测试用桩避免真退出。
	// 调用后进程逻辑本应中止，此处仍可能返回已部分构造的 server。
	_ = NewServer()
	if exited != 1 {
		t.Fatalf("exit code = %d, want 1", exited)
	}
	_ = os.Stderr // silence unused if any
}
