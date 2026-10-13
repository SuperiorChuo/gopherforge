package extdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

func TestRenderTimeRouteOpenClosedTarget(t *testing.T) {
	out := renderTimeRoute(store.CCTimeRoute{
		Name: "biz", AccessNumber: "6600", Wdays: "2-6",
		TimeRanges: "09:00-12:00,13:30-18:00",
		OpenTarget: "7000", ClosedTarget: "6000",
	})
	for _, want := range []string{
		`expression="^6600$"`,                                      // 号码不匹配即跳出（默认 break，不能 never——会全局劫持）
		`wday="2-6" time-of-day="09:00-12:00"`,                     // 第一段
		`wday="2-6" time-of-day="13:30-18:00"`,                     // 第二段
		`<action application="transfer" data="7000 XML default"/>`, // 营业转
		`name="tr_biz_closed"`,                                     // 兜底 extension
		`<action application="transfer" data="6000 XML default"/>`, // 非营业转
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// 两个时段 = 两条 open extension
	if n := strings.Count(out, "_open"); n != 2 {
		t.Errorf("want 2 open extensions, got %d:\n%s", n, out)
	}
}

func TestRenderTimeRouteClosedPromptFallback(t *testing.T) {
	// ClosedTarget 空 + ClosedPrompt 空 → answer+sleep+playback misc/sorry.wav+hangup
	out := renderTimeRoute(store.CCTimeRoute{
		Name: "shop", AccessNumber: "6601", Wdays: "2-6", TimeRanges: "09:00-18:00",
		OpenTarget: "7001",
	})
	for _, want := range []string{
		`<action application="answer"/>`,
		`<action application="sleep" data="500"/>`,
		`<action application="playback" data="misc/sorry.wav"/>`,
		`<action application="hangup"/>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "tr_shop_closed\">\n    <condition field=\"destination_number\" expression=\"^6601$\">\n      <action application=\"transfer\"") {
		t.Errorf("empty closed_target must not transfer:\n%s", out)
	}
}

func TestRenderTimeRouteClosedPromptCustom(t *testing.T) {
	out := renderTimeRoute(store.CCTimeRoute{
		Name: "vip", AccessNumber: "6602", Wdays: "2-6", TimeRanges: "09:00-18:00",
		OpenTarget: "7002", ClosedPrompt: "/etc/freeswitch/prompts/afterhours.wav",
	})
	if !strings.Contains(out, `data="/etc/freeswitch/prompts/afterhours.wav"`) {
		t.Errorf("custom closed prompt missing:\n%s", out)
	}
	if strings.Contains(out, "misc/sorry.wav") {
		t.Errorf("custom prompt must override default:\n%s", out)
	}
}

func TestSyncTimeRoutes(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "15_time-gone.xml")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	routes := []store.CCTimeRoute{{
		Name: "biz", AccessNumber: "6600", Wdays: "2-6", TimeRanges: "09:00-18:00", OpenTarget: "7000",
	}}
	if err := SyncTimeRoutes(dir, routes); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale time-route file should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "15_time-biz.xml")); err != nil {
		t.Errorf("missing 15_time-biz.xml: %v", err)
	}
}
