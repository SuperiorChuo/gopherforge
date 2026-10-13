package extdir

import (
	"os"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

func TestRenderGatewayMinimal(t *testing.T) {
	// IP 对接：无账号密码时不得输出空 username/password 参数（sofia 会丢弃网关）
	out := renderGateway(store.Gateway{Name: "peer", Realm: "10.0.0.1:5080", Register: false})
	if strings.Contains(out, "username") || strings.Contains(out, "password") {
		t.Errorf("empty creds must be omitted:\n%s", out)
	}
	if !strings.Contains(out, `<param name="register" value="false"/>`) {
		t.Errorf("register=false missing:\n%s", out)
	}
	if !strings.Contains(out, `<param name="proxy" value="10.0.0.1:5080"/>`) {
		t.Errorf("proxy should default to realm:\n%s", out)
	}
}

func TestRenderGatewayRegister(t *testing.T) {
	out := renderGateway(store.Gateway{Name: "prov", Username: "u1", Password: "p1", Realm: "sip.x.com", Register: true})
	for _, want := range []string{`value="u1"`, `value="p1"`, `<param name="register" value="true"/>`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

// 同字冠两网关：按费率升序串 failover bridge，便宜的排在 | 前面。
func TestRenderGroupDialplanRateOrder(t *testing.T) {
	files := dialplanFilesFor([]store.Gateway{
		{Name: "expensive", DialPrefix: "9", Prepend: "0086", RatePerMin: 30},
		{Name: "cheap", DialPrefix: "9", Prepend: "0086", RatePerMin: 12, CallerNumber: "0571"},
	})
	body, ok := files[prefixSlug("9")]
	if !ok {
		t.Fatalf("expected grouped file for prefix 9, got keys %v", keysOf(files))
	}
	// 只应有一个 bridge，且 cheap 在 expensive 之前。
	ci := strings.Index(body, "sofia/gateway/cheap/0086$1")
	ei := strings.Index(body, "sofia/gateway/expensive/0086$1")
	if ci < 0 || ei < 0 {
		t.Fatalf("both targets must appear:\n%s", body)
	}
	if ci > ei {
		t.Errorf("cheap must come before expensive in failover:\n%s", body)
	}
	if !strings.Contains(body, "sofia/gateway/cheap/0086$1|sofia/gateway/expensive/0086$1") {
		t.Errorf("targets must be | -joined in one bridge:\n%s", body)
	}
	// 显号取首条（cheap）的 caller_number。
	if !strings.Contains(body, `effective_caller_id_number=0571`) {
		t.Errorf("caller_number should follow first gateway:\n%s", body)
	}
	// 只给网关 B-leg 导出早媒体标记，监听侧不靠号码猜测。
	if !strings.Contains(body, `application="export" data="cc_early_media=1"`) {
		t.Errorf("missing early-media marker:\n%s", body)
	}
	// 匹配表达式用原始字冠。
	if !strings.Contains(body, `expression="^9(\d{3,20})$"`) {
		t.Errorf("match expression wrong:\n%s", body)
	}
}

// 费率 0（未设）视为最贵，排在有费率网关之后。
func TestSortByRateUnsetIsCheapestLast(t *testing.T) {
	gws := []store.Gateway{
		{Name: "unset", DialPrefix: "9", RatePerMin: 0},
		{Name: "mid", DialPrefix: "9", RatePerMin: 20},
		{Name: "low", DialPrefix: "9", RatePerMin: 5},
	}
	sortByRate(gws)
	want := []string{"low", "mid", "unset"}
	for i, g := range gws {
		if g.Name != want[i] {
			t.Fatalf("order[%d]=%s want %s (full=%v)", i, g.Name, want[i], namesOf(gws))
		}
	}
}

// 同费率按 SortWeight 升序（小在前）。
func TestSortByRateTieBreakWeight(t *testing.T) {
	gws := []store.Gateway{
		{Name: "b", RatePerMin: 10, SortWeight: 5},
		{Name: "a", RatePerMin: 10, SortWeight: 1},
	}
	sortByRate(gws)
	if gws[0].Name != "a" {
		t.Errorf("tie-break by weight failed: %v", namesOf(gws))
	}
}

// 无字冠网关不进 LCR：各渲染独立文件（组名=网关名），单目标 bridge。
func TestDialplanFilesNoPrefixSingle(t *testing.T) {
	files := dialplanFilesFor([]store.Gateway{
		{Name: "peerA", DialPrefix: "", Prepend: ""},
	})
	body, ok := files["peerA"]
	if !ok {
		t.Fatalf("no-prefix gateway should render own file, keys=%v", keysOf(files))
	}
	if strings.Contains(body, "|") {
		t.Errorf("single gateway must not have | failover:\n%s", body)
	}
	if !strings.Contains(body, "sofia/gateway/peerA/$1") {
		t.Errorf("expected single target:\n%s", body)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func namesOf(gws []store.Gateway) []string {
	out := make([]string, 0, len(gws))
	for _, g := range gws {
		out = append(out, g.Name)
	}
	return out
}

func TestRenderGroupDialplanMaxConcurrent(t *testing.T) {
	gws := []store.Gateway{{Name: "gw1", DialPrefix: "9", MaxConcurrent: 5}}
	out := renderGroupDialplan("9", gws)
	if !strings.Contains(out, `limit" data="hash cc gw_gw1 5 !USER_BUSY`) {
		t.Fatalf("单网关组应插并发 limit:\n%s", out)
	}
	multi := []store.Gateway{{Name: "a", DialPrefix: "9", MaxConcurrent: 5}, {Name: "b", DialPrefix: "9"}}
	out2 := renderGroupDialplan("9", multi)
	if strings.Contains(out2, "limit") {
		t.Fatalf("多网关 failover 组不应插 limit:\n%s", out2)
	}
}

func TestRenderGroupDialplanTenantExport(t *testing.T) {
	// 首选（最便宜）线路挂租户 2 → export cc_tenant_id=2；租户跟随首条网关。
	files := dialplanFilesFor([]store.Gateway{
		{Name: "other", DialPrefix: "9", RatePerMin: 30, TenantID: 5},
		{Name: "own", DialPrefix: "9", RatePerMin: 12, TenantID: 2},
	})
	body := files[prefixSlug("9")]
	if !strings.Contains(body, `<action application="export" data="cc_tenant_id=2"/>`) {
		t.Errorf("tenant export should follow first (cheapest) gateway:\n%s", body)
	}
	// 未挂租户的组不产出 export。
	files = dialplanFilesFor([]store.Gateway{{Name: "plain", DialPrefix: "8"}})
	if strings.Contains(files[prefixSlug("8")], "cc_tenant_id") {
		t.Errorf("no-tenant group must not export cc_tenant_id:\n%s", files[prefixSlug("8")])
	}
}

func TestDisabledGatewayExcludedFromDialplan(t *testing.T) {
	// 组内一停一开：failover 只剩启用线路；全停：整组拨号计划消失。
	files := dialplanFilesFor([]store.Gateway{
		{Name: "up", DialPrefix: "9", RatePerMin: 12},
		{Name: "down", DialPrefix: "9", RatePerMin: 10, Disabled: true},
	})
	body := files[prefixSlug("9")]
	if strings.Contains(body, "sofia/gateway/down/") {
		t.Errorf("disabled gateway must not appear in dialplan:\n%s", body)
	}
	if !strings.Contains(body, "sofia/gateway/up/") {
		t.Errorf("enabled gateway must remain:\n%s", body)
	}
	files = dialplanFilesFor([]store.Gateway{
		{Name: "solo", DialPrefix: "8", Disabled: true},
	})
	if len(files) != 0 {
		t.Errorf("all-disabled group must render no dialplan, got %v", keysOf(files))
	}
}

func TestWriteGatewayProfileDisabledRemovesFile(t *testing.T) {
	dir := t.TempDir()
	g := store.Gateway{Name: "acme", Realm: "sip.acme.com"}
	if err := WriteGatewayProfile(dir, g); err != nil {
		t.Fatalf("write enabled: %v", err)
	}
	if _, err := os.Stat(gatewayFile(dir, "acme")); err != nil {
		t.Fatalf("enabled gateway file missing: %v", err)
	}
	g.Disabled = true
	if err := WriteGatewayProfile(dir, g); err != nil {
		t.Fatalf("write disabled: %v", err)
	}
	if _, err := os.Stat(gatewayFile(dir, "acme")); !os.IsNotExist(err) {
		t.Fatalf("disabled gateway file must be removed, err=%v", err)
	}
	// 幂等：再停一次不报错。
	if err := WriteGatewayProfile(dir, g); err != nil {
		t.Fatalf("repeat disabled write: %v", err)
	}
	// 恢复重物化。
	g.Disabled = false
	if err := WriteGatewayProfile(dir, g); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if _, err := os.Stat(gatewayFile(dir, "acme")); err != nil {
		t.Fatalf("re-enabled gateway file missing: %v", err)
	}
}
