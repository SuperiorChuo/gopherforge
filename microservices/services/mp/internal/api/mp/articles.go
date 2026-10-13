package api

// M5 AI 图文：生成 → 生成封面 → 编辑 → 发布（草稿箱 / 自动 freepublish）。
// AI 生成走 aiclient（调 ai-service）；发布走 wxclient（草稿 + freepublish）。
// 封面图上传微信永久素材拿 thumb_media_id；正文图片先经 uploadimg 换成微信 URL，
// 全部成功后再创建草稿。

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/aiclient"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxarticle"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
)

// ---- 列表 / CRUD ----

func (s *Server) ListArticles(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	page, pageSize := pageOf(c)
	list, total, err := s.Store.ListArticles(u.TenantID, queryUint64(c, "account_id"), c.Query("status"), page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (s *Server) GetArticle(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	a, err := s.Store.GetArticle(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "文章不存在")
		return
	}
	ok(c, a)
}

type articleEditReq struct {
	Title       string `json:"title"`
	Author      string `json:"author"`
	Digest      string `json:"digest"`
	ContentHTML string `json:"content_html"`
	CoverURL    string `json:"cover_url"`
}

func (s *Server) UpdateArticle(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	a, err := s.Store.GetArticle(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "文章不存在")
		return
	}
	var req articleEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	a.Title, a.Author, a.Digest = req.Title, req.Author, req.Digest
	a.ContentHTML, a.CoverURL = req.ContentHTML, req.CoverURL
	if err := s.Store.UpdateArticle(a); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, a)
}

func (s *Server) DeleteArticle(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	if err := s.Store.DeleteArticle(id, u.TenantID); err != nil {
		notFoundOr(c, err, "文章不存在")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ---- AI 生成 ----

type generateReq struct {
	AccountID uint64 `json:"account_id"`
	Prompt    string `json:"prompt"`
}

// GenerateArticle POST /api/v1/mp/articles/generate — AI 生成图文草稿。
func (s *Server) GenerateArticle(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if s.AI == nil {
		fail(c, http.StatusServiceUnavailable, "AI 客户端未配置")
		return
	}
	var req generateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Prompt == "" {
		fail(c, http.StatusBadRequest, "prompt 必填")
		return
	}
	content, err := s.AI.ComposeArticle(c.Request.Context(), req.Prompt, s.aiAuth(c))
	if err != nil {
		fail(c, http.StatusBadGateway, "AI 生成失败："+err.Error())
		return
	}
	title, digest, html := parseArticleJSON(content)
	art := &store.MpArticle{
		TenantID: u.TenantID, AccountID: req.AccountID,
		Title: title, Digest: digest, ContentHTML: html,
		AIPrompt: req.Prompt, Status: "draft_ai", CreatedBy: u.UserID,
	}
	if err := s.Store.CreateArticle(art); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, art)
}

type coverReq struct {
	Prompt string `json:"prompt"`
	Size   string `json:"size"`
}

// GenerateCover POST /api/v1/mp/articles/:id/cover — AI 生成封面图，存 cover_url。
func (s *Server) GenerateCover(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if s.AI == nil {
		fail(c, http.StatusServiceUnavailable, "AI 客户端未配置")
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	art, err := s.Store.GetArticle(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "文章不存在")
		return
	}
	var req coverReq
	_ = c.ShouldBindJSON(&req)
	prompt := req.Prompt
	if prompt == "" {
		prompt = "微信公众号文章封面配图：" + art.Title
	}
	url, err := s.AI.GenerateImage(c.Request.Context(), prompt, req.Size, s.aiAuth(c))
	if err != nil {
		fail(c, http.StatusBadGateway, "AI 生图失败："+err.Error())
		return
	}
	art.CoverURL = url
	if err := s.Store.UpdateArticle(art); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"cover_url": url})
}

// ---- 发布 ----

// PublishArticle POST /api/v1/mp/articles/:id/publish — 封面进永久素材 → 草稿箱
// → 按账号 AutoPublish 决定是否 freepublish。
func (s *Server) PublishArticle(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	if s.WX == nil {
		fail(c, http.StatusServiceUnavailable, "微信客户端未配置")
		return
	}
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	art, err := s.Store.GetArticle(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "文章不存在")
		return
	}
	acc, err := s.Store.GetAccount(art.AccountID, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "账号不存在")
		return
	}
	if !requireActiveAccount(c, acc) {
		return
	}
	ctx := c.Request.Context()

	if art.CoverURL == "" {
		fail(c, http.StatusBadRequest, "请先生成或设置封面")
		return
	}

	// 先读取并校验所有源图片，再开始产生微信侧写入。
	imgBytes, ctype, err := wxarticle.FetchImage(ctx, art.CoverURL)
	if err != nil {
		fail(c, http.StatusBadRequest, "读取封面失败："+err.Error())
		return
	}
	rewrittenHTML, err := wxarticle.RewriteInlineImages(ctx, s.WX, acc, art.ContentHTML)
	if err != nil {
		if errors.Is(err, wxarticle.ErrUploadImage) {
			fail(c, http.StatusBadGateway, "正文图片上传微信失败："+err.Error())
		} else {
			fail(c, http.StatusBadRequest, "处理正文图片失败："+err.Error())
		}
		return
	}

	// 封面 → 微信永久缩略素材，拿 thumb_media_id。
	thumbID, _, err := s.WX.AddPermanentMaterial(ctx, acc, "thumb", "cover"+wxarticle.ExtensionForContentType(ctype), bytes.NewReader(imgBytes))
	if err != nil {
		fail(c, http.StatusBadGateway, "封面上传微信失败："+err.Error())
		return
	}
	art.ThumbMediaID = thumbID

	// 草稿箱只接收已经微信化的局部正文，数据库继续保留可编辑源稿。
	draftArticle := wxDraftArticle(art)
	draftArticle.Content = rewrittenHTML
	draftID, err := s.WX.AddDraft(ctx, acc, []wxclient.DraftArticle{draftArticle})
	if err != nil {
		fail(c, http.StatusBadGateway, "草稿箱失败："+err.Error())
		return
	}
	art.DraftMediaID = draftID
	art.Status = "drafted"

	// 自动发布？
	if acc.AutoPublish {
		pubID, perr := s.WX.SubmitFreepublish(ctx, acc, draftID)
		if perr != nil {
			// 草稿已建但发布提交失败：保留 drafted 状态，回报错误。
			_ = s.Store.UpdateArticle(art)
			fail(c, http.StatusBadGateway, "已存草稿，发布提交失败："+perr.Error())
			return
		}
		art.PublishID = pubID
		art.Status = "publishing"
	}
	if err := s.Store.UpdateArticle(art); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, art)
}

// ---- helpers ----

func (s *Server) aiAuth(c *gin.Context) aiclient.Auth {
	return aiclient.Auth{
		UserID:   c.GetHeader("X-Auth-User-ID"),
		Username: c.GetHeader("X-Auth-Username"),
		TenantID: c.GetHeader("X-Auth-Tenant-ID"),
		Bearer:   bearer(c),
	}
}

// wxDraftArticle 把 store.MpArticle 转成微信草稿元素。
func wxDraftArticle(a *store.MpArticle) wxclient.DraftArticle {
	return wxclient.DraftArticle{
		Title: a.Title, Author: a.Author, Digest: a.Digest,
		Content: a.ContentHTML, ThumbMediaID: a.ThumbMediaID,
	}
}

// parseArticleJSON 解析 wechat_article kind 返回的 JSON（宽容：非 JSON 时整体
// 当正文，标题留空由运营补）。
func parseArticleJSON(content string) (title, digest, html string) {
	trimmed := strings.TrimSpace(content)
	// 去掉可能的 ```json ... ``` 包裹
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	var obj struct {
		Title   string `json:"title"`
		Digest  string `json:"digest"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &obj); err == nil && (obj.Title != "" || obj.Content != "") {
		return obj.Title, obj.Digest, obj.Content
	}
	return "", "", content
}
