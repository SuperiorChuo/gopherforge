package wxclient

import (
	"context"
	"net/http"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// customerSendPath 是客服消息发送端点。48 小时客服窗口内可主动下发（服务号专属，
// 订阅号无此权限，微信会返 errcode，wxclient 层不做降级判断）。
const customerSendPath = "/cgi-bin/message/custom/send"

// SendCustomerText 发文本客服消息。touser=openid，content 为纯文本内容。
//
//	POST /cgi-bin/message/custom/send
//	{"touser":openID,"msgtype":"text","text":{"content":content}}
func (c *Client) SendCustomerText(ctx context.Context, account *store.MpAccount, openID, content string) error {
	body := map[string]any{
		"touser":  openID,
		"msgtype": "text",
		"text":    map[string]string{"content": content},
	}
	return c.doJSON(ctx, account, http.MethodPost, customerSendPath, body, nil)
}

// SendCustomerImage 发图片客服消息。mediaID 为已上传素材 id（临时或永久皆可）。
//
//	POST /cgi-bin/message/custom/send
//	{"touser":openID,"msgtype":"image","image":{"media_id":mediaID}}
func (c *Client) SendCustomerImage(ctx context.Context, account *store.MpAccount, openID, mediaID string) error {
	body := map[string]any{
		"touser":  openID,
		"msgtype": "image",
		"image":   map[string]string{"media_id": mediaID},
	}
	return c.doJSON(ctx, account, http.MethodPost, customerSendPath, body, nil)
}
