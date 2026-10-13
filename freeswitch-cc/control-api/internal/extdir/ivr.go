package extdir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

// IVR 物化：菜单 XML 落 ivr_menus 卷 + 接入号拨号计划落 dialplan 卷，
// reloadxml 生效。按键动作固定为白名单集合，target 由 API 层正则约束。

// DefaultGreeting 无外置音频也能听见的默认提示（1s 提示音）。
const DefaultGreeting = "tone_stream://%(1000,0,640)"

// CollectVar 收号存入的 channel 变量名。play_and_get_digits 存这里，之后
// export 到 A-leg，随 mod_json_cdr 的 variables 自然带出（cc-service 提取）。
const CollectVar = "cc_collected"

type IVREntry struct {
	Digits string `json:"digits"` // 0-9 * #
	Action string `json:"action"` // transfer | submenu | replay | back | hangup | collect
	// Target：transfer=目标号码（分机/队列/接入号）；submenu=子菜单名；
	// collect=收号参数串 "min:max:timeout:prompt:next"（min/max 位数、超时 ms、
	// 提示音文件、收号完成后转接号码）。冒号分隔，与现有字符串序列化兼容。
	Target string `json:"target"`
}

// CollectParams 是 collect 动作解析后的收号参数。
type CollectParams struct {
	Min     int    // 最小收号位数
	Max     int    // 最大收号位数
	Timeout int    // 单位 ms
	Prompt  string // 提示音文件（放音路径），空=默认提示音
	Next    string // 收号完成后转接目标号码（分机/队列/接入号）
}

// ParseCollect 解析 "min:max:timeout:prompt:next"。prompt 允许为空占位。
func ParseCollect(target string) (CollectParams, bool) {
	parts := strings.SplitN(target, ":", 5)
	if len(parts) != 5 {
		return CollectParams{}, false
	}
	min, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	max, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	timeout, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err1 != nil || err2 != nil || err3 != nil {
		return CollectParams{}, false
	}
	p := CollectParams{
		Min:     min,
		Max:     max,
		Timeout: timeout,
		Prompt:  strings.TrimSpace(parts[3]),
		Next:    strings.TrimSpace(parts[4]),
	}
	return p, true
}

func ParseIVREntries(raw string) ([]IVREntry, error) {
	var entries []IVREntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func ivrMenuFile(dir, name string) string     { return filepath.Join(dir, "ivr-"+name+".xml") }
func ivrDialplanFile(dir, name string) string { return filepath.Join(dir, "40_ivr-"+name+".xml") }

// collectDialplanExt 收号专用拨号计划的接入号（隐藏 extension，仅供 IVR 内部
// transfer 落点）。命名嵌 IVR 名与按键，保证多 IVR/多键不冲突。
func collectDialplanExt(ivr, digit string) string {
	return "cccollect_" + ivr + "_" + digitToken(digit)
}
func collectDialplanFile(dir, ivr, digit string) string {
	return filepath.Join(dir, "55_collect-"+ivr+"-"+digitToken(digit)+".xml")
}

// digitToken 把 * # 换成拨号计划名/文件名安全的 token。
func digitToken(digit string) string {
	switch digit {
	case "*":
		return "star"
	case "#":
		return "pound"
	default:
		return digit
	}
}

func renderIVRMenu(m store.IVRMenu, entries []IVREntry) string {
	greeting := m.Greeting
	if greeting == "" {
		greeting = DefaultGreeting
	}
	timeout := m.TimeoutMS
	if timeout <= 0 {
		timeout = 8000
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<include>
  <menu name="%s"
        greet-long="%s"
        greet-short="%s"
        invalid-sound="tone_stream://%%(250,250,480,620)"
        exit-sound="tone_stream://%%(500,0,320)"
        timeout="%d"
        inter-digit-timeout="2000"
        max-failures="3"
        max-timeouts="3"
        digit-len="1">
`, m.Name, xmlEscaper.Replace(greeting), xmlEscaper.Replace(greeting), timeout)
	for _, e := range entries {
		switch e.Action {
		case "transfer":
			fmt.Fprintf(&b, `    <entry action="menu-exec-app" digits="%s" param="transfer %s XML default"/>`+"\n", e.Digits, e.Target)
		case "submenu":
			fmt.Fprintf(&b, `    <entry action="menu-sub" digits="%s" param="%s"/>`+"\n", e.Digits, e.Target)
		case "replay":
			fmt.Fprintf(&b, `    <entry action="menu-top" digits="%s"/>`+"\n", e.Digits)
		case "back":
			fmt.Fprintf(&b, `    <entry action="menu-back" digits="%s"/>`+"\n", e.Digits)
		case "hangup":
			fmt.Fprintf(&b, `    <entry action="menu-exec-app" digits="%s" param="hangup"/>`+"\n", e.Digits)
		case "collect":
			// menu-exec-app 只能挂一个 app：这里 transfer 进收号专用隐藏拨号计划
			// （55_collect-<ivr>-<digit>.xml：answer→play_and_get_digits→export→
			// transfer next），把「收号 + 转接」两步拆到 extension 里干净完成。
			fmt.Fprintf(&b, `    <entry action="menu-exec-app" digits="%s" param="transfer %s XML default"/>`+"\n",
				e.Digits, collectDialplanExt(m.Name, e.Digits))
		}
	}
	b.WriteString("  </menu>\n</include>\n")
	return b.String()
}

// renderCollectDialplan 渲染收号专用隐藏 extension：
//
//	answer → play_and_get_digits <min> <max> 3 <timeout> # <prompt> silence_stream://250 cc_collected \d+
//	→ export cc_collected（带到 A-leg，随 CDR variables 带出）
//	→ transfer <next>（收号完成后转接）
//
// play_and_get_digits 参数序（官方）：<min> <max> <tries> <timeout> <terminators>
// <prompt_audio> <input_error_audio> <var_name> <regexp> [digit_timeout] [transfer_on_failure]
func renderCollectDialplan(ivr string, e IVREntry, p CollectParams) string {
	prompt := p.Prompt
	if prompt == "" {
		prompt = "silence_stream://500" // 无提示音也能进收号（兜底）
	}
	ext := collectDialplanExt(ivr, e.Digits)
	return fmt.Sprintf(`<!-- IVR %s 按 %s：收号（play_and_get_digits）→ 存 %s → 转 %s -->
<include>
  <extension name="%s">
    <condition field="destination_number" expression="^%s$">
      <action application="answer"/>
      <action application="sleep" data="300"/>
      <action application="play_and_get_digits" data="%d %d 3 %d # %s silence_stream://250 %s \d+"/>
      <action application="export" data="%s=${%s}"/>
      <action application="transfer" data="%s XML default"/>
    </condition>
  </extension>
</include>
`, ivr, e.Digits, CollectVar, xmlEscaper.Replace(p.Next),
		ext, ext,
		p.Min, p.Max, p.Timeout, xmlEscaper.Replace(prompt), CollectVar,
		CollectVar, CollectVar,
		xmlEscaper.Replace(p.Next))
}

func renderIVRDialplan(m store.IVRMenu) string {
	return fmt.Sprintf(`<!-- IVR %s：拨 %s 进入按键导航 -->
<include>
  <extension name="ivr_%s">
    <condition field="destination_number" expression="^%s$">
      <action application="answer"/>
      <action application="sleep" data="500"/>
      <action application="ivr" data="%s"/>
      <action application="hangup"/>
    </condition>
  </extension>
</include>
`, m.Name, m.AccessNumber, m.Name, m.AccessNumber, m.Name)
}

// WriteIVR materializes menu + dialplan entry for one IVR.
func WriteIVR(ivrDir, dialplanDir string, m store.IVRMenu) error {
	entries, err := ParseIVREntries(m.Entries)
	if err != nil {
		return fmt.Errorf("ivr %s entries: %w", m.Name, err)
	}
	if err := os.MkdirAll(ivrDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(ivrMenuFile(ivrDir, m.Name), []byte(renderIVRMenu(m, entries)), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(dialplanDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(ivrDialplanFile(dialplanDir, m.Name), []byte(renderIVRDialplan(m)), 0o644); err != nil {
		return err
	}
	// 收号专用隐藏拨号计划：先清本 IVR 旧的 collect 文件（收号按键改动/删除场景），
	// 再按当前 collect 按键重写，避免残留失效 extension。
	removeCollectDialplans(dialplanDir, m.Name)
	for _, e := range entries {
		if e.Action != "collect" {
			continue
		}
		p, ok := ParseCollect(e.Target)
		if !ok {
			return fmt.Errorf("ivr %s digit %s: invalid collect target %q", m.Name, e.Digits, e.Target)
		}
		if err := os.WriteFile(collectDialplanFile(dialplanDir, m.Name, e.Digits),
			[]byte(renderCollectDialplan(m.Name, e, p)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// removeCollectDialplans 清掉某 IVR 名下所有收号拨号计划文件。
func removeCollectDialplans(dialplanDir, name string) {
	old, _ := filepath.Glob(filepath.Join(dialplanDir, "55_collect-"+name+"-*.xml"))
	for _, f := range old {
		_ = os.Remove(f)
	}
}

// RemoveIVR deletes both materialized files.
func RemoveIVR(ivrDir, dialplanDir, name string) error {
	if err := removeIfExists(ivrMenuFile(ivrDir, name)); err != nil {
		return err
	}
	removeCollectDialplans(dialplanDir, name)
	return removeIfExists(ivrDialplanFile(dialplanDir, name))
}

// SyncIVRs rewrites all IVR files from DB rows (boot-time self-heal).
func SyncIVRs(ivrDir, dialplanDir string, list []store.IVRMenu) error {
	for _, m := range list {
		if err := WriteIVR(ivrDir, dialplanDir, m); err != nil {
			return err
		}
	}
	return nil
}
