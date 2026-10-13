package extdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

// mod_conference 会议室物化：
//   - 每个固定会议室一份拨号计划 40_conf-<name>.xml（拨 Number 应答后进
//     <name>@default 会议实例，可选人数上限/入会 PIN/进会自动录音）。
//   - 一份动态兜底 40_conf-dynamic.xml：拨 39xx 随拨随建 dyn-39xx@default。
//   - 落 dialplan 卷（→ /etc/freeswitch/dialplan/default），reloadxml 生效。
//     vanilla 默认已启用 mod_conference 且自带 default profile，无需额外配置。
//
// 文件名前缀 40_conf-* 避开其他批次（30_ccq-*、50_gw-*）。

const confDynamicFile = "40_conf-dynamic.xml"

func confFile(dir, name string) string {
	return filepath.Join(dir, "40_conf-"+name+".xml")
}

// renderConferenceDialplan 生成单个固定会议室的接入号拨号计划。
func renderConferenceDialplan(conf store.CCConference) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<!-- 会议室 %s：拨 %s 进会（mod_conference） -->
<include>
  <extension name="conf_%s">
    <condition field="destination_number" expression="^%s$">
      <action application="answer"/>
`, xmlEscaper.Replace(conf.Name), conf.Number, conf.Name, conf.Number)
	if conf.MaxMembers > 0 {
		fmt.Fprintf(&b, "      <action application=\"set\" data=\"max_members=%d\"/>\n", conf.MaxMembers)
	}
	if conf.AutoRecord {
		fmt.Fprintf(&b, "      <action application=\"set\" data=\"conference_auto_record=$${recordings_dir}/conf-%s-${strftime(%%Y%%m%%d-%%H%%M%%S)}.wav\"/>\n", conf.Name)
	}
	// PIN 非空时拼 +PIN，mod_conference 用它做入会密码
	confArg := conf.Name + "@default"
	if conf.PIN != "" {
		confArg += "+" + conf.PIN
	}
	fmt.Fprintf(&b, "      <action application=\"conference\" data=\"%s\"/>\n", confArg)
	b.WriteString("    </condition>\n  </extension>\n</include>\n")
	return b.String()
}

// renderDynamicConferenceDialplan 生成 39xx 随拨随建的兜底拨号计划。
func renderDynamicConferenceDialplan() string {
	return `<!-- 动态会议段：拨 39xx 随拨随建 dyn-39xx@default（无需预先配置） -->
<include>
  <extension name="conf_dynamic">
    <condition field="destination_number" expression="^(39[0-9]{2})$">
      <action application="answer"/>
      <action application="conference" data="dyn-$1@default"/>
    </condition>
  </extension>
</include>
`
}

// SyncConferences rewrites every conference dialplan (含动态兜底) from DB rows.
// 先清旧 40_conf-*.xml 再全量重写（删除会议室场景）。
func SyncConferences(dialplanDir string, rooms []store.CCConference) error {
	if err := os.MkdirAll(dialplanDir, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dialplanDir, "40_conf-*.xml"))
	for _, f := range old {
		_ = os.Remove(f)
	}
	for _, conf := range rooms {
		if err := os.WriteFile(confFile(dialplanDir, conf.Name), []byte(renderConferenceDialplan(conf)), 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dialplanDir, confDynamicFile), []byte(renderDynamicConferenceDialplan()), 0o644)
}
