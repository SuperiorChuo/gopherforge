package api

// AI 生成类接口：品牌/竞品识别、SEO 关键词生成（均为"生成候选→前端勾选→批量入库"
// 两段式，与 GEO 问题生成同一交互模式），以及检测完成后的优化建议生成。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/store/visibility"
)

// requireAI 统一拦截未配置模型密钥的情况。
func (s *Server) requireAI(c *gin.Context) bool {
	if s.Runner.ResolveAI().APIKey == "" {
		writeFail(c, http.StatusBadRequest, "未配置 AI 模型密钥：请在控制台 AI 设置或服务器环境变量中配置")
		return false
	}
	return true
}

type generateBrandsReq struct {
	SiteURL string `json:"site_url"` // 可选：填了就抓站识别；不填则按品牌库推荐竞品
	Hint    string `json:"hint"`
}

// GenerateBrands 用 AI 识别品牌/竞品候选，同步返回，不落库。
func (s *Server) GenerateBrands(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.requireAI(c) {
		return
	}
	var req generateBrandsReq
	_ = c.ShouldBindJSON(&req)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 80*time.Second)
	defer cancel()
	items, fetched, err := s.Runner.GenerateBrands(ctx, u.TenantID, req.SiteURL, req.Hint)
	if err != nil {
		writeFail(c, http.StatusBadGateway, "AI 识别失败："+err.Error())
		return
	}
	writeOK(c, gin.H{"items": items, "site_fetched": fetched})
}

type batchCreateBrandsReq struct {
	Items []brandReq `json:"items"`
}

// BatchCreateBrands 批量落库用户勾选的品牌候选；与品牌库已有名称/别名（忽略大小写）
// 重复的自动跳过，返回 created / skipped 计数。
func (s *Server) BatchCreateBrands(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req batchCreateBrandsReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		writeFail(c, http.StatusBadRequest, "items required")
		return
	}
	if len(req.Items) > 100 {
		writeFail(c, http.StatusBadRequest, "一次最多提交 100 条")
		return
	}
	seen, err := s.Store.ListBrandNames(u.TenantID)
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created, skipped := 0, 0
	for _, it := range req.Items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		if seen[strings.ToLower(name)] {
			skipped++
			continue
		}
		kind := strings.TrimSpace(it.Kind)
		if kind != "brand" {
			kind = "competitor"
		}
		b := &model.Brand{
			TenantID: u.TenantID,
			Name:     name,
			Aliases:  strings.TrimSpace(it.Aliases),
			Kind:     kind,
			Website:  strings.TrimSpace(it.Website),
			Note:     strings.TrimSpace(it.Note),
			Status:   1,
		}
		if err := s.Store.CreateBrand(c.Request.Context(), b); err != nil {
			writeFail(c, http.StatusInternalServerError, err.Error())
			return
		}
		seen[strings.ToLower(name)] = true
		created++
	}
	writeOK(c, gin.H{"created": created, "skipped": skipped})
}

type generateKeywordsReq struct {
	SiteURL string `json:"site_url"` // 可选：填了就抓站生成；不填则按品牌库生成
	Hint    string `json:"hint"`
	Count   int    `json:"count"`
}

// GenerateKeywords 用 AI 生成 SEO 关键词候选，同步返回，不落库。
// target_url 统一预填为 site_url（用户可在前端逐条改）。
func (s *Server) GenerateKeywords(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.requireAI(c) {
		return
	}
	var req generateKeywordsReq
	_ = c.ShouldBindJSON(&req)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 80*time.Second)
	defer cancel()
	items, fetched, err := s.Runner.GenerateKeywords(ctx, u.TenantID, req.SiteURL, req.Hint, req.Count)
	if err != nil {
		writeFail(c, http.StatusBadGateway, "AI 生成失败："+err.Error())
		return
	}
	target := strings.TrimSpace(req.SiteURL)
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, gin.H{"keyword": it.Keyword, "locale": it.Locale, "target_url": target})
	}
	writeOK(c, gin.H{"items": out, "site_fetched": fetched})
}

type batchCreateKeywordsReq struct {
	Items []keywordReq `json:"items"`
}

// BatchCreateKeywords 批量落库用户勾选的关键词候选；与已有关键词（忽略大小写与首尾空白）
// 重复的自动跳过，返回 created / skipped 计数。
func (s *Server) BatchCreateKeywords(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req batchCreateKeywordsReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		writeFail(c, http.StatusBadRequest, "items required")
		return
	}
	if len(req.Items) > 100 {
		writeFail(c, http.StatusBadRequest, "一次最多提交 100 条")
		return
	}
	existing, err := s.Store.ListKeywordTexts(u.TenantID)
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	seen := make(map[string]bool, len(existing))
	for _, k := range existing {
		seen[strings.ToLower(strings.TrimSpace(k))] = true
	}
	created, skipped := 0, 0
	for _, it := range req.Items {
		kw := strings.TrimSpace(it.Keyword)
		if kw == "" {
			continue
		}
		key := strings.ToLower(kw)
		if seen[key] {
			skipped++
			continue
		}
		k := &model.Keyword{
			TenantID:  u.TenantID,
			Keyword:   kw,
			Locale:    orDefault(strings.TrimSpace(it.Locale), "zh-CN"),
			TargetURL: strings.TrimSpace(it.TargetURL),
			Status:    "tracking",
		}
		if err := s.Store.CreateKeyword(c.Request.Context(), k); err != nil {
			writeFail(c, http.StatusInternalServerError, err.Error())
			return
		}
		seen[key] = true
		created++
	}
	writeOK(c, gin.H{"created": created, "skipped": skipped})
}

// GeoLeaderboard 返回「AI 推荐排行榜」：最近几次检测里各品牌被提到的次数与自有品牌排名。
func (s *Server) GeoLeaderboard(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	stats, err := s.Store.GeoLeaderboard(u.TenantID, 3, s.DemoMode)
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, stats)
}

// leaderboardText 把排行榜压成一段文字，喂给命名分析当事实依据。
func leaderboardText(st *store.GeoLeaderboardStats) string {
	if st == nil || st.RunsUsed == 0 {
		return ""
	}
	var parts []string
	for i, it := range st.Items {
		if i >= 10 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d次(第%d)", it.Name, it.Mentions, i+1))
	}
	text := fmt.Sprintf("最近 %d 次检测共 %d 题，提及榜：%s。自有品牌「%s」提及率 %d%%",
		st.RunsUsed, st.TotalQuestions, strings.Join(parts, "、"), st.OwnName, st.MentionRate)
	if st.AvgPosition > 0 {
		text += fmt.Sprintf("，推荐列表平均第 %.1f 位、最好第 %d 位", st.AvgPosition, st.BestPosition)
	}
	if st.Mode == "stub" {
		text += "（注意：这是模拟演示数据，仅供参考）"
	}
	return text
}

// BrandNameAnalysis 品牌命名体检：相似撞名检测 + 改名候选 + 保留/改名结论。
func (s *Server) BrandNameAnalysis(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.requireAI(c) {
		return
	}
	stats, err := s.Store.GeoLeaderboard(u.TenantID, 3, s.DemoMode)
	if err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 150*time.Second)
	defer cancel()
	res, err := s.Runner.GenerateNamingAnalysis(ctx, u.TenantID, leaderboardText(stats))
	if err != nil {
		writeFail(c, http.StatusBadGateway, "命名分析失败："+err.Error())
		return
	}
	writeOK(c, res)
}

// GenerateGeoAdvice 对一次已完成的检测生成 AI 优化建议 + 可执行清单并保存。
// 已有建议时重复调用会重新生成并覆盖（前端"重新生成"按钮）。
func (s *Server) GenerateGeoAdvice(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.requireAI(c) {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	// 建议是长文输出，内网中转模型出全文可能超过 1 分钟，放宽到 3 分钟
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	advice, items, err := s.Runner.GenerateGeoAdvice(ctx, u.TenantID, id)
	if err != nil {
		writeFail(c, http.StatusBadGateway, "生成建议失败："+err.Error())
		return
	}
	writeOK(c, gin.H{"advice": advice, "advice_items": items})
}

// PatchGeoAdviceItems 更新可执行清单勾选状态（不重新调 AI）。
func (s *Server) PatchGeoAdviceItems(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	var req struct {
		Items []model.AdviceItem `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		writeFail(c, http.StatusBadRequest, "items required")
		return
	}
	if len(req.Items) > 100 {
		writeFail(c, http.StatusBadRequest, "一次最多提交 100 条")
		return
	}
	run, err := s.Store.GetGeoRun(u.TenantID, id)
	if err != nil {
		writeFail(c, http.StatusNotFound, "检测记录不存在")
		return
	}
	// 合并 done 状态：以 body 为准覆盖全量
	raw, _ := json.Marshal(req.Items)
	if err := s.Store.UpdateGeoRun(u.TenantID, id, map[string]any{"advice_items_json": string(raw)}); err != nil {
		writeFail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = run
	writeOK(c, gin.H{"advice_items": req.Items})
}

// CompareGeoRun 与上一次完成的检测对比提及率与逐题得失。
func (s *Server) CompareGeoRun(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	cur, err := s.Store.GetGeoRun(u.TenantID, id)
	if err != nil {
		writeFail(c, http.StatusNotFound, "检测记录不存在")
		return
	}
	if cur.Status != "done" {
		writeFail(c, http.StatusBadRequest, "仅已完成的检测可对比")
		return
	}
	var prev *model.GeoRun
	if withID, _ := strconv.ParseUint(c.Query("with"), 10, 64); withID > 0 {
		prev, err = s.Store.GetGeoRun(u.TenantID, withID)
		if err != nil {
			writeFail(c, http.StatusNotFound, "对比目标不存在")
			return
		}
		// 半程 run 的计数不可比；自比恒等无意义
		if prev.Status != "done" || prev.ID == id {
			writeFail(c, http.StatusBadRequest, "对比目标须是另一次已完成的检测")
			return
		}
	} else {
		prev, err = s.Store.FindPreviousDoneGeoRun(u.TenantID, id)
		if err != nil {
			writeFail(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if prev == nil {
		writeOK(c, gin.H{"has_previous": false, "compare": nil})
		return
	}
	curRes, _ := s.Store.ListGeoResults(u.TenantID, cur.ID)
	prevRes, _ := s.Store.ListGeoResults(u.TenantID, prev.ID)
	cmp := store.BuildGeoRunCompare(cur, curRes, prev, prevRes)
	writeOK(c, gin.H{
		"has_previous": true,
		"compare":      cmp,
		"previous":     prev,
	})
}
