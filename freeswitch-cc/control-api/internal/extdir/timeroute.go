package extdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

// 营业时间路由物化：
//   - 每个时间路由一份拨号计划 15_time-<name>.xml（前缀 15 保证抢在队列
//     30_ccq-*、IVR、会议 40_conf-* 之前匹配——拨接入号先命中时间路由再分流）。
//   - 每个 TimeRange 段生成一条 open extension（号码匹配 break=never 不消费 +
//     wday/time-of-day 时间条件；两条件同 extension 内为 AND）。命中即 transfer
//     到 OpenTarget；不命中的段落到下一 extension。
//   - 最后一条 closed 兜底（同号码无时间条件）：ClosedTarget 非空→transfer；
//     空→answer+sleep+playback ClosedPrompt（空则 misc/sorry.wav）+hangup。
//   - 落 dialplan 卷（→ /etc/freeswitch/dialplan/default），reloadxml 生效。
//
// 参考 FS 官方 time-of-day 路由：一个 extension 两个 condition，第一个匹配号码
// （默认 break=on-false：号码不匹配即跳出本 extension——不能用 break="never"，
// 那表示"失败也继续"，会让任意号码都落进时间条件被 transfer，实测造成全局
// 路由死循环 EXCHANGE_ROUTING_ERROR），第二个用 wday/time-of-day 匹配时间；
// 时间条件失败则整条 extension 跳过，落到下一个（下一时段或兜底）。

func timeRouteFile(dir, name string) string {
	return filepath.Join(dir, "15_time-"+name+".xml")
}

// renderTimeRoute 生成单个营业时间路由的拨号计划。
func renderTimeRoute(t store.CCTimeRoute) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- 营业时间路由 %s：拨 %s 按工作日/时段分流（前缀 15 抢在队列/IVR 前匹配） -->\n<include>\n",
		xmlEscaper.Replace(t.Name), t.AccessNumber)

	// 每个时段一条 open extension：号码匹配 break=never + 时间条件（AND）
	for i, rng := range splitTimeRanges(t.TimeRanges) {
		fmt.Fprintf(&b, `  <extension name="tr_%s_open%d">
    <condition field="destination_number" expression="^%s$"/>
    <condition wday="%s" time-of-day="%s">
      <action application="transfer" data="%s XML default"/>
    </condition>
  </extension>
`, t.Name, i+1, t.AccessNumber, xmlEscaper.Replace(t.Wdays), xmlEscaper.Replace(rng), xmlEscaper.Replace(t.OpenTarget))
	}

	// closed 兜底：同号码无时间条件，前面所有 open 段都没命中才走到这里
	fmt.Fprintf(&b, `  <extension name="tr_%s_closed">
    <condition field="destination_number" expression="^%s$">
`, t.Name, t.AccessNumber)
	if t.ClosedTarget != "" {
		fmt.Fprintf(&b, "      <action application=\"transfer\" data=\"%s XML default\"/>\n", xmlEscaper.Replace(t.ClosedTarget))
	} else {
		prompt := t.ClosedPrompt
		if prompt == "" {
			prompt = "misc/sorry.wav"
		}
		b.WriteString("      <action application=\"answer\"/>\n")
		b.WriteString("      <action application=\"sleep\" data=\"500\"/>\n")
		fmt.Fprintf(&b, "      <action application=\"playback\" data=\"%s\"/>\n", xmlEscaper.Replace(prompt))
		b.WriteString("      <action application=\"hangup\"/>\n")
	}
	b.WriteString("    </condition>\n  </extension>\n</include>\n")
	return b.String()
}

// splitTimeRanges 拆 "09:00-12:00,13:30-18:00" 为逐段；空/全空白返回 nil。
func splitTimeRanges(s string) []string {
	var out []string
	for _, seg := range strings.Split(s, ",") {
		if seg = strings.TrimSpace(seg); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// SyncTimeRoutes rewrites every time-route dialplan from DB rows.
// 先清旧 15_time-*.xml 再全量重写（删除时间路由场景）。
func SyncTimeRoutes(dialplanDir string, routes []store.CCTimeRoute) error {
	if err := os.MkdirAll(dialplanDir, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dialplanDir, "15_time-*.xml"))
	for _, f := range old {
		_ = os.Remove(f)
	}
	for _, t := range routes {
		if err := os.WriteFile(timeRouteFile(dialplanDir, t.Name), []byte(renderTimeRoute(t)), 0o644); err != nil {
			return err
		}
	}
	return nil
}
