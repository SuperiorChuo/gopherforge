// Package extdir materializes dynamic extensions as FreeSWITCH directory XML
// files. The files land in a volume shared with the FS container
// (/etc/freeswitch/directory/default)，写完由调用方 reloadxml 生效。DB 是
// 唯一事实源：启动时 Sync 全量重写，卷丢了也能自愈。
package extdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

// ext/password 由 API 层正则约束成安全字符集，label 这里再做 XML 转义兜底。
var xmlEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
)

// DialStringParam 是分机互拨必需的用户 param：bridge user/ 的拨号解析只认它
// （运行时 ${sofia_contact} 查注册表定位被叫），缺失时 FreeSWITCH 报
// "No dial-string available"、呼叫以 MANDATORY_IE_MISSING 失败——与分机是否
// 在线无关（2026-08-08 分机互打全挂的根因，当时所有分机 XML 都缺这个 param）。
// ${dialed_extension} 由内线拨号计划（dialplan/default/01_local_extensions.xml）
// export，${domain_name} 是 FS 全局域。
// 2026-08-10 升级为 vanilla 完整版（sip_invite_domain/presence_id 前缀）：
// FreeSWITCH 1.11 的 user/ originate 对简化版报 MANDATORY_IE_MISSING，
// 必须带该前缀；1.10 同样支持（标准写法）。extdir 物化文件与 fsxml 动态
// 目录共用，两处必须保持一致。
const DialStringParam = `<param name="dial-string" value="{^^:sip_invite_domain=${dialed_domain}:presence_id=${dialed_user}@${dialed_domain}}${sofia_contact(*/${dialed_extension}@${domain_name})}"/>`

func fileFor(dir, ext string) string {
	return filepath.Join(dir, ext+".xml")
}

func render(e store.Extension) string {
	label := e.Label
	if label == "" {
		label = "Ext " + e.Ext
	}
	return fmt.Sprintf(`<include>
  <user id="%s">
    <params>
      <param name="password" value="%s"/>
      %s%s
    </params>
    <variables>
      <variable name="toll_allow" value="domestic,local"/>
      <variable name="accountcode" value="%s"/>
      <variable name="user_context" value="default"/>
      <variable name="effective_caller_id_name" value="%s"/>
      <variable name="effective_caller_id_number" value="%s"/>
      <variable name="callgroup" value="default"/>%s
    </variables>
  </user>
</include>
`, e.Ext, xmlEscaper.Replace(e.Password), DialStringParam, VMParam(e), e.Ext, xmlEscaper.Replace(label), e.Ext, directoryVars(e))
}

// directoryVars 产出 mod_directory（411 按名字拨号）需要的用户变量：拼音名非空
// 时挂 directory-visible=true + first_name/last_name（mod_directory 扫这几个字段做
// 姓名匹配与报读）。两者都空则返回空串——该分机不进 411 目录。
// 字段名按 vanilla directory 示例（conf/directory/default 的用户 variables）来。
func directoryVars(e store.Extension) string {
	return DirectoryVars(e)
}

// DirectoryVars 是 directoryVars 的导出版，供 fsxml.go 动态目录复用（两处生成的
// directory <user> 必须字段一致）。返回的字符串以换行开头，直接拼进 variables 块尾部。
func DirectoryVars(e store.Extension) string {
	if e.DirFirstName == "" && e.DirLastName == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n      <variable name=\"directory-visible\" value=\"true\"/>")
	b.WriteString("\n      <variable name=\"directory-exten-visible\" value=\"true\"/>")
	if e.DirFirstName != "" {
		fmt.Fprintf(&b, "\n      <variable name=\"first_name\" value=\"%s\"/>", xmlEscaper.Replace(e.DirFirstName))
	}
	if e.DirLastName != "" {
		fmt.Fprintf(&b, "\n      <variable name=\"last_name\" value=\"%s\"/>", xmlEscaper.Replace(e.DirLastName))
	}
	return b.String()
}

// Write materializes one extension file (idempotent overwrite).
func Write(dir string, e store.Extension) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(fileFor(dir, e.Ext), []byte(render(e)), 0o644)
}

// Remove deletes the extension file; missing file is not an error.
func Remove(dir, ext string) error {
	err := os.Remove(fileFor(dir, ext))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Sync rewrites every dynamic extension file from the DB rows (boot-time
// self-heal). 不清理多余文件：目录里还住着镜像播种的静态分机 XML。
func Sync(dir string, exts []store.Extension) error {
	for _, e := range exts {
		if err := Write(dir, e); err != nil {
			return fmt.Errorf("materialize ext %s: %w", e.Ext, err)
		}
	}
	return nil
}

// VMPasswordFor 返回该分机语音信箱的有效密码：VMPassword 非空取之，否则同分机号。
func VMPasswordFor(e store.Extension) string {
	if e.VMPassword != "" {
		return e.VMPassword
	}
	return e.Ext
}

// VMParam 生成 <user> 内的 vm-password param 片段（含前导换行+缩进），
// 未开启语音信箱时返回空串。directory 物理文件与 mod_xml_curl 动态目录共用同一
// 渲染逻辑，保证两处 vm-password 一致。
func VMParam(e store.Extension) string {
	if !e.VMEnabled {
		return ""
	}
	return fmt.Sprintf("\n      <param name=\"vm-password\" value=\"%s\"/>", xmlEscaper.Replace(VMPasswordFor(e)))
}
