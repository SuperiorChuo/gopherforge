package extdir

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

// SIP 网关（线路）物化：gateway XML 落 sip_profiles/external 卷（sofia rescan
// 生效）；出局拨号计划落 dialplan 卷（reloadxml 生效）。
//
// LCR 费率选路：同一 DialPrefix 下的多条网关合并成一个 extension，组内按费率
// 升序（RatePerMin，0 视为最贵沉底；同费率按 SortWeight 手动排序）串成一条
// failover bridge——`sofia/gw/a/号|sofia/gw/b/号`，前一条失败 FS 自动切下一条。
// 无字冠（DialPrefix 空）的网关不参与选路，各自渲染独立文件。
//
// 文件名：gateway XML 用 gw-<name>.xml；拨号计划用 20_gw-<groupKey>.xml
// （groupKey：有字冠取字冠 slug，无字冠取网关名），前缀 20_gw 与其他批次隔离。
// 历史上每网关一份 50_gw-<name>.xml，SyncGatewayDialplans 会连旧命名一起清。

func gatewayFile(dir, name string) string { return filepath.Join(dir, "gw-"+name+".xml") }

// gwDialplanFile 组文件名：groupKey 已是文件安全 slug。
func gwDialplanFile(dir, groupKey string) string {
	return filepath.Join(dir, "20_gw-"+groupKey+".xml")
}

// prefixSlug 把字冠里的 */# 换成文件名安全的记号（*→star，#→hash），纯数字原样。
func prefixSlug(prefix string) string {
	r := strings.NewReplacer("*", "star", "#", "hash")
	return "p" + r.Replace(prefix)
}

func renderGateway(g store.Gateway) string {
	register := "false"
	if g.Register {
		register = "true"
	}
	proxy := g.Proxy
	if proxy == "" {
		proxy = g.Realm
	}
	expire := g.ExpireSeconds
	if expire <= 0 {
		expire = 600
	}
	retry := g.RetrySeconds
	if retry <= 0 {
		retry = 30
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<include>\n  <gateway name=\"%s\">\n", g.Name)
	// 空参数不输出：sofia 对空 username/password 会在解析期丢弃网关
	// （Invalid Gateway），其余空值走 sofia 自身的默认继承链
	optional := []struct{ name, value string }{
		{"username", g.Username},
		{"password", g.Password},
		{"from-user", g.FromUser},
		{"from-domain", g.FromDomain},
		{"extension", g.Extension},
		{"register-transport", g.RegisterTransport},
	}
	for _, p := range optional {
		if p.value != "" {
			fmt.Fprintf(&b, "    <param name=\"%s\" value=\"%s\"/>\n", p.name, xmlEscaper.Replace(p.value))
		}
	}
	fmt.Fprintf(&b, `    <param name="realm" value="%s"/>
    <param name="proxy" value="%s"/>
    <param name="register" value="%s"/>
    <param name="expire-seconds" value="%d"/>
    <param name="retry-seconds" value="%d"/>
    <param name="caller-id-in-from" value="%t"/>
`, xmlEscaper.Replace(g.Realm), xmlEscaper.Replace(proxy), register, expire, retry, g.CallerIDInFrom)
	// OPTIONS 保活：失败自动标记 DOWN（呼叫拿到 NETWORK_OUT_OF_ORDER），最小 5s
	if g.PingSeconds >= 5 {
		fmt.Fprintf(&b, "    <param name=\"ping\" value=\"%d\"/>\n", g.PingSeconds)
	}
	b.WriteString("  </gateway>\n</include>\n")
	return b.String()
}

// sortByRate 按 LCR 排序规则排一组网关（就地排序）：
// RatePerMin 升序（0=未设视为最贵，排最后）；同费率按 SortWeight 升序（小在前）；
// 再退到网关名，保证渲染顺序稳定（幂等）。
func sortByRate(gws []store.Gateway) {
	rank := func(rate int64) (int64, bool) {
		if rate <= 0 {
			return 0, true // unset → 最贵桶
		}
		return rate, false
	}
	sort.SliceStable(gws, func(i, j int) bool {
		ri, ui := rank(gws[i].RatePerMin)
		rj, uj := rank(gws[j].RatePerMin)
		if ui != uj {
			return !ui // 有费率的排在未设之前
		}
		if !ui && ri != rj {
			return ri < rj
		}
		if gws[i].SortWeight != gws[j].SortWeight {
			return gws[i].SortWeight < gws[j].SortWeight
		}
		return gws[i].Name < gws[j].Name
	})
}

// renderGroupDialplan 渲染同字冠一组网关的 failover 出局拨号计划。gws 需已按
// sortByRate 排好序（第一条为最便宜/首选）。显号改写取首条网关的 CallerNumber
// （组内若各不相同，多网关逐 bridge 前 set 会复杂化，暂只用首条，见任务注释）。
func renderGroupDialplan(prefix string, gws []store.Gateway) string {
	first := gws[0]
	names := make([]string, 0, len(gws))
	for _, g := range gws {
		names = append(names, g.Name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<!-- 出局 LCR：拨 %s+号码，按费率升序 failover 走 [%s]（去冠→送网关，前一条失败自动切下一条），接通即录音 -->
<include>
  <extension name="gw_out_%s">
    <condition field="destination_number" expression="^%s(\d{3,20})$">
      <action application="set" data="hangup_after_bridge=true"/>
      <action application="set" data="continue_on_fail=true"/>
      <action application="set" data="RECORD_STEREO=true"/>
      <action application="export" data="execute_on_answer=record_session $${recordings_dir}/${uuid}.wav"/>
      <action application="export" data="cc_early_media=1"/>
`, prefix, strings.Join(names, ", "), prefixSlug(prefix), prefix)
	if first.CallerNumber != "" {
		fmt.Fprintf(&b, "      <action application=\"set\" data=\"effective_caller_id_number=%s\"/>\n", xmlEscaper.Replace(first.CallerNumber))
	}
	// 租户标记（转售计费）：export 使 A/B 腿都带 cc_tenant_id，随 json_cdr
	// variables 与 ESL 事件回中台按租户入账。与显号同语义取首条网关（组内多
	// 租户混排时以首选线路为准；转售场景一个字冠组通常同属一个客户）。
	if first.TenantID > 0 {
		fmt.Fprintf(&b, "      <action application=\"export\" data=\"cc_tenant_id=%d\"/>\n", first.TenantID)
	}
	// 本线路出局并发上限（mod_hash 后端）：仅单网关组插 limit（多网关 failover
	// 是一条通道走 | 序列，无法按目标线路分别计数——组内并发限制取首条会误伤
	// 备选线路，故多网关组不限，注释留档）。超限 USER_BUSY。
	if len(gws) == 1 && first.MaxConcurrent > 0 {
		fmt.Fprintf(&b, "      <action application=\"limit\" data=\"hash cc gw_%s %d !USER_BUSY\"/>\n", first.Name, first.MaxConcurrent)
	}
	// failover bridge：多条目标用 | 连接，前一条不通 FS 自动尝试下一条。
	targets := make([]string, 0, len(gws))
	for _, g := range gws {
		targets = append(targets, fmt.Sprintf("sofia/gateway/%s/%s$1", g.Name, g.Prepend))
	}
	fmt.Fprintf(&b, `      <action application="bridge" data="%s"/>
    </condition>
  </extension>
</include>
`, strings.Join(targets, "|"))
	return b.String()
}

// dialplanFilesFor 计算给定网关全集应落盘的拨号计划文件名→内容映射。
// 有字冠的按字冠分组渲染 failover；无字冠的各渲染独立文件（组名=网关名）。
// 停用（Disabled，欠费冻结联动）的线路不参与——组内全停则整组拨号计划消失。
func dialplanFilesFor(all []store.Gateway) map[string]string {
	groups := map[string][]store.Gateway{}
	var noPrefix []store.Gateway
	for _, g := range all {
		if g.Disabled {
			continue
		}
		if g.DialPrefix == "" {
			noPrefix = append(noPrefix, g)
			continue
		}
		groups[g.DialPrefix] = append(groups[g.DialPrefix], g)
	}
	files := map[string]string{}
	for prefix, gws := range groups {
		sortByRate(gws)
		files[prefixSlug(prefix)] = renderGroupDialplan(prefix, gws)
	}
	// 无字冠网关不入 LCR：各自一份 single-target 拨号计划（组名=网关名）。
	for _, g := range noPrefix {
		files[g.Name] = renderGroupDialplan(g.DialPrefix, []store.Gateway{g})
	}
	return files
}

// WriteGatewayProfile materializes只写 sofia gateway XML（每网关一份）。
// 拨号计划由 SyncGatewayDialplans 负责（LCR 分组需要网关全集）。
// 停用线路删物化文件（rescan 后 sofia 摘掉网关，注册/入局一并断）。
func WriteGatewayProfile(gwDir string, g store.Gateway) error {
	if err := os.MkdirAll(gwDir, 0o755); err != nil {
		return err
	}
	if g.Disabled {
		return removeIfExists(gatewayFile(gwDir, g.Name))
	}
	return os.WriteFile(gatewayFile(gwDir, g.Name), []byte(renderGateway(g)), 0o644)
}

// SyncGatewayDialplans 清旧网关拨号计划（新旧命名都清）后按网关全集全量重写。
// LCR 分组渲染必须拿到全集，故任何增删改后都调用它。
func SyncGatewayDialplans(dialplanDir string, all []store.Gateway) error {
	if err := os.MkdirAll(dialplanDir, 0o755); err != nil {
		return err
	}
	// 清旧文件：新命名 20_gw-*.xml + 历史每网关命名 50_gw-*.xml。
	for _, pat := range []string{"20_gw-*.xml", "50_gw-*.xml"} {
		old, _ := filepath.Glob(filepath.Join(dialplanDir, pat))
		for _, f := range old {
			_ = os.Remove(f)
		}
	}
	for key, body := range dialplanFilesFor(all) {
		if err := os.WriteFile(gwDialplanFile(dialplanDir, key), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// RemoveGatewayProfile deletes the sofia gateway XML（拨号计划由调用方随后
// SyncGatewayDialplans 全量重写清掉）。missing file is fine.
func RemoveGatewayProfile(gwDir, name string) error {
	return removeIfExists(gatewayFile(gwDir, name))
}

// SyncGateways rewrites all gateway files from DB rows (boot-time self-heal)：
// 每网关的 sofia XML + 按全集分组的 LCR 拨号计划。
func SyncGateways(gwDir, dialplanDir string, list []store.Gateway) error {
	for _, g := range list {
		if err := WriteGatewayProfile(gwDir, g); err != nil {
			return fmt.Errorf("materialize gateway %s: %w", g.Name, err)
		}
	}
	return SyncGatewayDialplans(dialplanDir, list)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
