package api

// M4 微信主动能力后台接口：自定义菜单、客服消息、粉丝同步。均需 s.WX（微信 API
// 客户端）；未配置（无 Redis/密钥）时判空降级返回明确错误。订阅号（type=subscribe）
// 无客服/网页授权权限，调用前按 account.Type 拦截。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
	"gorm.io/gorm"
)

const maxCustomerImageBytes int64 = 10 << 20

var customerImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/bmp":  ".bmp",
}

// account 取当前租户下的账号；草稿读写不依赖微信客户端。
func (s *Server) account(c *gin.Context, tenantID uint64) (*store.MpAccount, bool) {
	id, ok2 := pathID(c, "id")
	if !ok2 {
		return nil, false
	}
	acc, err := s.Store.GetAccount(id, tenantID)
	if err != nil {
		notFoundOr(c, err, "账号不存在")
		return nil, false
	}
	return acc, true
}

// wxAccount 取当前租户下的账号，并校验 s.WX 已就绪。
func (s *Server) wxAccount(c *gin.Context, tenantID uint64) (*store.MpAccount, bool) {
	acc, ok2 := s.account(c, tenantID)
	if !ok2 {
		return nil, false
	}
	if !requireActiveAccount(c, acc) {
		return nil, false
	}
	if s.WX == nil {
		fail(c, http.StatusServiceUnavailable, "微信客户端未配置（缺 Redis 或密钥）")
		return nil, false
	}
	return acc, true
}

func requireActiveAccount(c *gin.Context, acc *store.MpAccount) bool {
	if acc.Status != "active" {
		fail(c, http.StatusConflict, "公众号账号已停用")
		return false
	}
	return true
}

// requireService 拦截订阅号：客服消息/网页授权仅服务号可用。
func requireService(c *gin.Context, acc *store.MpAccount) bool {
	if acc.Type != "service" {
		fail(c, http.StatusForbidden, "该能力仅服务号可用，当前为订阅号")
		return false
	}
	return true
}

// ---- 自定义菜单 ----

type menuResponse struct {
	AccountID uint64          `json:"account_id"`
	Buttons   json.RawMessage `json:"buttons"`
	Published bool            `json:"published"`
	UpdatedAt *time.Time      `json:"updated_at,omitempty"`
}

func menuData(accountID uint64, menu *store.MpMenu) menuResponse {
	if menu == nil {
		return menuResponse{AccountID: accountID, Buttons: json.RawMessage("[]")}
	}
	updatedAt := menu.UpdatedAt
	return menuResponse{
		AccountID: accountID, Buttons: json.RawMessage(menu.Buttons),
		Published: menu.Published, UpdatedAt: &updatedAt,
	}
}

func bindMenuButtons(c *gin.Context) (json.RawMessage, bool) {
	var body struct {
		Buttons json.RawMessage `json:"buttons"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMenuPayloadBytes+1024)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		fail(c, http.StatusBadRequest, "buttons 格式错误")
		return nil, false
	}
	if err := ensureJSONEOF(dec); err != nil {
		fail(c, http.StatusBadRequest, "buttons 格式错误")
		return nil, false
	}
	buttons, err := normalizeMenuButtons(body.Buttons)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return buttons, true
}

// GetMenu GET /api/v1/mp/accounts/:id/menu — 读取本地草稿，不依赖微信在线。
func (s *Server) GetMenu(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.account(c, u.TenantID)
	if !ok2 {
		return
	}
	menu, err := s.Store.GetMenu(acc.TenantID, acc.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ok(c, menuData(acc.ID, nil))
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Older releases left an empty tombstone row after deleting a menu.
	// Preserve upgrade compatibility by exposing it as an empty draft.
	if strings.TrimSpace(menu.Buttons) == "" {
		menu.Buttons = "[]"
		menu.Published = false
		ok(c, menuData(acc.ID, menu))
		return
	}
	normalized, err := normalizeMenuButtons(json.RawMessage(menu.Buttons))
	if err != nil {
		fail(c, http.StatusInternalServerError, "本地菜单草稿格式无效")
		return
	}
	menu.Buttons = string(normalized)
	ok(c, menuData(acc.ID, menu))
}

// SaveMenu PUT /api/v1/mp/accounts/:id/menu — 仅保存本地草稿。
func (s *Server) SaveMenu(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.account(c, u.TenantID)
	if !ok2 {
		return
	}
	buttons, ok2 := bindMenuButtons(c)
	if !ok2 {
		return
	}
	if err := s.Store.UpsertMenu(acc.TenantID, acc.ID, string(buttons), false); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	menu, err := s.Store.GetMenu(acc.TenantID, acc.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, menuData(acc.ID, menu))
}

// PublishMenu POST /api/v1/mp/accounts/:id/menu — 兼容旧调用：保存草稿并发布。
func (s *Server) PublishMenu(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.account(c, u.TenantID)
	if !ok2 {
		return
	}
	buttons, ok2 := bindMenuButtons(c)
	if !ok2 {
		return
	}
	if err := s.Store.UpsertMenu(acc.TenantID, acc.ID, string(buttons), false); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishMenu(c, acc, buttons, string(buttons))
}

// PublishSavedMenu POST /api/v1/mp/accounts/:id/menu/publish — 发布已保存草稿。
func (s *Server) PublishSavedMenu(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.account(c, u.TenantID)
	if !ok2 {
		return
	}
	menu, err := s.Store.GetMenu(acc.TenantID, acc.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusUnprocessableEntity, "请先保存菜单草稿")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	buttons, err := normalizeMenuButtons(json.RawMessage(menu.Buttons))
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "本地菜单草稿格式无效，请重新保存")
		return
	}
	s.publishMenu(c, acc, buttons, menu.Buttons)
}

func (s *Server) publishMenu(c *gin.Context, acc *store.MpAccount, buttons json.RawMessage, expectedButtons string) {
	if !requireActiveAccount(c, acc) {
		return
	}
	if s.WX == nil {
		fail(c, http.StatusServiceUnavailable, "菜单草稿已保存，但微信客户端未配置（缺 Redis 或密钥）")
		return
	}
	if err := s.WX.CreateMenu(c.Request.Context(), acc, buttons); err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	updated, err := s.Store.MarkMenuPublished(acc.TenantID, acc.ID, expectedButtons, string(buttons))
	if err != nil {
		fail(c, http.StatusInternalServerError, "微信菜单已发布，但本地发布状态保存失败")
		return
	}
	if !updated {
		fail(c, http.StatusConflict, "微信已发布当前版本，但本地已有更新草稿；请重新加载后再发布")
		return
	}
	menu, err := s.Store.GetMenu(acc.TenantID, acc.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, menuData(acc.ID, menu))
}

// DeleteMenu DELETE /api/v1/mp/accounts/:id/menu — 删除微信菜单。
func (s *Server) DeleteMenu(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.account(c, u.TenantID)
	if !ok2 {
		return
	}
	// Deleting the remote menu is a cleanup operation that must remain
	// available after an account is disabled.
	if s.WX == nil {
		fail(c, http.StatusServiceUnavailable, "微信客户端未配置（缺 Redis 或密钥）")
		return
	}
	if err := s.WX.DeleteMenu(c.Request.Context(), acc); err != nil {
		var wxErr *wxclient.WxError
		if !errors.As(err, &wxErr) || wxErr.Code != 46003 {
			fail(c, http.StatusBadGateway, err.Error())
			return
		}
	}
	if err := s.Store.DeleteMenu(acc.TenantID, acc.ID); err != nil {
		fail(c, http.StatusInternalServerError, "微信菜单已删除，但本地草稿清理失败")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ---- 客服消息 ----

// SendCustomerMessage POST /api/v1/mp/accounts/:id/customer-message
// body {open_id, msg_type:text|image, content|media_id}。
func (s *Server) SendCustomerMessage(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.wxAccount(c, u.TenantID)
	if !ok2 {
		return
	}
	if !requireService(c, acc) {
		return
	}
	var body struct {
		OpenID  string `json:"open_id"`
		MsgType string `json:"msg_type"`
		Content string `json:"content"`
		MediaID string `json:"media_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "open_id 必填")
		return
	}
	body.OpenID = strings.TrimSpace(body.OpenID)
	body.MsgType = strings.TrimSpace(body.MsgType)
	body.Content = strings.TrimSpace(body.Content)
	body.MediaID = strings.TrimSpace(body.MediaID)
	if body.OpenID == "" || len([]byte(body.OpenID)) > 128 {
		fail(c, http.StatusBadRequest, "open_id 必填且不能超过 128 字节")
		return
	}
	recent, err := s.Store.HasRecentInboundMessage(
		acc.TenantID, acc.ID, body.OpenID, time.Now().Add(-customerReplyWindow),
	)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if !recent {
		fail(c, http.StatusConflict, "客服回复窗口已超过 48 小时")
		return
	}
	ctx := c.Request.Context()
	var sendErr error
	archiveContent := body.Content
	switch body.MsgType {
	case "", "text":
		if body.Content == "" || len([]byte(body.Content)) > 2048 {
			fail(c, http.StatusBadRequest, "content 必填且不能超过 2048 字节")
			return
		}
		sendErr = s.WX.SendCustomerText(ctx, acc, body.OpenID, body.Content)
	case "image":
		if body.MediaID == "" || len([]byte(body.MediaID)) > 128 {
			fail(c, http.StatusBadRequest, "media_id 必填且不能超过 128 字节")
			return
		}
		archiveContent = body.MediaID
		sendErr = s.WX.SendCustomerImage(ctx, acc, body.OpenID, body.MediaID)
	default:
		fail(c, http.StatusBadRequest, "不支持的 msg_type")
		return
	}
	if sendErr != nil {
		var wxErr *wxclient.WxError
		if errors.As(sendErr, &wxErr) && wxErr.Code == 45015 {
			fail(c, http.StatusConflict, "客服回复窗口已超过 48 小时")
			return
		}
		fail(c, http.StatusBadGateway, sendErr.Error())
		return
	}
	// 留档一条出站客服消息。
	if err := s.Store.LogMessage(&store.MpMessage{
		TenantID: acc.TenantID, AccountID: acc.ID,
		OpenID: body.OpenID, Direction: "out",
		MsgType: firstNonEmpty(body.MsgType, "text"), Content: archiveContent,
	}); err != nil {
		log.Printf("mp customer message archive failed: tenant_id=%d account_id=%d: %v", acc.TenantID, acc.ID, err)
		ok(c, gin.H{"sent": true, "archived": false})
		return
	}
	ok(c, gin.H{"sent": true, "archived": true})
}

// UploadCustomerMedia POST /api/v1/mp/accounts/:id/customer-media
// 上传图片到微信临时素材库，供图片客服回复使用。临时素材由微信侧按期失效。
func (s *Server) UploadCustomerMedia(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.wxAccount(c, u.TenantID)
	if !ok2 {
		return
	}
	if !requireService(c, acc) {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCustomerImageBytes+(1<<20))
	header, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			fail(c, http.StatusRequestEntityTooLarge, "图片大小必须在 10 MB 以内")
			return
		}
		fail(c, http.StatusBadRequest, "图片文件必填")
		return
	}
	if header.Size <= 0 || header.Size > maxCustomerImageBytes {
		fail(c, http.StatusRequestEntityTooLarge, "图片大小必须在 10 MB 以内")
		return
	}

	file, err := header.Open()
	if err != nil {
		fail(c, http.StatusBadRequest, "读取图片失败")
		return
	}
	defer file.Close()

	prefix := make([]byte, 512)
	n, readErr := io.ReadFull(file, prefix)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		fail(c, http.StatusBadRequest, "读取图片失败")
		return
	}
	contentType := http.DetectContentType(prefix[:n])
	extension, allowed := customerImageTypes[contentType]
	if !allowed {
		fail(c, http.StatusUnsupportedMediaType, "仅支持 JPEG、PNG、GIF 或 BMP 图片")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		fail(c, http.StatusBadRequest, "读取图片失败")
		return
	}

	filename := filepath.Base(strings.TrimSpace(header.Filename))
	if filename == "." || filename == "" {
		filename = "customer-image" + extension
	}
	if filepath.Ext(filename) == "" {
		filename += extension
	}
	if detected := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); detected != "" && detected != contentType {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + extension
	}

	mediaID, err := s.WX.UploadTempMedia(c.Request.Context(), acc, wxclient.MediaTypeImage, filename, file)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	if strings.TrimSpace(mediaID) == "" {
		fail(c, http.StatusBadGateway, fmt.Sprintf("微信未返回图片 media_id（文件 %s）", filename))
		return
	}
	ok(c, gin.H{"media_id": mediaID})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
