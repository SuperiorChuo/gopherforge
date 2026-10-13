package api

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/authjwt"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// 呼叫控制代理：转发 control-api 的结构化端点（fsclient 带 X-CC-Token）。
// 入参在这里再校验一遍（纵深防御），格式与 control-api 侧一致。
//
// 越权收口（多租户）：
//   - 全平台运维数据/全局配置（注册清单、活动通道、分机/线路/IVR 增删改、录音与
//     停泊清单等）在 FS 侧没有租户概念，一律收口平台管理员（tenant=1）。
//   - 与租户可判定归属的操作（对某通话强拆/转移/保持/停泊/DTMF、按分机查留言、
//     下载录音）按 cc_calls / cc_agents 归属校验后放行本租户，平台管理员全放。

var (
	fsNumberRe = regexp.MustCompile(`^[0-9*#+]{2,32}$`)
	fsUUIDRe   = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)
)

// fsAdminProxyGet 只读代理，收口平台管理员（tenant=1）：全平台注册/通道/分机/
// 线路/IVR/录音等清单在 FS 侧无租户概念，无法按租户过滤，普通租户一律 403。
func (s *Server) fsAdminProxyGet(c *gin.Context, path string) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	s.fsProxyGetAuthed(c, path)
}

// callInTenant 校验通话归属：平台管理员放行任意通话；普通租户仅当 cc_calls 里
// 存在本租户的该通话记录时放行。查不到 = 判定不了归属，按越权拒绝（fail closed）。
// 返回 false 时已写响应。
func (s *Server) callInTenant(c *gin.Context, agent *authjwt.AgentClaims, callID string) bool {
	if agent.TenantID == 1 {
		return true
	}
	if _, err := s.Store.GetCall(c.Request.Context(), agent.TenantID, callID); err != nil {
		if store.IsNotFound(err) {
			fail(c, http.StatusForbidden, "call not in your tenant")
		} else {
			fail(c, http.StatusInternalServerError, "load call failed")
		}
		return false
	}
	return true
}

// extensionInTenant 校验分机归属：平台管理员放行任意分机；普通租户要求该分机在
// cc_agents 里绑定到本租户（未绑定 = 判定不了归属，拒绝）。返回 false 时已写响应。
func (s *Server) extensionInTenant(c *gin.Context, agent *authjwt.AgentClaims, ext string) bool {
	if agent.TenantID == 1 {
		return true
	}
	bound, err := s.Store.GetAgentByExtension(c.Request.Context(), agent.TenantID, ext)
	if err != nil {
		fail(c, http.StatusInternalServerError, "load extension binding failed")
		return false
	}
	if bound == nil {
		fail(c, http.StatusForbidden, "extension not in your tenant")
		return false
	}
	return true
}

// FSRegistrations handles GET /api/v1/cc/fs/registrations — 分机注册在线状态
// （全平台清单含各分机源 IP，仅平台管理员）。
func (s *Server) FSRegistrations(c *gin.Context) {
	s.fsAdminProxyGet(c, "/v1/registrations")
}

// FSChannels handles GET /api/v1/cc/fs/channels — 当前活动通话通道
// （全平台主被叫号码，仅平台管理员）。
func (s *Server) FSChannels(c *gin.Context) {
	s.fsAdminProxyGet(c, "/v1/channels")
}

type fsOriginateReq struct {
	FromExt string `json:"from_ext"`
	To      string `json:"to"`
	// Gateway 可选：SIP 线路名（sofia gateway），CRM 外呼选线路用。
	Gateway string `json:"gateway"`
}

var fsGatewayNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// FSOriginate handles POST /api/v1/cc/fs/originate — 点击外呼：坐席分机先振铃，
// 接起后按拨号计划路由被叫。发起前做预付费拦截（冻结/余额不足）并注入通话时长上限。
func (s *Server) FSOriginate(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var req fsOriginateReq
	if err := c.ShouldBindJSON(&req); err != nil || !fsNumberRe.MatchString(req.FromExt) || !fsNumberRe.MatchString(req.To) {
		fail(c, http.StatusBadRequest, "invalid from_ext or to")
		return
	}
	if req.Gateway != "" && !fsGatewayNameRe.MatchString(req.Gateway) {
		fail(c, http.StatusBadRequest, "invalid gateway")
		return
	}
	// 发起分机必须属于当前租户（cc_agents 绑定），防止冒用他租户分机外呼；
	// 平台管理员放行任意分机。
	if !s.extensionInTenant(c, agent, req.FromExt) {
		return
	}

	// 预付费拦截：账户冻结→403；命中费率且余额可用额≤0→402。
	// 命中费率且费率>0 时注入 execute_on_answer 定时挂断，接通后按可通话秒数强拆。
	// tenant_id 随发起注入 cc_tenant_id 通道变量（转售计费显式归属，CDR 带回）。
	body := gin.H{"from_ext": req.FromExt, "to": req.To, "gateway": req.Gateway, "tenant_id": agent.TenantID}
	ctx := c.Request.Context()
	acc, err := s.Store.GetOrCreateAccount(ctx, agent.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "load billing account failed")
		return
	}
	if acc.Status == "frozen" {
		fail(c, http.StatusForbidden, "账户已冻结")
		return
	}
	rate, err := s.Store.MatchRate(ctx, agent.TenantID, req.To)
	if err != nil {
		fail(c, http.StatusInternalServerError, "match rate failed")
		return
	}
	var ratePerMin int64
	if rate != nil && rate.RatePerMin > 0 {
		ratePerMin = rate.RatePerMin
		avail := acc.Balance + acc.CreditLimit
		if avail <= 0 {
			fail(c, http.StatusPaymentRequired, "余额不足")
			return
		}
		// 可通话秒数 = 可用额*60/每分钟费率，上限 4 小时。
		maxSec := avail * 60 / rate.RatePerMin
		if maxSec > 4*3600 {
			maxSec = 4 * 3600
		}
		if maxSec < 1 {
			maxSec = 1
		}
		// 接通后定时挂断（单引号包变量，与现有 originate 变量段约定一致）。
		body["execute_on_answer"] = fmt.Sprintf("sched_hangup +%d alloted_timeout", maxSec)
	}
	// 未命中费率 = 不拦截、不限时。空风控策略同样放行。
	if !s.enforceOutboundRisk(c, agent.TenantID, req.FromExt, req.To, ratePerMin) {
		return
	}

	out, err := s.FS.Post(ctx, "/v1/calls/originate", body)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// ---- 分机管理 ----

var (
	fsExtRe      = regexp.MustCompile(`^[0-9]{4,6}$`)
	fsExtPassRe  = regexp.MustCompile(`^[A-Za-z0-9]{4,32}$`)
	fsVMPassRe   = regexp.MustCompile(`^[0-9]{4,8}$`)
	fsRecordFile = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.wav$`)
)

// FSExtensions handles GET /api/v1/cc/fs/extensions — 静态+动态分机清单
// （全平台配置，FS 侧无租户概念，仅平台管理员）。
func (s *Server) FSExtensions(c *gin.Context) {
	s.fsAdminProxyGet(c, "/v1/extensions")
}

type fsCreateExtensionReq struct {
	Ext        string `json:"ext"`
	Password   string `json:"password"`
	Label      string `json:"label"`
	VMEnabled  bool   `json:"vm_enabled"`
	VMPassword string `json:"vm_password"`
	// 411 目录拼音名
	DirFirstName string `json:"dir_first_name"`
	DirLastName  string `json:"dir_last_name"`
}

// fsDirNameRe 411 目录拼音名：0-32 位小写字母（与 control-api 同）。
var fsDirNameRe = regexp.MustCompile(`^[a-z]{0,32}$`)

// FSCreateExtension handles POST /api/v1/cc/fs/extensions — 动态创建分机
// （全局 SIP 目录写操作，仅平台管理员）。
func (s *Server) FSCreateExtension(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var req fsCreateExtensionReq
	if err := c.ShouldBindJSON(&req); err != nil || !fsExtRe.MatchString(req.Ext) || !fsExtPassRe.MatchString(req.Password) {
		fail(c, http.StatusBadRequest, "invalid ext or password")
		return
	}
	// 语音信箱密码：4-8 位数字（空=同分机号，交对端处理）
	if req.VMPassword != "" && !fsVMPassRe.MatchString(req.VMPassword) {
		fail(c, http.StatusBadRequest, "invalid voicemail password")
		return
	}
	if !fsDirNameRe.MatchString(req.DirFirstName) || !fsDirNameRe.MatchString(req.DirLastName) {
		fail(c, http.StatusBadRequest, "dir name must be 0-32 lowercase letters (pinyin)")
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/extensions", gin.H{
		"ext": req.Ext, "password": req.Password, "label": req.Label,
		"vm_enabled": req.VMEnabled, "vm_password": req.VMPassword,
		"dir_first_name": req.DirFirstName, "dir_last_name": req.DirLastName,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSEditExtension handles PUT /api/v1/cc/fs/extensions/:ext — 更新动态分机
// （改密码/名称/语音信箱，密码必填——无默认密码；仅平台管理员）。
func (s *Server) FSEditExtension(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	ext := c.Param("ext")
	if !fsExtRe.MatchString(ext) {
		fail(c, http.StatusBadRequest, "invalid ext")
		return
	}
	var req fsCreateExtensionReq
	if err := c.ShouldBindJSON(&req); err != nil || !fsExtPassRe.MatchString(req.Password) {
		fail(c, http.StatusBadRequest, "password must be 4-32 alphanumeric")
		return
	}
	if req.VMPassword != "" && !fsVMPassRe.MatchString(req.VMPassword) {
		fail(c, http.StatusBadRequest, "invalid voicemail password")
		return
	}
	if !fsDirNameRe.MatchString(req.DirFirstName) || !fsDirNameRe.MatchString(req.DirLastName) {
		fail(c, http.StatusBadRequest, "dir name must be 0-32 lowercase letters (pinyin)")
		return
	}
	out, err := s.FS.Put(c.Request.Context(), "/v1/extensions/"+ext, gin.H{
		"ext": req.Ext, "password": req.Password, "label": req.Label,
		"vm_enabled": req.VMEnabled, "vm_password": req.VMPassword,
		"dir_first_name": req.DirFirstName, "dir_last_name": req.DirLastName,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSDeleteExtension handles DELETE /api/v1/cc/fs/extensions/:ext（仅平台管理员）。
func (s *Server) FSDeleteExtension(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	ext := c.Param("ext")
	if !fsExtRe.MatchString(ext) {
		fail(c, http.StatusBadRequest, "invalid ext")
		return
	}
	out, err := s.FS.Delete(c.Request.Context(), "/v1/extensions/"+ext)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// ---- SIP 网关（线路）----

var fsGwNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// FSGateways handles GET /api/v1/cc/fs/gateways — 线路清单 + 注册状态
// （含各租户线路与 SIP 凭据侧信息，仅平台管理员）。
func (s *Server) FSGateways(c *gin.Context) {
	s.fsAdminProxyGet(c, "/v1/gateways")
}

// fsLcrNumRe LCR 预览号码：3-32 位 [0-9*#]（与 control-api 侧一致）。
var fsLcrNumRe = regexp.MustCompile(`^[0-9*#]{3,32}$`)

// FSLCRPreview handles GET /api/v1/cc/fs/lcr-preview?number=... — 选路预览代理
// （暴露全平台线路与路由策略，仅平台管理员）。
func (s *Server) FSLCRPreview(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	number := strings.TrimSpace(c.Query("number"))
	if !fsLcrNumRe.MatchString(number) {
		fail(c, http.StatusBadRequest, "number must be 3-32 of [0-9*#]")
		return
	}
	out, err := s.FS.Get(c.Request.Context(), "/v1/lcr-preview?number="+url.QueryEscape(number))
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSCreateGateway handles POST /api/v1/cc/fs/gateways（细粒度校验在 control-api 侧；
// 全局线路配置写操作，仅平台管理员）。
func (s *Server) FSCreateGateway(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/gateways", body)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSUpdateGateway handles PUT /api/v1/cc/fs/gateways/:name（仅平台管理员）。
func (s *Server) FSUpdateGateway(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	name := c.Param("name")
	if !fsGwNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := s.FS.Put(c.Request.Context(), "/v1/gateways/"+name, body)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSDeleteGateway handles DELETE /api/v1/cc/fs/gateways/:name（仅平台管理员）。
func (s *Server) FSDeleteGateway(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	name := c.Param("name")
	if !fsGwNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	out, err := s.FS.Delete(c.Request.Context(), "/v1/gateways/"+name)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// ---- IVR ----
// IVR 在 FS 侧是全局拨号计划配置、无租户归属，列查删改一律仅平台管理员。

// FSIVRs handles GET /api/v1/cc/fs/ivrs（仅平台管理员）。
func (s *Server) FSIVRs(c *gin.Context) {
	s.fsAdminProxyGet(c, "/v1/ivrs")
}

// FSCreateIVR handles POST /api/v1/cc/fs/ivrs。
func (s *Server) FSCreateIVR(c *gin.Context) {
	s.fsProxyWriteIVR(c, "", http.MethodPost)
}

// FSUpdateIVR handles PUT /api/v1/cc/fs/ivrs/:name。
func (s *Server) FSUpdateIVR(c *gin.Context) {
	s.fsProxyWriteIVR(c, c.Param("name"), http.MethodPut)
}

func (s *Server) fsProxyWriteIVR(c *gin.Context, name, method string) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	if name != "" && !fsGwNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	var (
		out map[string]any
		err error
	)
	if method == http.MethodPut {
		out, err = s.FS.Put(c.Request.Context(), "/v1/ivrs/"+name, body)
	} else {
		out, err = s.FS.Post(c.Request.Context(), "/v1/ivrs", body)
	}
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSDeleteIVR handles DELETE /api/v1/cc/fs/ivrs/:name（仅平台管理员）。
func (s *Server) FSDeleteIVR(c *gin.Context) {
	if _, authorized := s.requirePlatformAdmin(c); !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	name := c.Param("name")
	if !fsGwNameRe.MatchString(name) {
		fail(c, http.StatusBadRequest, "invalid name")
		return
	}
	out, err := s.FS.Delete(c.Request.Context(), "/v1/ivrs/"+name)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// ---- 录音 ----

// FSRecordings handles GET /api/v1/cc/fs/recordings — 录音文件列表。FS 侧只有
// 文件名/大小、无租户字段，无法按租户过滤，仅平台管理员。
func (s *Server) FSRecordings(c *gin.Context) {
	s.fsAdminProxyGet(c, "/v1/recordings")
}

// FSRecordingFile handles GET /api/v1/cc/fs/recordings/:file — 流式透传 wav。
// 归属校验：普通租户仅当该文件出现在本租户话单（cc_calls.recording）里才放行；
// 平台管理员全放。
func (s *Server) FSRecordingFile(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	file := c.Param("file")
	if !fsRecordFile.MatchString(file) {
		fail(c, http.StatusBadRequest, "invalid file name")
		return
	}
	if agent.TenantID != 1 {
		if _, err := s.Store.FindCallByRecording(c.Request.Context(), agent.TenantID, file); err != nil {
			if store.IsNotFound(err) {
				fail(c, http.StatusForbidden, "recording not in your tenant")
			} else {
				fail(c, http.StatusInternalServerError, "load recording ownership failed")
			}
			return
		}
	}
	resp, err := s.FS.Stream(c.Request.Context(), "/v1/recordings/"+file)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, map[string]string{
		"Content-Disposition": resp.Header.Get("Content-Disposition"),
	})
}

// ---- 语音信箱留言 ----

var fsVMExtRe = regexp.MustCompile(`^[0-9]{2,6}$`)

// FSVoicemails handles GET /api/v1/cc/fs/voicemails?ext= — 留言文件列表。
// 带 ext：校验该分机绑定在本租户（cc_agents）后放行；不带 ext（全平台清单）
// 仅平台管理员。
func (s *Server) FSVoicemails(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	ext := c.Query("ext")
	if ext == "" {
		if agent.TenantID != 1 {
			fail(c, http.StatusForbidden, "platform admin only")
			return
		}
		s.fsProxyGetAuthed(c, "/v1/voicemails")
		return
	}
	if !fsVMExtRe.MatchString(ext) {
		fail(c, http.StatusBadRequest, "invalid ext")
		return
	}
	if !s.extensionInTenant(c, agent, ext) {
		return
	}
	s.fsProxyGetAuthed(c, "/v1/voicemails?ext="+ext)
}

// fsProxyGetAuthed 同 fsProxyGet 但调用方已完成鉴权/归属校验。
func (s *Server) fsProxyGetAuthed(c *gin.Context, path string) {
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	out, err := s.FS.Get(c.Request.Context(), path)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSVoicemailFile handles GET /api/v1/cc/fs/voicemails/:ext/:file — 流式透传 wav。
// 校验该分机绑定在本租户；平台管理员全放。
func (s *Server) FSVoicemailFile(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	ext, file := c.Param("ext"), c.Param("file")
	if !fsVMExtRe.MatchString(ext) || !fsRecordFile.MatchString(file) {
		fail(c, http.StatusBadRequest, "invalid ext or file name")
		return
	}
	if !s.extensionInTenant(c, agent, ext) {
		return
	}
	resp, err := s.FS.Stream(c.Request.Context(), "/v1/voicemails/"+ext+"/"+file)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, map[string]string{
		"Content-Disposition": resp.Header.Get("Content-Disposition"),
	})
}

// FSVoicemailDelete handles DELETE /api/v1/cc/fs/voicemails/:ext/:file。
// 校验该分机绑定在本租户；平台管理员全放。
func (s *Server) FSVoicemailDelete(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	ext, file := c.Param("ext"), c.Param("file")
	if !fsVMExtRe.MatchString(ext) || !fsRecordFile.MatchString(file) {
		fail(c, http.StatusBadRequest, "invalid ext or file name")
		return
	}
	if !s.extensionInTenant(c, agent, ext) {
		return
	}
	s.fsProxyDeleteAuthed(c, "/v1/voicemails/"+ext+"/"+file)
}

type fsHangupReq struct {
	CallID string `json:"call_id"`
}

// FSHangup handles POST /api/v1/cc/fs/hangup — 强拆一路通道。
// 归属校验：普通租户仅可强拆本租户话单（cc_calls）里的通话；平台管理员全放。
func (s *Server) FSHangup(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var req fsHangupReq
	if err := c.ShouldBindJSON(&req); err != nil || !fsUUIDRe.MatchString(req.CallID) {
		fail(c, http.StatusBadRequest, "invalid call_id")
		return
	}
	if !s.callInTenant(c, agent, req.CallID) {
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/calls/hangup", gin.H{"call_id": req.CallID})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// ---- 坐席通话操控（转移/保持/停泊/DTMF）----
// 都以 call_id 定位通道，统一按 callInTenant 校验归属：普通租户只能操控本租户
// 话单里的通话，平台管理员全放。

var (
	fsValetSlotRe = regexp.MustCompile(`^61[0-9]{2}$`)
	fsDTMFRe      = regexp.MustCompile(`^[0-9*#]{1,32}$`)
)

type fsTransferReq struct {
	CallID string `json:"call_id"`
	To     string `json:"to"`
	Both   bool   `json:"both"`
}

// FSTransfer handles POST /api/v1/cc/fs/transfer — 转移通话到目标号码。
func (s *Server) FSTransfer(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var req fsTransferReq
	if err := c.ShouldBindJSON(&req); err != nil || !fsUUIDRe.MatchString(req.CallID) || !fsNumberRe.MatchString(req.To) {
		fail(c, http.StatusBadRequest, "invalid call_id or to")
		return
	}
	if !s.callInTenant(c, agent, req.CallID) {
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/calls/transfer", gin.H{
		"call_id": req.CallID, "to": req.To, "both": req.Both,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

type fsHoldReq struct {
	CallID string `json:"call_id"`
	Action string `json:"action"`
}

// FSHold handles POST /api/v1/cc/fs/hold — 通话保持/恢复/切换。
func (s *Server) FSHold(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var req fsHoldReq
	if err := c.ShouldBindJSON(&req); err != nil || !fsUUIDRe.MatchString(req.CallID) {
		fail(c, http.StatusBadRequest, "invalid call_id")
		return
	}
	switch req.Action {
	case "on", "off", "toggle":
	default:
		fail(c, http.StatusBadRequest, "action must be on/off/toggle")
		return
	}
	if !s.callInTenant(c, agent, req.CallID) {
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/calls/hold", gin.H{
		"call_id": req.CallID, "action": req.Action,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

type fsParkReq struct {
	CallID string `json:"call_id"`
	Slot   string `json:"slot"`
}

// FSPark handles POST /api/v1/cc/fs/park — 把通话停泊到指定车位。
func (s *Server) FSPark(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var req fsParkReq
	if err := c.ShouldBindJSON(&req); err != nil || !fsUUIDRe.MatchString(req.CallID) || !fsValetSlotRe.MatchString(req.Slot) {
		fail(c, http.StatusBadRequest, "invalid call_id or slot")
		return
	}
	if !s.callInTenant(c, agent, req.CallID) {
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/calls/park", gin.H{
		"call_id": req.CallID, "slot": req.Slot,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

type fsDTMFReq struct {
	CallID string `json:"call_id"`
	Digits string `json:"digits"`
}

// FSDTMF handles POST /api/v1/cc/fs/dtmf — 向通话发送 DTMF 按键。
func (s *Server) FSDTMF(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var req fsDTMFReq
	if err := c.ShouldBindJSON(&req); err != nil || !fsUUIDRe.MatchString(req.CallID) || !fsDTMFRe.MatchString(req.Digits) {
		fail(c, http.StatusBadRequest, "invalid call_id or digits")
		return
	}
	if !s.callInTenant(c, agent, req.CallID) {
		return
	}
	out, err := s.FS.Post(c.Request.Context(), "/v1/calls/dtmf", gin.H{
		"call_id": req.CallID, "digits": req.Digits,
	})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, out)
}

// FSParked handles GET /api/v1/cc/fs/parked — 列出停泊车位里的通话
// （全平台车位含各租户主叫号，仅平台管理员）。
func (s *Server) FSParked(c *gin.Context) {
	s.fsAdminProxyGet(c, "/v1/parked")
}
