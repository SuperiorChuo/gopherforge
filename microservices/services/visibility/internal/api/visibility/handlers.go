package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"gorm.io/gorm"
)

// ---------- Overview ----------

func (s *Server) Overview(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	days := 30
	if v := c.Query("days"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			days = n
		}
	}
	stats, err := s.Store.Overview(u.TenantID, days, s.DemoMode)
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// demo_mode 供总览页决定是否渲染「初始化演示数据」入口（路由本身也只在
	// 演示模式下挂载，见 RegisterRoutes）。
	writeOK(c, gin.H{"stats": stats, "days": days, "demo_mode": s.DemoMode})
}

// ---------- Brands ----------

func (s *Server) ListBrands(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListBrands(u.TenantID, c.Query("kind"), pageOf(c))
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"list": list, "total": total})
}

type brandReq struct {
	Name    string `json:"name"`
	Aliases string `json:"aliases"`
	Kind    string `json:"kind"`
	Website string `json:"website"`
	Note    string `json:"note"`
	Status  *int16 `json:"status"`
}

func (s *Server) CreateBrand(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req brandReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeFail(c, http.StatusBadRequest, "name required")
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "brand"
	}
	if kind != "brand" && kind != "competitor" {
		writeFail(c, http.StatusBadRequest, "kind must be brand|competitor")
		return
	}
	b := &model.Brand{
		TenantID: u.TenantID,
		Name:     strings.TrimSpace(req.Name),
		Aliases:  strings.TrimSpace(req.Aliases),
		Kind:     kind,
		Website:  strings.TrimSpace(req.Website),
		Note:     strings.TrimSpace(req.Note),
		Status:   1,
	}
	if err := s.Store.CreateBrand(c.Request.Context(), b); err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, b)
}

func (s *Server) UpdateBrand(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	var req brandReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeFail(c, http.StatusBadRequest, "invalid body")
		return
	}
	updates := map[string]any{}
	if n := strings.TrimSpace(req.Name); n != "" {
		updates["name"] = n
	}
	updates["aliases"] = strings.TrimSpace(req.Aliases)
	if k := strings.TrimSpace(req.Kind); k == "brand" || k == "competitor" {
		updates["kind"] = k
	}
	updates["website"] = strings.TrimSpace(req.Website)
	updates["note"] = strings.TrimSpace(req.Note)
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if err := s.Store.UpdateBrand(c.Request.Context(), u.TenantID, id, updates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeFail(c, http.StatusNotFound, "not found")
			return
		}
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	b, _ := s.Store.GetBrand(u.TenantID, id)
	writeOK(c, b)
}

func (s *Server) DeleteBrand(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	if err := s.Store.DeleteBrand(c.Request.Context(), u.TenantID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeFail(c, http.StatusNotFound, "not found")
			return
		}
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"deleted": true})
}

// ---------- Keywords ----------

func (s *Server) ListKeywords(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListKeywords(u.TenantID, c.Query("status"), c.Query("keyword"), pageOf(c))
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"list": list, "total": total})
}

type keywordReq struct {
	Keyword      string `json:"keyword"`
	Locale       string `json:"locale"`
	TargetURL    string `json:"target_url"`
	Rank         *int   `json:"rank"`
	PrevRank     *int   `json:"prev_rank"`
	SearchVolume *int   `json:"search_volume"`
	Status       string `json:"status"`
	Note         string `json:"note"`
}

func (s *Server) CreateKeyword(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req keywordReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Keyword) == "" {
		writeFail(c, http.StatusBadRequest, "keyword required")
		return
	}
	k := &model.Keyword{
		TenantID:  u.TenantID,
		Keyword:   strings.TrimSpace(req.Keyword),
		Locale:    orDefault(strings.TrimSpace(req.Locale), "zh-CN"),
		TargetURL: strings.TrimSpace(req.TargetURL),
		Status:    orDefault(strings.TrimSpace(req.Status), "tracking"),
		Note:      strings.TrimSpace(req.Note),
	}
	if req.Rank != nil {
		k.Rank = *req.Rank
	}
	if req.PrevRank != nil {
		k.PrevRank = *req.PrevRank
	}
	if req.SearchVolume != nil {
		k.SearchVolume = *req.SearchVolume
	}
	if err := s.Store.CreateKeyword(c.Request.Context(), k); err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, k)
}

func (s *Server) UpdateKeyword(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	var req keywordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeFail(c, http.StatusBadRequest, "invalid body")
		return
	}
	updates := map[string]any{}
	if n := strings.TrimSpace(req.Keyword); n != "" {
		updates["keyword"] = n
	}
	if req.Locale != "" {
		updates["locale"] = req.Locale
	}
	updates["target_url"] = strings.TrimSpace(req.TargetURL)
	if req.Rank != nil {
		updates["rank"] = *req.Rank
		now := time.Now()
		updates["checked_at"] = now
	}
	if req.PrevRank != nil {
		updates["prev_rank"] = *req.PrevRank
	}
	if req.SearchVolume != nil {
		updates["search_volume"] = *req.SearchVolume
	}
	if st := strings.TrimSpace(req.Status); st != "" {
		updates["status"] = st
	}
	updates["note"] = strings.TrimSpace(req.Note)
	if err := s.Store.UpdateKeyword(c.Request.Context(), u.TenantID, id, updates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeFail(c, http.StatusNotFound, "not found")
			return
		}
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"updated": true})
}

func (s *Server) DeleteKeyword(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	if err := s.Store.DeleteKeyword(c.Request.Context(), u.TenantID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeFail(c, http.StatusNotFound, "not found")
			return
		}
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"deleted": true})
}

// ---------- Geo prompts ----------

func (s *Server) ListGeoPrompts(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListGeoPrompts(u.TenantID, pageOf(c))
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"list": list, "total": total})
}

type geoPromptReq struct {
	Category string `json:"category"`
	Question string `json:"question"`
	Engine   string `json:"engine"`
	Status   *int16 `json:"status"`
}

func (s *Server) CreateGeoPrompt(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req geoPromptReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Question) == "" {
		writeFail(c, http.StatusBadRequest, "question required")
		return
	}
	p := &model.GeoPrompt{
		TenantID: u.TenantID,
		Category: orDefault(strings.TrimSpace(req.Category), "category"),
		Question: strings.TrimSpace(req.Question),
		Engine:   orDefault(strings.TrimSpace(req.Engine), "llm"),
		Status:   1,
	}
	if err := s.Store.CreateGeoPrompt(c.Request.Context(), p); err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, p)
}

type generatePromptsReq struct {
	SiteURL string `json:"site_url"` // 可选：填了就抓站生成，并顺带识别品牌
	Hint    string `json:"hint"`
	Count   int    `json:"count"`
}

// GenerateGeoPrompts 用 AI 生成候选检测问题，同步返回，不落库。
// 两种模式：默认按租户品牌/竞品生成；带 site_url 时抓取该网址内容生成，并顺带识别站点品牌。
func (s *Server) GenerateGeoPrompts(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if s.Runner.ResolveAI().APIKey == "" {
		writeFail(c, http.StatusBadRequest, "未配置 AI 模型密钥：请在控制台 AI 设置或服务器环境变量中配置")
		return
	}
	var req generatePromptsReq
	if !bindOptionalJSON(c, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 80*time.Second)
	defer cancel()
	if site := strings.TrimSpace(req.SiteURL); site != "" {
		items, brand, fetched, err := s.Runner.GeneratePromptsFromSite(ctx, u.TenantID, site, req.Hint, req.Count)
		if err != nil {
			writeFail(c, http.StatusBadGateway, "AI 生成失败："+err.Error())
			return
		}
		writeOK(c, gin.H{"items": items, "suggested_brand": brand, "site_fetched": fetched})
		return
	}
	items, err := s.Runner.GeneratePrompts(ctx, u.TenantID, req.Hint, req.Count)
	if err != nil {
		writeFail(c, http.StatusBadGateway, "AI 生成失败："+err.Error())
		return
	}
	writeOK(c, gin.H{"items": items})
}

type batchCreatePromptsReq struct {
	Items []geoPromptReq `json:"items"`
}

// BatchCreateGeoPrompts 批量落库用户勾选的候选问题；与题库已有问题（忽略大小写与首尾空白）
// 重复的自动跳过，返回 created / skipped 两个计数。
func (s *Server) BatchCreateGeoPrompts(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req batchCreatePromptsReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		writeFail(c, http.StatusBadRequest, "items required")
		return
	}
	if len(req.Items) > 100 {
		writeFail(c, http.StatusBadRequest, "一次最多提交 100 条")
		return
	}
	items := make([]model.GeoPrompt, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, model.GeoPrompt{Question: it.Question, Category: it.Category, Engine: it.Engine})
	}
	created, skipped, err := s.Store.BatchCreateGeoPrompts(c.Request.Context(), u.TenantID, items)
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"created": created, "skipped": skipped})
}

func (s *Server) UpdateGeoPrompt(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	var req geoPromptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeFail(c, http.StatusBadRequest, "invalid body")
		return
	}
	updates := map[string]any{}
	if q := strings.TrimSpace(req.Question); q != "" {
		updates["question"] = q
	}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	if req.Engine != "" {
		updates["engine"] = req.Engine
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if err := s.Store.UpdateGeoPrompt(c.Request.Context(), u.TenantID, id, updates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeFail(c, http.StatusNotFound, "not found")
			return
		}
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"updated": true})
}

func (s *Server) DeleteGeoPrompt(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	if err := s.Store.DeleteGeoPrompt(c.Request.Context(), u.TenantID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeFail(c, http.StatusNotFound, "not found")
			return
		}
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, gin.H{"deleted": true})
}

// ---------- Geo runs ----------

type createRunReq struct {
	Mode   string `json:"mode"`
	Engine string `json:"engine"` // chatgpt | selfhost | stub
	Note   string `json:"note"`
}

func (s *Server) CreateGeoRun(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req createRunReq
	if !bindOptionalJSON(c, &req) {
		return
	}
	mode := strings.TrimSpace(req.Mode)
	engine := strings.TrimSpace(req.Engine)
	// 默认走真实检测：不传 mode 曾经默认跑 stub，等于把"点一下开始检测"
	// 兑换成一份模板拼的假报告。缺 AI 密钥在下面明确报错，不再兜底假数据。
	if mode == "" {
		mode = "llm"
	}
	if mode != "stub" && mode != "llm" {
		writeFail(c, http.StatusBadRequest, "mode must be stub|llm")
		return
	}
	// 引擎归一：空值 = 自建接地（向后兼容），stub 与 demo 门控在下面。
	if engine == "" {
		engine = "selfhost"
	}
	if engine != "chatgpt" && engine != "selfhost" && engine != "perplexity" && engine != "gemini" && engine != "stub" {
		writeFail(c, http.StatusBadRequest, "engine must be chatgpt|selfhost|perplexity|gemini|stub")
		return
	}
	if mode == "stub" && !s.DemoMode {
		writeFail(c, http.StatusBadRequest, "演示模式已关闭：本环境不产出模拟检测数据，请改用真实 AI 检测（mode=llm）")
		return
	}
	if mode == "llm" && !s.Runner.EngineConfigured(engine) {
		msg := "未配置 AI 模型密钥：请在控制台 AI 设置或服务器环境变量中配置"
		if engine == "perplexity" {
			msg = "未配置 Perplexity API 密钥（服务器环境变量 PERPLEXITY_API_KEY）"
		} else if engine == "gemini" {
			msg = "未配置 Gemini API 密钥（服务器环境变量 GEMINI_API_KEY）"
		}
		writeFail(c, http.StatusBadRequest, msg)
		return
	}

	run, reused, err := s.Runner.SubmitGeoRun(u.TenantID, u.UserID, mode, engine, req.Note)
	if err != nil {
		writeFail(c, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(c, gin.H{"run": run, "reused": reused})
}

func (s *Server) ListGeoRuns(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListGeoRuns(u.TenantID, pageOf(c))
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// demo_mode 供检测页决定是否渲染「模拟演示」按钮：能力由后端说了算，
	// 前端不引环境变量（生产下按钮不渲染，即便硬调也会被上面的 mode 校验拒掉）。
	writeOK(c, gin.H{"list": list, "total": total, "demo_mode": s.DemoMode})
}

func (s *Server) GetGeoRun(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	run, err := s.Store.GetGeoRun(u.TenantID, id)
	if err != nil {
		writeLookupFailure(c, err, "not found")
		return
	}
	results, err := s.Store.ListGeoResults(u.TenantID, id)
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	var items []model.AdviceItem
	if strings.TrimSpace(run.AdviceItemsJSON) != "" {
		_ = json.Unmarshal([]byte(run.AdviceItemsJSON), &items)
	}
	writeOK(c, gin.H{"run": run, "results": results, "advice_items": items})
}

// SeedDemo creates sample brand + keywords + prompts for empty tenants.
// 路由只在演示模式下挂载（见 RegisterRoutes），这里再挡一道：往租户库写演示
// 数据的端点，绝不能因为将来有人无条件挂了路由就在生产上活过来。
func (s *Server) SeedDemo(c *gin.Context) {
	if !s.DemoMode {
		writeFail(c, http.StatusNotFound, "演示数据播种未启用")
		return
	}
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	// 只在三类数据都为空时播种，且不写任何假排名——排名一律由真实采集产生
	brands, _ := s.Store.ListAllBrands(u.TenantID)
	kws, _, _ := s.Store.ListKeywords(u.TenantID, "", "", pageOf(c))
	prompts, _, _ := s.Store.ListGeoPrompts(u.TenantID, pageOf(c))
	if len(brands) > 0 || len(kws) > 0 || len(prompts) > 0 {
		writeFail(c, http.StatusConflict, "已有真实数据，不再写入演示数据（避免混入）")
		return
	}
	_ = s.Store.CreateBrand(c.Request.Context(), &model.Brand{TenantID: u.TenantID, Name: "GopherForge", Kind: "brand", Aliases: "gopherforge,go-admin-kit", Website: "https://github.com/SuperiorChuo/gopherforge", Status: 1})
	_ = s.Store.CreateBrand(c.Request.Context(), &model.Brand{TenantID: u.TenantID, Name: "竞品示例A", Kind: "competitor", Status: 1})
	_ = s.Store.CreateBrand(c.Request.Context(), &model.Brand{TenantID: u.TenantID, Name: "竞品示例B", Kind: "competitor", Status: 1})
	_ = s.Store.CreateKeyword(c.Request.Context(), &model.Keyword{TenantID: u.TenantID, Keyword: "开源后台管理系统", SearchVolume: 800, Status: "tracking", Locale: "zh-CN"})
	_ = s.Store.CreateKeyword(c.Request.Context(), &model.Keyword{TenantID: u.TenantID, Keyword: "React Ant Design 管理后台", SearchVolume: 500, Status: "tracking", Locale: "zh-CN"})
	_ = s.Store.CreateKeyword(c.Request.Context(), &model.Keyword{TenantID: u.TenantID, Keyword: "微服务中台脚手架", SearchVolume: 200, Status: "tracking", Locale: "zh-CN"})
	_ = s.Store.CreateGeoPrompt(c.Request.Context(), &model.GeoPrompt{TenantID: u.TenantID, Category: "category", Question: "有没有适合中小团队的开源后台管理系统推荐？", Engine: "llm", Status: 1})
	_ = s.Store.CreateGeoPrompt(c.Request.Context(), &model.GeoPrompt{TenantID: u.TenantID, Category: "comparison", Question: "GopherForge 和常见竞品比起来怎么样？", Engine: "llm", Status: 1})
	_ = s.Store.CreateGeoPrompt(c.Request.Context(), &model.GeoPrompt{TenantID: u.TenantID, Category: "scenario", Question: "想做带 IM 客服和 CRM 的中台，用什么脚手架比较合适？", Engine: "llm", Status: 1})
	writeOK(c, gin.H{"seeded": true})
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func parseInt(s string) (int, error) {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("nan")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
