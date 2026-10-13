package ai

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// Hermes M2 技能市场的 console + device handlers。

func (h *HermesHandler) ListSkills(c *gin.Context) {
	page, pageSize := hermesPage(c)
	items, total, err := h.service.ListSkills(c.Request.Context(), page, pageSize)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, items, total, page, pageSize)
}

func (h *HermesHandler) CreateSkill(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	// multipart: name/display_name/description/version/signature + file
	request := service.CreateHermesSkillRequest{
		Name:        c.PostForm("name"),
		DisplayName: c.PostForm("display_name"),
		Description: c.PostForm("description"),
		Version:     c.PostForm("version"),
		Signature:   c.PostForm("signature"),
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "skill file is required")
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "failed to read skill file")
		return
	}
	if len(content) > 16<<20 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "skill file exceeds 16MB")
		return
	}
	skill, err := h.service.CreateSkill(c.Request.Context(), actorID, request, content)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 技能已录入，等待审批", skill)
}

func (h *HermesHandler) ApproveSkill(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.ApproveSkill(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 技能已批准", nil)
}

func (h *HermesHandler) RejectSkill(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.RejectSkill(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 技能已拒绝", nil)
}

func (h *HermesHandler) RevokeSkill(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.RevokeSkill(c.Request.Context(), actorID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Hermes 技能已吊销", nil)
}

func (h *HermesHandler) ListClusterSkills(c *gin.Context) {
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	items, err := h.service.ListClusterSkills(c.Request.Context(), id)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *HermesHandler) BindClusterSkill(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	clusterID, ok := parseHermesID(c)
	if !ok {
		return
	}
	var request struct {
		SkillID uint64 `json:"skill_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	if err := h.service.BindClusterSkill(c.Request.Context(), actorID, clusterID, request.SkillID); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "技能已绑定到集群", nil)
}

func (h *HermesHandler) UnbindClusterSkill(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	clusterID, ok := parseHermesID(c)
	if !ok {
		return
	}
	skillID, err := strconv.ParseUint(c.Param("skill_id"), 10, 64)
	if err != nil || skillID == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid skill id")
		return
	}
	if err := h.service.UnbindClusterSkill(c.Request.Context(), actorID, clusterID, skillID); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "技能已从集群解绑", nil)
}

// DownloadSkill 是设备签名端点：manager 按 SourceURL 拉取技能 tarball。
// 服务端只放行已审批技能；租户/吊销由设备鉴权中间件兜底。
func (h *HermesHandler) DownloadSkill(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		writeHermesError(c, service.ErrHermesInvalidInput)
		return
	}
	content, err := h.service.GetSkillContent(c.Request.Context(), id)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	if len(content) == 0 {
		writeHermesError(c, service.ErrHermesInvalidInput)
		return
	}
	c.Data(http.StatusOK, "application/octet-stream", content)
}

// ---------------------------------------------------------------------------
// 员工自助（M6）。身份永远来自会话（requireUserID），端点不接受 user_id 参数。
// ---------------------------------------------------------------------------

// ListMySkillCatalog 员工技能目录：仅已审批技能 + 本人 installed 标记。
func (h *HermesHandler) ListMySkillCatalog(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	page, pageSize := hermesPage(c)
	items, total, err := h.service.ListSkillCatalog(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.PageSuccess(c, items, total, page, pageSize)
}

// InstallMySkill 员工自助安装（仅已审批技能，幂等）。
func (h *HermesHandler) InstallMySkill(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.SelfBindSkill(c.Request.Context(), userID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "技能已安装，设备下次同步生效", nil)
}

// UninstallMySkill 员工自助卸载（幂等）。
func (h *HermesHandler) UninstallMySkill(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	id, ok := parseHermesID(c)
	if !ok {
		return
	}
	if err := h.service.SelfUnbindSkill(c.Request.Context(), userID, id); err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "技能已卸载，设备下次同步生效", nil)
}

// ---------------------------------------------------------------------------
// 官方 Skills Hub（GitHub 系源浏览 + 导入）
// ---------------------------------------------------------------------------

// BrowseHubSkills 浏览官方市场技能目录。
func (h *HermesHandler) BrowseHubSkills(c *gin.Context) {
	source := c.DefaultQuery("source", "official")
	browse, err := h.service.ListHubSkills(c.Request.Context(), source, c.Query("repo"), c.Query("path"))
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.Success(c, browse)
}

// ImportHubSkill 从官方市场导入技能（落 pending 待审批）。
func (h *HermesHandler) ImportHubSkill(c *gin.Context) {
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	var request service.HermesSkillHubImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid request body")
		return
	}
	skill, err := h.service.ImportHubSkill(c.Request.Context(), actorID, request)
	if err != nil {
		writeHermesError(c, err)
		return
	}
	response.SuccessWithMessage(c, "技能已导入，待审批后可绑定/自装", skill)
}
