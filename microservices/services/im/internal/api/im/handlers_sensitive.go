package api

// 敏感词管理（借鉴 ruoyi-vue-pro 敏感词模块）：DB 词库 + 控制台 CRUD + 热生效。
// 命中策略：mask 打码为 *，block 直接拒发。env IM_SENSITIVE_WORDS 兜底词
// 继续由 maskSensitive 叠加打码（不支持 block，保持旧行为）。

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/store/im"
)

// applySensitive 是 REST/WS 文本消息的唯一过滤入口：
// DB 词库（分租户，mask/block）先行，env 兜底词打码殿后。
// blocked=true 时返回原文不落库，调用方拒绝该消息。
func (s *Server) applySensitive(tenantID uint64, text string) (masked string, blocked bool) {
	if s.Sensitive != nil {
		out, hit, _ := s.Sensitive.Apply(tenantID, text)
		if hit {
			return text, true
		}
		text = out
	}
	return s.maskSensitive(text), false
}

const sensitiveBlockedMsg = "消息包含敏感词，已被拦截"

// GET /api/v1/im/admin/sensitive-words（敏感词列表）
func (s *Server) AdminListSensitiveWords(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	f := store.SensitiveWordFilter{
		Keyword: strings.TrimSpace(c.Query("keyword")),
		Action:  c.Query("action"),
		Tag:     strings.TrimSpace(c.Query("tag")),
	}
	if v := c.Query("status"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 16); err == nil {
			st := int16(n)
			f.Status = &st
		}
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}
	f.Offset = (page - 1) * pageSize
	f.Limit = pageSize
	list, total, err := s.Store.ListSensitiveWords(agent.TenantID, f)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

type createSensitiveWordsReq struct {
	// Words 支持批量导入（前端多行文本拆词）；Word 单个添加两者取并。
	Words  []string `json:"words"`
	Word   string   `json:"word"`
	Action string   `json:"action"`
	Tag    string   `json:"tag"`
}

// POST /api/v1/im/admin/sensitive-words（新增敏感词）
func (s *Server) AdminCreateSensitiveWords(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	var req createSensitiveWordsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	words := req.Words
	if strings.TrimSpace(req.Word) != "" {
		words = append(words, req.Word)
	}
	created, skipped, err := s.Store.CreateSensitiveWords(c.Request.Context(), agent.TenantID, agent.UserID, words, req.Action, req.Tag)
	if err != nil {
		// 逐行写入可能部分成功（前面行已提交）——即使报错也要刷缓存，
		// 否则新词 30s 缓存 TTL 内不生效。
		if created > 0 {
			s.Sensitive.Invalidate(agent.TenantID)
		}
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.Sensitive.Invalidate(agent.TenantID)
	OK(c, gin.H{"created": created, "skipped": skipped})
}

type updateSensitiveWordReq struct {
	Word   *string `json:"word"`
	Action *string `json:"action"`
	Tag    *string `json:"tag"`
	Status *int16  `json:"status"`
}

// PUT /api/v1/im/admin/sensitive-words/:id（更新敏感词）
func (s *Server) AdminUpdateSensitiveWord(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req updateSensitiveWordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	row, err := s.Store.UpdateSensitiveWord(c.Request.Context(), agent.TenantID, id, store.SensitiveWordUpdate{
		Word: req.Word, Action: req.Action, Tag: req.Tag, Status: req.Status,
	})
	if err != nil {
		if errorsIsNotFound(err) {
			Fail(c, http.StatusNotFound, "敏感词不存在")
			return
		}
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.Sensitive.Invalidate(agent.TenantID)
	OK(c, row)
}

// DELETE /api/v1/im/admin/sensitive-words/:id（删除敏感词）
func (s *Server) AdminDeleteSensitiveWord(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.Store.DeleteSensitiveWord(c.Request.Context(), agent.TenantID, id); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.Sensitive.Invalidate(agent.TenantID)
	OK(c, gin.H{"deleted": true})
}

type testSensitiveReq struct {
	Text string `json:"text"`
}

// POST /api/v1/im/admin/sensitive-words/test — 管理端试敏：返回命中词与打码结果。
func (s *Server) AdminTestSensitiveText(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	var req testSensitiveReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		Fail(c, http.StatusBadRequest, "text 不能为空")
		return
	}
	maskedDB, blocked, hits := s.Sensitive.Apply(agent.TenantID, req.Text)
	masked := maskedDB
	if !blocked {
		masked = s.maskSensitive(maskedDB)
	}
	OK(c, gin.H{"masked_text": masked, "blocked": blocked, "hits": hits})
}
