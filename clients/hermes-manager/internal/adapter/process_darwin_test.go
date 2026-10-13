//go:build darwin

package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinStartReinstallsServiceDefinition(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{}
	adp, err := NewAdapter(Options{Home: home, Runner: runner})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	base := adp.(*Base)
	plistFile := filepath.Join(t.TempDir(), launchAgentLabel+".plist")
	base.process.(*darwinProcess).plistFile = plistFile
	if err := base.savePointer(&currentPointer{Version: "v1"}); err != nil {
		t.Fatalf("save pointer: %v", err)
	}

	if err := base.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := os.Stat(plistFile); err != nil {
		t.Fatalf("plist must be written only to the injected temp path: %v", err)
	}

	// 换版本后 launchd 不会自动重读 plist，必须 bootout 再 bootstrap
	joined := runner.joined()
	if !strings.Contains(joined, "launchctl bootout") || !strings.Contains(joined, "launchctl bootstrap") {
		t.Fatalf("start must reinstall service definition, got:\n%s", joined)
	}
	if !strings.Contains(joined, plistFile) {
		t.Fatalf("bootstrap must use injected plist path %q, got:\n%s", plistFile, joined)
	}
	bootout := strings.Index(joined, "bootout")
	bootstrap := strings.Index(joined, "bootstrap")
	if bootout > bootstrap {
		t.Fatalf("bootout must precede bootstrap, got:\n%s", joined)
	}
}
