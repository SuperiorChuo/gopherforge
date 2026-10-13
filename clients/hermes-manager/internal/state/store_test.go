package state

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"os"
	"runtime"
	"testing"
)

func TestStorePersistsDeviceKeyWithoutChangingIt(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	publicKey, privateKey, err := store.EnsureDeviceKey()
	if err != nil {
		t.Fatalf("ensure key: %v", err)
	}
	if len(publicKey) != ed25519.PublicKeySize || len(privateKey) != ed25519.PrivateKeySize {
		t.Fatalf("key sizes = %d/%d", len(publicKey), len(privateKey))
	}
	loaded, err := store.LoadPrivateKey()
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	if !bytes.Equal(privateKey, loaded) {
		t.Fatal("stored private key changed")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(store.dir + string(os.PathSeparator) + privateKeyFileName)
		if err != nil {
			t.Fatalf("stat key: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("key permissions = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestStoreReportsNotEnrolledWithoutConfig(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := store.LoadConfig(); !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("LoadConfig() error = %v, want ErrNotEnrolled", err)
	}
}
