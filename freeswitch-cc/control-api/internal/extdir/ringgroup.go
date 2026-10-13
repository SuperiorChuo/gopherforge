package extdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

// 响铃组物化：
//   - 每个响铃组一份拨号计划 45_rg-<name>.xml（拨 AccessNumber 触发）。
//   - 同振 simultaneous：bridge 目标用逗号 user/1000,user/1001（同时振铃）；
//     顺振 sequential：用竖线 user/1000|user/1001（依次振铃，前一个超时/未接
//     再振下一个）。
//   - call_timeout 控制整组振铃超时；continue_on_fail=true + hangup_after_bridge
//     =true 保证桥接失败能落到 bridge 之后的兜底动作。
//   - bridge 之后：FailoverTarget 非空→transfer；空→hangup。
//   - 落 dialplan 卷（→ /etc/freeswitch/dialplan/default），reloadxml 生效。
//
// 文件名前缀 45_rg-* 排在队列 30_ccq-*、会议 40_conf- 之后（响铃组是终点接入号，
// 无需抢跑），与时间路由 15_time-* 前后有别。

func ringGroupFile(dir, name string) string {
	return filepath.Join(dir, "45_rg-"+name+".xml")
}

// renderRingGroup 生成单个响铃组的拨号计划。
func renderRingGroup(g store.CCRingGroup) string {
	timeout := g.RingTimeout
	if timeout <= 0 {
		timeout = 20
	}
	// 同振逗号、顺振竖线拼接 user/<ext>
	sep := ","
	if g.Strategy == "sequential" {
		sep = "|"
	}
	exts := splitExts(g.Exts)
	dials := make([]string, 0, len(exts))
	for _, e := range exts {
		dials = append(dials, "user/"+e)
	}
	bridgeData := strings.Join(dials, sep)

	var b strings.Builder
	fmt.Fprintf(&b, `<!-- 响铃组 %s：拨 %s %s一组分机 -->
<include>
  <extension name="rg_%s">
    <condition field="destination_number" expression="^%s$">
      <action application="set" data="call_timeout=%d"/>
      <action application="set" data="continue_on_fail=true"/>
      <action application="set" data="hangup_after_bridge=true"/>
      <action application="bridge" data="%s"/>
`, xmlEscaper.Replace(g.Name), g.AccessNumber, strategyLabel(g.Strategy), g.Name, g.AccessNumber, timeout, xmlEscaper.Replace(bridgeData))
	if g.FailoverTarget != "" {
		fmt.Fprintf(&b, "      <action application=\"transfer\" data=\"%s XML default\"/>\n", xmlEscaper.Replace(g.FailoverTarget))
	} else {
		b.WriteString("      <action application=\"hangup\"/>\n")
	}
	b.WriteString("    </condition>\n  </extension>\n</include>\n")
	return b.String()
}

// strategyLabel 注释里给的人话说明（同振/顺振）。
func strategyLabel(s string) string {
	if s == "sequential" {
		return "顺振"
	}
	return "同振"
}

// splitExts 拆 "1000,1001,1002" 为逐个分机；空/全空白返回 nil。
func splitExts(s string) []string {
	var out []string
	for _, e := range strings.Split(s, ",") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// SyncRingGroups rewrites every ring-group dialplan from DB rows.
// 先清旧 45_rg-*.xml 再全量重写（删除响铃组场景）。
func SyncRingGroups(dialplanDir string, groups []store.CCRingGroup) error {
	if err := os.MkdirAll(dialplanDir, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dialplanDir, "45_rg-*.xml"))
	for _, f := range old {
		_ = os.Remove(f)
	}
	for _, g := range groups {
		if err := os.WriteFile(ringGroupFile(dialplanDir, g.Name), []byte(renderRingGroup(g)), 0o644); err != nil {
			return err
		}
	}
	return nil
}
