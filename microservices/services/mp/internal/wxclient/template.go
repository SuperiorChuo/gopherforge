package wxclient

import (
	"context"
	"net/http"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// templateSendPath 是模板消息发送端点。
const templateSendPath = "/cgi-bin/message/template/send"

// TemplateField 是模板消息里的单个字段值。Color 可选（如 "#173177"），留空则用
// 模板默认色。
type TemplateField struct {
	Value string `json:"value"`
	Color string `json:"color,omitempty"`
}

// TemplateMessage 是一条模板消息。Data 的 key 对应模板占位符名（如 first、keyword1）。
type TemplateMessage struct {
	ToUser     string                   // openid
	TemplateID string                   // 模板 id
	URL        string                   // 可选，点击模板消息跳转的地址
	Data       map[string]TemplateField // 模板字段
}

// templateSendReq 是发送请求体（微信要求的 snake_case 字段名）。
type templateSendReq struct {
	ToUser     string                   `json:"touser"`
	TemplateID string                   `json:"template_id"`
	URL        string                   `json:"url,omitempty"`
	Data       map[string]TemplateField `json:"data"`
}

// templateSendResp 是发送响应；成功带 msgid，失败带 errcode/errmsg。
type templateSendResp struct {
	wxCommonResp
	MsgID int64 `json:"msgid"`
}

// SendTemplate 发送一条模板消息，返回微信侧 msgid（可用于查询送达状态）。
//
//	POST /cgi-bin/message/template/send
//	{"touser":...,"template_id":...,"url":...,"data":{...}}
//	→ {"errcode":0,"errmsg":"ok","msgid":123}
func (c *Client) SendTemplate(ctx context.Context, account *store.MpAccount, msg TemplateMessage) (int64, error) {
	body := templateSendReq{
		ToUser:     msg.ToUser,
		TemplateID: msg.TemplateID,
		URL:        msg.URL,
		Data:       msg.Data,
	}
	var out templateSendResp
	if err := c.doJSON(ctx, account, http.MethodPost, templateSendPath, body, &out); err != nil {
		return 0, err
	}
	return out.MsgID, nil
}
