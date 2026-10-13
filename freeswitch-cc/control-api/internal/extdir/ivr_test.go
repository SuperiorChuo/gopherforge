package extdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

func TestParseCollect(t *testing.T) {
	p, ok := ParseCollect("2:6:8000:welcome.wav:1000")
	if !ok {
		t.Fatal("expected parse ok")
	}
	if p.Min != 2 || p.Max != 6 || p.Timeout != 8000 || p.Prompt != "welcome.wav" || p.Next != "1000" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	// 提示音留空占位仍合法
	if p2, ok2 := ParseCollect("4:4:5000::2000"); !ok2 || p2.Prompt != "" || p2.Next != "2000" {
		t.Fatalf("empty prompt parse failed: %+v ok=%v", p2, ok2)
	}
	// 字段不足直接失败
	if _, ok3 := ParseCollect("2:6:8000:welcome.wav"); ok3 {
		t.Fatal("expected parse failure for missing next")
	}
}

func TestWriteIVRCollect(t *testing.T) {
	ivrDir := t.TempDir()
	dpDir := t.TempDir()
	m := store.IVRMenu{
		Name:         "main-menu",
		AccessNumber: "6000",
		Entries:      `[{"digits":"1","action":"transfer","target":"1000"},{"digits":"2","action":"collect","target":"3:6:8000:enter-account.wav:2000"}]`,
	}
	if err := WriteIVR(ivrDir, dpDir, m); err != nil {
		t.Fatal(err)
	}
	// 菜单里 collect 键渲染成 transfer 到收号隐藏拨号计划
	menu, err := os.ReadFile(ivrMenuFile(ivrDir, m.Name))
	if err != nil {
		t.Fatal(err)
	}
	want := `transfer cccollect_main-menu_2 XML default`
	if !strings.Contains(string(menu), want) {
		t.Errorf("menu missing collect transfer:\n%s", menu)
	}
	// 收号拨号计划文件存在且 play_and_get_digits 参数序正确
	dp, err := os.ReadFile(collectDialplanFile(dpDir, m.Name, "2"))
	if err != nil {
		t.Fatalf("collect dialplan not written: %v", err)
	}
	s := string(dp)
	if !strings.Contains(s, `play_and_get_digits" data="3 6 3 8000 # enter-account.wav silence_stream://250 cc_collected \d+"`) {
		t.Errorf("play_and_get_digits param order wrong:\n%s", s)
	}
	if !strings.Contains(s, `export" data="cc_collected=${cc_collected}"`) {
		t.Errorf("missing export of cc_collected:\n%s", s)
	}
	if !strings.Contains(s, `transfer" data="2000 XML default"`) {
		t.Errorf("missing transfer to next:\n%s", s)
	}

	// 更新去掉 collect 键 → 收号拨号计划应被清理
	m.Entries = `[{"digits":"1","action":"transfer","target":"1000"}]`
	if err := WriteIVR(ivrDir, dpDir, m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(collectDialplanFile(dpDir, m.Name, "2")); !os.IsNotExist(err) {
		t.Error("stale collect dialplan should be removed on update")
	}

	// 删除 IVR → 主拨号计划与收号拨号计划都清掉
	// 先加回 collect 再删
	m.Entries = `[{"digits":"2","action":"collect","target":"3:6:8000::2000"}]`
	if err := WriteIVR(ivrDir, dpDir, m); err != nil {
		t.Fatal(err)
	}
	if err := RemoveIVR(ivrDir, dpDir, m.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(collectDialplanFile(dpDir, m.Name, "2")); !os.IsNotExist(err) {
		t.Error("collect dialplan should be removed on delete")
	}
	if _, err := os.Stat(filepath.Join(dpDir, "40_ivr-main-menu.xml")); !os.IsNotExist(err) {
		t.Error("ivr dialplan should be removed on delete")
	}
}
