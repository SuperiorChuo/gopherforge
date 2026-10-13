package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// mod_xml_curl 动态目录：FS 每次做 directory 查询（注册鉴权 / 呼叫定位用户）
// 都会 application/x-www-form-urlencoded POST 到这里。命中即答 directory XML，
// 未命中/非 directory 段答 not found → FS 回退磁盘上的物化分机文件（双轨兜底）。
//
// 鉴权用 BasicAuth（凭据内嵌在 xml_curl gateway-url 里，不落 X-CC-Token 头）：
// 用户固定 "fs"，口令与 CC_API_TOKEN 同源。此路由挂在 token 鉴权组之外，
// 自带 BasicAuth，避免 FS 端再拼自定义头。

const fsXMLBasicUser = "fs"

// fsXMLNotFound 是 mod_xml_curl 的标准“未命中”应答，FS 收到后回退磁盘文件。
const fsXMLNotFound = `<document type="freeswitch/xml">
  <section name="result">
    <result status="not found"/>
  </section>
</document>
`

// ext/password 已由 API 层正则约束成安全字符集；label 再做一层 XML 转义兜底，
// 与 extdir 物化文件保持同样的转义策略。
var fsXMLEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
)

func (s *Server) fsXML(c *gin.Context) {
	// BasicAuth：口令与 CC_API_TOKEN 同源。APIToken 为空时（本地/测试）不鉴权。
	if s.Cfg.APIToken != "" {
		user, pass, ok := c.Request.BasicAuth()
		if !ok || user != fsXMLBasicUser || pass != s.Cfg.APIToken {
			c.Header("WWW-Authenticate", `Basic realm="fs-xml"`)
			c.String(http.StatusUnauthorized, "unauthorized")
			return
		}
	}

	section := c.PostForm("section")
	user := c.PostForm("user")
	domain := c.PostForm("domain")

	// 非 directory 段或没带 user（如 configuration/dialplan 绑定误投）→ not found，
	// FS 回退磁盘。本 binding 只订阅 directory，这里是防御性兜底。
	if section != "directory" || user == "" {
		s.fsXMLRespond(c, fsXMLNotFound)
		return
	}

	// 按 ext 查动态分机；domain 不匹配也容错（只按 ext，用 FS 送来的 domain 回填）。
	e, err := s.extLookup().GetExtension(user)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 动态库没有 → 让 FS 回退磁盘上的静态/已物化分机文件。
			s.fsXMLRespond(c, fsXMLNotFound)
			return
		}
		// 库异常也答 not found，避免 500 阻断注册/呼叫（磁盘文件仍可顶）。
		s.fsXMLRespond(c, fsXMLNotFound)
		return
	}

	s.fsXMLRespond(c, renderDirectoryXML(domain, *e))
}

func (s *Server) fsXMLRespond(c *gin.Context, body string) {
	c.Header("Content-Type", "text/xml")
	c.String(http.StatusOK, body)
}

// renderDirectoryXML 产出 mod_xml_curl directory 段应答。
// <user> 内的 params/variables 与 extdir 物化文件（directory/default/<ext>.xml）
// 字段对齐：password param、user_context/effective_caller_id_* 等 variables，
// 保证动态命中与文件兜底行为一致。411 目录字段（directory-visible/first_name/
// last_name）复用 extdir.DirectoryVars，两处一致。domain 为空时回落到
// $${domain}（FS 全局域）。
func renderDirectoryXML(domain string, e store.Extension) string {
	label := e.Label
	if label == "" {
		label = "Ext " + e.Ext
	}
	dom := domain
	if dom == "" {
		dom = "$${domain}"
	}
	// 语音信箱开启时补 vm-password param（与 extdir 物化文件同源，密码为空跟随分机号）。
	vmParam := ""
	if e.VMEnabled {
		vmParam = fmt.Sprintf("\n          <param name=\"vm-password\" value=\"%s\"/>", fsXMLEscaper.Replace(extdir.VMPasswordFor(e)))
	}
	return fmt.Sprintf(`<document type="freeswitch/xml">
  <section name="directory">
    <domain name="%s">
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
    </domain>
  </section>
</document>
`,
		fsXMLEscaper.Replace(dom),
		e.Ext,
		fsXMLEscaper.Replace(e.Password),
		extdir.DialStringParam,
		vmParam,
		e.Ext,
		fsXMLEscaper.Replace(label),
		e.Ext,
		extdir.DirectoryVars(e),
	)
}
