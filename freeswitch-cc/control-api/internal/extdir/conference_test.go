package extdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

func TestRenderConferencePINRecord(t *testing.T) {
	out := renderConferenceDialplan(store.CCConference{
		Name: "board", Number: "3001", PIN: "1234", MaxMembers: 10, AutoRecord: true,
	})
	for _, want := range []string{
		`expression="^3001$"`,
		`data="max_members=10"`,
		`conf-board-`,               // 自动录音文件名
		`data="board@default+1234"`, // PIN 拼在会议参数后
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderConferenceNoPINNoRecord(t *testing.T) {
	out := renderConferenceDialplan(store.CCConference{Name: "daily", Number: "3002"})
	if strings.Contains(out, "+") {
		t.Errorf("no PIN must not append +PIN:\n%s", out)
	}
	if strings.Contains(out, "max_members") {
		t.Errorf("MaxMembers=0 must omit max_members:\n%s", out)
	}
	if strings.Contains(out, "conference_auto_record") {
		t.Errorf("AutoRecord=false must omit auto-record:\n%s", out)
	}
	if !strings.Contains(out, `data="daily@default"`) {
		t.Errorf("plain conference arg missing:\n%s", out)
	}
}

func TestSyncConferences(t *testing.T) {
	dir := t.TempDir()
	// 先放一个旧文件，验证全量重写会清掉已删会议室
	stale := filepath.Join(dir, "40_conf-gone.xml")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	rooms := []store.CCConference{{Name: "board", Number: "3001"}}
	if err := SyncConferences(dir, rooms); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale conference file should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "40_conf-board.xml")); err != nil {
		t.Errorf("missing 40_conf-board.xml: %v", err)
	}
	// 动态兜底必须始终存在
	dyn, err := os.ReadFile(filepath.Join(dir, confDynamicFile))
	if err != nil {
		t.Fatalf("missing dynamic fallback: %v", err)
	}
	if !strings.Contains(string(dyn), `dyn-$1@default`) {
		t.Errorf("dynamic fallback wrong:\n%s", dyn)
	}
}
