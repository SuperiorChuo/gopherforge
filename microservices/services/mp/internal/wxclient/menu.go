package wxclient

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// 自定义菜单端点。create 走 POST，get/delete 走 GET。
const (
	menuCreatePath = "/cgi-bin/menu/create"
	menuGetPath    = "/cgi-bin/menu/get"
	menuDeletePath = "/cgi-bin/menu/delete"
)

// menuCreateReq 包裹菜单按钮数组：{"button":[...]}。buttons 透传运营在前端编辑好的
// 微信 button 结构，wxclient 不校验其内部形态。
type menuCreateReq struct {
	Button json.RawMessage `json:"button"`
}

// CreateMenu 创建自定义菜单。buttons 是已序列化的微信 button 数组 JSON（如
// [{"type":"click","name":"今日歌曲","key":"V1001"}]），透传下发。
//
//	POST /cgi-bin/menu/create
//	{"button":buttons}
func (c *Client) CreateMenu(ctx context.Context, account *store.MpAccount, buttons json.RawMessage) error {
	body := menuCreateReq{Button: buttons}
	return c.doJSON(ctx, account, http.MethodPost, menuCreatePath, body, nil)
}

// GetMenu 查询当前生效的自定义菜单，返回微信原始 JSON（含 menu / conditionalmenu 等）。
//
//	GET /cgi-bin/menu/get
func (c *Client) GetMenu(ctx context.Context, account *store.MpAccount) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.doJSON(ctx, account, http.MethodGet, menuGetPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteMenu 删除全部自定义菜单（含个性化菜单）。
//
//	GET /cgi-bin/menu/delete
func (c *Client) DeleteMenu(ctx context.Context, account *store.MpAccount) error {
	return c.doJSON(ctx, account, http.MethodGet, menuDeletePath, nil, nil)
}
