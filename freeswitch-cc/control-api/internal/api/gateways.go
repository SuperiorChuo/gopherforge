package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SIP 网关（线路）：DB → XML 物化到 external profile 卷 → sofia rescan 生效。
// 名称/地址/前缀均白名单校验后才进 XML 与 ESL 命令。

var (
	gwNameRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)
	gwHostRe    = regexp.MustCompile(`^[A-Za-z0-9.\-]{2,64}(:\d{2,5})?$`)
	gwUserRe    = regexp.MustCompile(`^[A-Za-z0-9._\-+]{0,64}$`)
	gwPassRe    = regexp.MustCompile(`^[^<>&"'\s]{0,64}$`)
	gwPrefixRe  = regexp.MustCompile(`^[0-9*#]{0,6}$`)
	gwPrependRe = regexp.MustCompile(`^[0-9*#+]{0,16}$`)
	gwNumberRe  = regexp.MustCompile(`^[0-9+]{0,32}$`)
	gwExtRe     = regexp.MustCompile(`^[0-9]{0,16}$`)
)

// listGateways handles GET /v1/gateways — DB 清单 + 实时注册状态。
func (s *Server) listGateways(c *gin.Context) {
	list, err := s.Store.ListGateways()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type view struct {
		store.Gateway
		State string `json:"state"`
	}
	out := make([]view, 0, len(list))
	for _, g := range list {
		v := view{Gateway: g, State: "UNKNOWN"}
		if raw, err := s.ESL.API("sofia status gateway " + g.Name); err == nil {
			v.State = parseGatewayState(raw)
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, gin.H{"list": out, "total": len(out)})
}

// parseGatewayState digs the State line out of `sofia status gateway` output;
// gateway 不存在时 FS 回 "Invalid Gateway!"。
func parseGatewayState(raw string) string {
	if strings.Contains(raw, "Invalid Gateway") {
		return "NOT_LOADED"
	}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "State" {
			return fields[len(fields)-1]
		}
	}
	return "UNKNOWN"
}

type gatewayReq struct {
	Name              string `json:"name" binding:"required"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	Realm             string `json:"realm" binding:"required"`
	Proxy             string `json:"proxy"`
	Register          *bool  `json:"register"`
	FromUser          string `json:"from_user"`
	FromDomain        string `json:"from_domain"`
	Extension         string `json:"extension"`
	RegisterTransport string `json:"register_transport"`
	ExpireSeconds     int    `json:"expire_seconds"`
	RetrySeconds      int    `json:"retry_seconds"`
	PingSeconds       int    `json:"ping_seconds"`
	CallerIDInFrom    bool   `json:"caller_id_in_from"`
	DialPrefix        string `json:"dial_prefix"`
	Prepend           string `json:"prepend"`
	CallerNumber      string `json:"caller_number"`
	MaxConcurrent     int    `json:"max_concurrent"`
	// LCR 费率选路：RatePerMin 分/分钟（≥0，0=未设→最贵）；SortWeight 同费率手动排序
	RatePerMin int64 `json:"rate_per_min"`
	SortWeight int   `json:"sort_weight"`
	// TenantID 线路归属租户（转售计费）：>0 时出局通话 export cc_tenant_id
	TenantID uint64 `json:"tenant_id"`
}

// validate 归一化并校验请求；出错时已写响应，返回 nil。
func (r *gatewayReq) validate(c *gin.Context) *store.Gateway {
	fail := func(msg string) *store.Gateway {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return nil
	}
	switch {
	case !gwNameRe.MatchString(r.Name):
		return fail("name must be 2-32 lowercase letters/digits/_-")
	case !gwHostRe.MatchString(r.Realm) || (r.Proxy != "" && !gwHostRe.MatchString(r.Proxy)):
		return fail("invalid realm or proxy")
	case !gwUserRe.MatchString(r.Username) || !gwPassRe.MatchString(r.Password):
		return fail("invalid username or password")
	case !gwUserRe.MatchString(r.FromUser) || (r.FromDomain != "" && !gwHostRe.MatchString(r.FromDomain)):
		return fail("invalid from_user or from_domain")
	case !gwExtRe.MatchString(r.Extension):
		return fail("invalid extension")
	case r.RegisterTransport != "" && r.RegisterTransport != "udp" && r.RegisterTransport != "tcp" && r.RegisterTransport != "tls":
		return fail("register_transport must be udp/tcp/tls")
	case r.ExpireSeconds != 0 && (r.ExpireSeconds < 60 || r.ExpireSeconds > 86400):
		return fail("expire_seconds must be 60-86400")
	case r.RetrySeconds != 0 && (r.RetrySeconds < 5 || r.RetrySeconds > 3600):
		return fail("retry_seconds must be 5-3600")
	case r.PingSeconds != 0 && (r.PingSeconds < 5 || r.PingSeconds > 3600):
		return fail("ping_seconds must be 5-3600 (0=off)")
	case !gwPrefixRe.MatchString(r.DialPrefix):
		return fail("dial_prefix must be 0-6 of [0-9*#]")
	case !gwPrependRe.MatchString(r.Prepend):
		return fail("prepend must be 0-16 of [0-9*#+]")
	case !gwNumberRe.MatchString(r.CallerNumber):
		return fail("invalid caller_number")
	case r.MaxConcurrent < 0 || r.MaxConcurrent > 500:
		return fail("max_concurrent must be 0-500 (0=unlimited)")
	case r.RatePerMin < 0:
		return fail("rate_per_min must be >= 0 (cents/min, 0=unset)")
	case r.SortWeight < 0:
		return fail("sort_weight must be >= 0")
	}
	register := true
	if r.Register != nil {
		register = *r.Register
	}
	if register && (r.Username == "" || r.Password == "") {
		return fail("register mode requires username and password")
	}
	return &store.Gateway{
		Name: r.Name, Username: r.Username, Password: r.Password,
		Realm: r.Realm, Proxy: r.Proxy, Register: register,
		FromUser: r.FromUser, FromDomain: r.FromDomain, Extension: r.Extension,
		RegisterTransport: r.RegisterTransport,
		ExpireSeconds:     r.ExpireSeconds, RetrySeconds: r.RetrySeconds, PingSeconds: r.PingSeconds,
		CallerIDInFrom: r.CallerIDInFrom,
		DialPrefix:     r.DialPrefix, Prepend: r.Prepend, CallerNumber: r.CallerNumber,
		MaxConcurrent: r.MaxConcurrent,
		RatePerMin:    r.RatePerMin, SortWeight: r.SortWeight,
		TenantID:      r.TenantID,
	}
}

// createGateway handles POST /v1/gateways.
func (s *Server) createGateway(c *gin.Context) {
	var req gatewayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and realm required"})
		return
	}
	g := req.validate(c)
	if g == nil {
		return
	}
	if err := s.Store.CreateGateway(g); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "gateway already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extdir.WriteGatewayProfile(s.Cfg.GwDir, *g); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize xml: " + err.Error()})
		return
	}
	if err := s.syncGatewayDialplans(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize dialplan: " + err.Error()})
		return
	}
	applied := s.applyGatewayChange("")
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": g.Name, "applied": applied})
}

// updateGateway handles PUT /v1/gateways/:name — 名称不可改，其余全量更新。
// killgw 摘掉旧运行态后 rescan 按新 XML 重建（注册型会重新 REGISTER）。
func (s *Server) updateGateway(c *gin.Context) {
	name := c.Param("name")
	if !gwNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	existing, err := s.Store.GetGateway(name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "gateway not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req gatewayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Name = name
	g := req.validate(c)
	if g == nil {
		return
	}
	g.ID = existing.ID
	g.CreatedAt = existing.CreatedAt
	// 停用态不随编辑翻转：欠费冻结期间改线路参数，不得顺手把线路复活
	// （恢复只走租户 state 端点）。
	g.Disabled = existing.Disabled
	if err := s.Store.UpdateGateway(g); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extdir.WriteGatewayProfile(s.Cfg.GwDir, *g); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize xml: " + err.Error()})
		return
	}
	if err := s.syncGatewayDialplans(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize dialplan: " + err.Error()})
		return
	}
	applied := s.applyGatewayChange(name)
	c.JSON(http.StatusOK, gin.H{"ok": true, "applied": applied})
}

// deleteGateway handles DELETE /v1/gateways/:name.
func (s *Server) deleteGateway(c *gin.Context) {
	name := c.Param("name")
	if !gwNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	if err := s.Store.DeleteGateway(name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "gateway not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extdir.RemoveGatewayProfile(s.Cfg.GwDir, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "remove xml: " + err.Error()})
		return
	}
	if err := s.syncGatewayDialplans(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize dialplan: " + err.Error()})
		return
	}
	applied := s.applyGatewayChange(name)
	c.JSON(http.StatusOK, gin.H{"ok": true, "applied": applied})
}

type tenantGatewaysStateReq struct {
	Disabled *bool `json:"disabled" binding:"required"`
}

// setTenantGatewaysState handles POST /v1/tenants/:tenant_id/gateways/state —
// 批量停用/恢复某租户名下全部线路（中台欠费冻结/解冻联动）。停用：删物化
// XML + killgw（注册断、入局断）+ 拨号计划剔除（字冠直拨断）；恢复：重物化 +
// rescan 重建。DB 行始终保留。
func (s *Server) setTenantGatewaysState(c *gin.Context) {
	tenantID, err := strconv.ParseUint(c.Param("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}
	var req tenantGatewaysStateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Disabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "disabled (bool) required"})
		return
	}
	names, err := s.Store.SetGatewaysDisabledByTenant(tenantID, *req.Disabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(names) == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": true, "affected": []string{}, "applied": true})
		return
	}
	// 重物化：按新状态写/删每条线路的 XML，再全量重写拨号计划。
	all, err := s.Store.ListGateways()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	affected := map[string]bool{}
	for _, n := range names {
		affected[n] = true
	}
	for _, g := range all {
		if !affected[g.Name] {
			continue
		}
		if err := extdir.WriteGatewayProfile(s.Cfg.GwDir, g); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize xml: " + err.Error()})
			return
		}
	}
	if err := extdir.SyncGatewayDialplans(s.Cfg.DialplanDir, all); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize dialplan: " + err.Error()})
		return
	}
	// ESL 应用：reloadxml 后逐条 killgw 摘运行态（停用即断；恢复时 rescan 按新
	// XML 重建，注册型重新 REGISTER）。尽力而为，FS 不在时启动自愈对齐。
	applied := true
	if _, err := s.ESL.API("reloadxml"); err != nil {
		applied = false
	}
	for _, n := range names {
		if _, err := s.ESL.API("sofia profile external killgw " + n); err != nil {
			applied = false
		}
	}
	if _, err := s.ESL.API("sofia profile external rescan"); err != nil {
		applied = false
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "affected": names, "disabled": *req.Disabled, "applied": applied})
}

// syncGatewayDialplans 从 DB 全集重渲染出局拨号计划（LCR 分组需要网关全集）。
// 任何网关增删改后调用：先清旧文件再全量写，failover 顺序随费率实时更新。
func (s *Server) syncGatewayDialplans() error {
	all, err := s.Store.ListGateways()
	if err != nil {
		return err
	}
	return extdir.SyncGatewayDialplans(s.Cfg.DialplanDir, all)
}

// applyGatewayChange reloads XML then rescans the external profile；删除时先
// killgw 摘掉运行态网关。尽力而为：FS 不在时下次启动 seed+自愈也能对齐。
func (s *Server) applyGatewayChange(killName string) bool {
	ok := true
	if _, err := s.ESL.API("reloadxml"); err != nil {
		ok = false
	}
	if killName != "" {
		if _, err := s.ESL.API("sofia profile external killgw " + killName); err != nil {
			ok = false
		}
	}
	if _, err := s.ESL.API("sofia profile external rescan"); err != nil {
		ok = false
	}
	return ok
}

// lcrPreviewNumRe 选路预览的号码入参：3-32 位数字（含可能的字冠），够长才有意义。
var lcrPreviewNumRe = regexp.MustCompile(`^[0-9*#]{3,32}$`)

// lcrPreview handles GET /v1/lcr-preview?number=... —— 纯内存模拟 LCR 选路，
// 返回命中最长字冠组的 failover 序列（次序/网关/费率），不打 FS。
func (s *Server) lcrPreview(c *gin.Context) {
	number := strings.TrimSpace(c.Query("number"))
	if !lcrPreviewNumRe.MatchString(number) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "number must be 3-32 of [0-9*#]"})
		return
	}
	all, err := s.Store.ListGateways()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rows := extdir.LCRPreview(all, number)
	c.JSON(http.StatusOK, gin.H{"number": number, "matched": len(rows) > 0, "list": rows, "total": len(rows)})
}
