package extdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

func TestRenderRingGroupSimultaneous(t *testing.T) {
	out := renderRingGroup(store.CCRingGroup{
		Name: "sales", AccessNumber: "8000", Strategy: "simultaneous",
		Exts: "1000,1001,1002", RingTimeout: 25, FailoverTarget: "6000",
	})
	for _, want := range []string{
		`expression="^8000$"`,
		`data="call_timeout=25"`,
		`data="continue_on_fail=true"`,
		`data="hangup_after_bridge=true"`,
		`<action application="bridge" data="user/1000,user/1001,user/1002"/>`, // 同振逗号
		`<action application="transfer" data="6000 XML default"/>`,            // 兜底转接
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderRingGroupSequential(t *testing.T) {
	out := renderRingGroup(store.CCRingGroup{
		Name: "support", AccessNumber: "8001", Strategy: "sequential",
		Exts: "1000,1001",
	})
	if !strings.Contains(out, `data="user/1000|user/1001"`) {
		t.Errorf("sequential must use pipe separator:\n%s", out)
	}
	// FailoverTarget 空 → hangup
	if !strings.Contains(out, `<action application="hangup"/>`) {
		t.Errorf("empty failover must hangup:\n%s", out)
	}
	if strings.Contains(out, "transfer") {
		t.Errorf("empty failover must not transfer:\n%s", out)
	}
}

func TestRenderRingGroupDefaultTimeout(t *testing.T) {
	out := renderRingGroup(store.CCRingGroup{
		Name: "g", AccessNumber: "8002", Strategy: "simultaneous", Exts: "1000",
	})
	if !strings.Contains(out, `data="call_timeout=20"`) {
		t.Errorf("RingTimeout=0 must default to 20:\n%s", out)
	}
}

func TestSyncRingGroups(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "45_rg-gone.xml")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	groups := []store.CCRingGroup{{
		Name: "sales", AccessNumber: "8000", Strategy: "simultaneous", Exts: "1000,1001",
	}}
	if err := SyncRingGroups(dir, groups); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale ring-group file should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "45_rg-sales.xml")); err != nil {
		t.Errorf("missing 45_rg-sales.xml: %v", err)
	}
}
