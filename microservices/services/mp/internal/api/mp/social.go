package api

// 社媒内容中台 API（阶段B）：Post CRUD + 多渠道发布/导出。发布经 Publisher registry
// 分发到渠道 adapter，写 PostPublication 记录。渠道账号列表供前端选择发布目标。
//
// 与存量微信 /api/v1/mp/articles 并存：新内容走 /api/v1/social/posts，旧 MpArticle
// 只读兼容。

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/content"
	"github.com/go-admin-kit/services/mp/internal/publisher"
)

func (s *Server) contentReady(c *gin.Context) bool {
	if s.Content == nil {
		fail(c, http.StatusServiceUnavailable, "内容中台未就绪")
		return false
	}
	return true
}

// ---- 渠道能力 ----

// ListChannels GET /api/v1/social/channels — 已注册渠道及能力（前端渲染发布选项）。
func (s *Server) ListChannels(c *gin.Context) {
	if _, authed := s.requireUser(c); !authed {
		return
	}
	caps := map[string]publisher.Capabilities{}
	if s.Publishers != nil {
		caps = s.Publishers.List()
	}
	ok(c, gin.H{"channels": caps})
}

// ---- Post CRUD ----

type postReq struct {
	Title    string                `json:"title"`
	BodyHTML string                `json:"body_html"`
	Summary  string                `json:"summary"`
	CoverURL string                `json:"cover_url"`
	Platform string                `json:"platform"`
	Media    content.MediaItemList `json:"media"`
	Tags     content.StringList    `json:"tags"`
	Status   string                `json:"status"`
}

func (s *Server) ListPosts(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) {
		return
	}
	page, pageSize := pageOf(c)
	list, total, err := s.Content.ListPosts(u.TenantID, content.PostFilter{
		Platform: c.Query("platform"), Status: c.Query("status"),
		Keyword: c.Query("keyword"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (s *Server) GetPost(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) {
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	p, err := s.Content.GetPost(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "内容不存在")
		return
	}
	pubs, _ := s.Content.ListPublications(u.TenantID, id)
	ok(c, gin.H{"post": p, "publications": pubs})
}

func (s *Server) CreatePost(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) {
		return
	}
	var req postReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	p := &content.Post{
		TenantID: u.TenantID, Title: req.Title, BodyHTML: req.BodyHTML, Summary: req.Summary,
		CoverURL: req.CoverURL, Platform: req.Platform, Media: req.Media, Tags: req.Tags,
		Status: req.Status, CreatedBy: u.UserID,
	}
	if err := s.Content.CreatePost(p); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, p)
}

func (s *Server) UpdatePost(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) {
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	p, err := s.Content.GetPost(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "内容不存在")
		return
	}
	var req postReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	p.Title, p.BodyHTML, p.Summary = req.Title, req.BodyHTML, req.Summary
	p.CoverURL, p.Platform, p.Media, p.Tags = req.CoverURL, req.Platform, req.Media, req.Tags
	if req.Status != "" {
		p.Status = req.Status
	}
	if err := s.Content.UpdatePost(p); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, p)
}

func (s *Server) DeletePost(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) {
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	if err := s.Content.DeletePost(id, u.TenantID); err != nil {
		notFoundOr(c, err, "内容不存在")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ---- AI 生成 ----

type genPostReq struct {
	Prompt   string `json:"prompt"`
	Platform string `json:"platform"` // wechat/xiaohongshu/zhihu/weibo/douyin，空=通用
}

// GeneratePost POST /api/v1/social/posts/generate — 按平台风格 AI 生成一条内容并入库。
func (s *Server) GeneratePost(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) {
		return
	}
	if s.AI == nil {
		fail(c, http.StatusServiceUnavailable, "AI 客户端未配置")
		return
	}
	var req genPostReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Prompt == "" {
		fail(c, http.StatusBadRequest, "prompt 必填")
		return
	}
	raw, err := s.AI.ComposeForPlatform(c.Request.Context(), req.Prompt, req.Platform, s.aiAuth(c))
	if err != nil {
		fail(c, http.StatusBadGateway, "AI 生成失败："+err.Error())
		return
	}
	title, digest, body := parseArticleJSON(raw)
	p := &content.Post{
		TenantID: u.TenantID, Title: title, Summary: digest, BodyHTML: body,
		Platform: req.Platform, AIPrompt: req.Prompt, Status: "draft", CreatedBy: u.UserID,
	}
	if err := s.Content.CreatePost(p); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, p)
}

// ---- 渠道账号（供发布选择目标）----

// ListChannelAccounts GET /api/v1/social/channel-accounts?channel= — 当前租户可发布账号。
func (s *Server) ListChannelAccounts(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) {
		return
	}
	list, err := s.Content.ListChannelAccounts(u.TenantID, c.Query("channel"))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list})
}

// ---- 发布 / 导出 ----

type publishReq struct {
	Channel   string `json:"channel"`    // wechat/weibo/...
	AccountID uint64 `json:"account_id"` // content.ChannelAccount.id
}

// PublishPost POST /api/v1/social/posts/:id/publish — 发布到某渠道账号。有 API 直发，
// 无 API 引导用导出。写 PostPublication 记录。
func (s *Server) PublishPost(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) || s.Publishers == nil {
		fail(c, http.StatusServiceUnavailable, "发布器未就绪")
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	post, err := s.Content.GetPost(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "内容不存在")
		return
	}
	var req publishReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Channel == "" {
		fail(c, http.StatusBadRequest, "channel 必填")
		return
	}
	pub, err := s.Publishers.Get(req.Channel)
	if err != nil {
		fail(c, http.StatusBadRequest, "未知渠道："+req.Channel)
		return
	}
	acc, err := s.Content.GetChannelAccount(req.AccountID, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "渠道账号不存在")
		return
	}

	rec := &content.PostPublication{
		TenantID: u.TenantID, PostID: post.ID, Channel: req.Channel,
		AccountID: req.AccountID, Mode: "api", Status: "publishing",
	}
	res, perr := pub.Publish(c.Request.Context(), acc, post)
	if errors.Is(perr, publisher.ErrPublishUnsupported) {
		// 无 API：转导出模式，前端拿 ExportPackage。
		rec.Mode, rec.Status = "export", "exporting"
		_ = s.Content.CreatePublication(rec)
		fail(c, http.StatusConflict, "该渠道无 API 直发，请用导出接口")
		return
	}
	if perr != nil {
		rec.Status, rec.Error = "failed", perr.Error()
		_ = s.Content.CreatePublication(rec)
		fail(c, http.StatusBadGateway, perr.Error())
		return
	}
	rec.ExternalID = res.ExternalID
	rec.ExternalURL = res.ExternalURL
	if res.Async {
		rec.Status = "publishing" // 异步，等回调/查询
	} else {
		rec.Status = "published"
	}
	if err := s.Content.CreatePublication(rec); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, rec)
}

// ExportPost POST /api/v1/social/posts/:id/export — 半自动导出（无 API 渠道用）。
func (s *Server) ExportPost(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if !s.contentReady(c) || s.Publishers == nil {
		fail(c, http.StatusServiceUnavailable, "发布器未就绪")
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	post, err := s.Content.GetPost(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "内容不存在")
		return
	}
	var req publishReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Channel == "" {
		fail(c, http.StatusBadRequest, "channel 必填")
		return
	}
	pub, err := s.Publishers.Get(req.Channel)
	if err != nil {
		fail(c, http.StatusBadRequest, "未知渠道："+req.Channel)
		return
	}
	var acc *content.ChannelAccount
	if req.AccountID > 0 {
		acc, _ = s.Content.GetChannelAccount(req.AccountID, u.TenantID)
	}
	pkg, perr := pub.ExportPackage(c.Request.Context(), acc, post)
	if errors.Is(perr, publisher.ErrExportUnsupported) {
		fail(c, http.StatusConflict, "该渠道不支持导出")
		return
	}
	if perr != nil {
		fail(c, http.StatusBadGateway, perr.Error())
		return
	}
	// 留一条 export 记录（可选）
	_ = s.Content.CreatePublication(&content.PostPublication{
		TenantID: u.TenantID, PostID: post.ID, Channel: req.Channel,
		AccountID: req.AccountID, Mode: "export", Status: "exported",
	})
	ok(c, pkg)
}
