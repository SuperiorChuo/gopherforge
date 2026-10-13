package api

// 消息记录列表：mp_messages 收发留档（tenant 隔离，按 id desc）。

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
)

const customerReplyWindow = 48 * time.Hour

type messageResponse struct {
	ID        uint64    `json:"id"`
	AccountID uint64    `json:"account_id"`
	OpenID    string    `json:"open_id"`
	Direction string    `json:"direction"`
	MsgType   string    `json:"msg_type"`
	Content   string    `json:"content"`
	MsgID     string    `json:"msg_id"`
	Event     string    `json:"event"`
	CreatedAt time.Time `json:"created_at"`
}

type customerReplyResponse struct {
	Sent     bool             `json:"sent"`
	Archived bool             `json:"archived"`
	Message  *messageResponse `json:"message,omitempty"`
}

func toMessageResponses(list []store.MpMessage) []messageResponse {
	out := make([]messageResponse, 0, len(list))
	for _, item := range list {
		out = append(out, messageResponse{
			ID: item.ID, AccountID: item.AccountID, OpenID: item.OpenID,
			Direction: item.Direction, MsgType: item.MsgType, Content: item.Content,
			MsgID: item.MsgID, Event: item.Event, CreatedAt: item.CreatedAt,
		})
	}
	return out
}

func (s *Server) ListMessages(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	page, pageSize := pageOf(c)
	list, total, err := s.Store.ListMessages(
		u.TenantID,
		queryUint64(c, "account_id"),
		c.Query("open_id"),
		c.Query("direction"),
		page, pageSize,
	)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": toMessageResponses(list), "total": total, "page": page, "page_size": pageSize})
}

// ReplyMessage POST /api/v1/mp/messages/:id/reply
// 回复目标从当前租户的入站消息派生，避免客户端自由指定 account/openid。
func (s *Server) ReplyMessage(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	messageID, ok2 := pathID(c, "id")
	if !ok2 {
		return
	}
	inbound, err := s.Store.GetMessage(messageID, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "入站消息不存在")
		return
	}
	if inbound.Direction != "in" || strings.TrimSpace(inbound.OpenID) == "" {
		fail(c, http.StatusUnprocessableEntity, "只能回复带 OpenID 的入站消息")
		return
	}
	account, err := s.Store.GetAccount(inbound.AccountID, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "公众号账号不存在")
		return
	}
	if !requireActiveAccount(c, account) {
		return
	}
	if !requireService(c, account) {
		return
	}
	recent, err := s.Store.HasRecentInboundMessage(
		u.TenantID, inbound.AccountID, inbound.OpenID, time.Now().Add(-customerReplyWindow),
	)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if !recent {
		fail(c, http.StatusConflict, "客服回复窗口已超过 48 小时")
		return
	}
	if s.WX == nil {
		fail(c, http.StatusServiceUnavailable, "微信客户端未配置（缺 Redis 或密钥）")
		return
	}
	var body struct {
		MsgType string `json:"msg_type"`
		Content string `json:"content"`
		MediaID string `json:"media_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.MsgType = strings.TrimSpace(body.MsgType)
	if body.MsgType == "" {
		body.MsgType = "text"
	}
	body.Content = strings.TrimSpace(body.Content)
	body.MediaID = strings.TrimSpace(body.MediaID)

	var sendErr error
	switch body.MsgType {
	case "text":
		if body.Content == "" || len([]byte(body.Content)) > 2048 {
			fail(c, http.StatusBadRequest, "content 必填且不能超过 2048 字节")
			return
		}
		sendErr = s.WX.SendCustomerText(c.Request.Context(), account, inbound.OpenID, body.Content)
	case "image":
		if body.MediaID == "" || len([]byte(body.MediaID)) > 128 {
			fail(c, http.StatusBadRequest, "media_id 必填且不能超过 128 字节")
			return
		}
		sendErr = s.WX.SendCustomerImage(c.Request.Context(), account, inbound.OpenID, body.MediaID)
	default:
		fail(c, http.StatusBadRequest, "msg_type 仅支持 text 或 image")
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
	archivedContent := body.Content
	if body.MsgType == "image" {
		archivedContent = body.MediaID
	}
	outbound := &store.MpMessage{
		TenantID: account.TenantID, AccountID: account.ID, OpenID: inbound.OpenID,
		Direction: "out", MsgType: body.MsgType, Content: archivedContent,
	}
	if err := s.Store.LogMessage(outbound); err != nil {
		log.Printf("mp customer reply archive failed: tenant_id=%d account_id=%d inbound_message_id=%d: %v", u.TenantID, account.ID, inbound.ID, err)
		ok(c, customerReplyResponse{Sent: true, Archived: false})
		return
	}
	archived := toMessageResponses([]store.MpMessage{*outbound})[0]
	ok(c, customerReplyResponse{Sent: true, Archived: true, Message: &archived})
}
