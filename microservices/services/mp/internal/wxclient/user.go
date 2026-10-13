package wxclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// 用户信息 / 粉丝列表端点。
const (
	userInfoPath      = "/cgi-bin/user/info"
	userInfoBatchPath = "/cgi-bin/user/info/batchget"
	userGetPath       = "/cgi-bin/user/get"
	MaxBatchUserInfo  = 100
)

// WxUserInfo 是单个粉丝的详细信息（拉取自 /cgi-bin/user/info）。Subscribe=0 表示未
// 关注（此时多数字段为空）；Sex：0 未知 / 1 男 / 2 女。
type WxUserInfo struct {
	OpenID        string  `json:"openid"`
	Nickname      string  `json:"nickname"`
	Sex           int8    `json:"sex"`
	City          string  `json:"city"`
	Province      string  `json:"province"`
	Country       string  `json:"country"`
	HeadImgURL    string  `json:"headimgurl"`
	Subscribe     int8    `json:"subscribe"`
	SubscribeTime int64   `json:"subscribe_time"`
	UnionID       string  `json:"unionid"`
	Remark        string  `json:"remark"`
	TagIDList     []int64 `json:"tagid_list"`
}

// GetUserInfo 拉取单个粉丝的详细信息。lang 固定 zh_CN（城市/省份中文）。
//
//	GET /cgi-bin/user/info?openid=X&lang=zh_CN
func (c *Client) GetUserInfo(ctx context.Context, account *store.MpAccount, openID string) (WxUserInfo, error) {
	var out WxUserInfo
	path := userInfoPath + "?openid=" + url.QueryEscape(openID) + "&lang=zh_CN"
	if err := c.doJSON(ctx, account, http.MethodGet, path, nil, &out); err != nil {
		return WxUserInfo{}, err
	}
	return out, nil
}

type batchUserInfoItem struct {
	OpenID string `json:"openid"`
	Lang   string `json:"lang"`
}

type batchUserInfoRequest struct {
	UserList []batchUserInfoItem `json:"user_list"`
}

type batchUserInfoResponse struct {
	UserInfoList []WxUserInfo `json:"user_info_list"`
}

// BatchGetUserInfo retrieves at most 100 profiles in one WeChat request.
func (c *Client) BatchGetUserInfo(ctx context.Context, account *store.MpAccount, openIDs []string) ([]WxUserInfo, error) {
	if len(openIDs) == 0 {
		return nil, errors.New("openid batch is empty")
	}
	if len(openIDs) > MaxBatchUserInfo {
		return nil, fmt.Errorf("openid batch exceeds %d", MaxBatchUserInfo)
	}
	body := batchUserInfoRequest{UserList: make([]batchUserInfoItem, 0, len(openIDs))}
	for _, raw := range openIDs {
		openID := strings.TrimSpace(raw)
		if openID == "" {
			return nil, errors.New("openid batch contains empty value")
		}
		body.UserList = append(body.UserList, batchUserInfoItem{OpenID: openID, Lang: "zh_CN"})
	}
	var out batchUserInfoResponse
	if err := c.doJSON(ctx, account, http.MethodPost, userInfoBatchPath, body, &out); err != nil {
		return nil, err
	}
	return out.UserInfoList, nil
}

// fanListResp 是粉丝列表分页响应。data.openid 为本页 openid 数组；next_openid 为
// 下一页游标，为空表示已到底。
type fanListResp struct {
	Total int `json:"total"`
	Count int `json:"count"`
	Data  struct {
		OpenID []string `json:"openid"`
	} `json:"data"`
	NextOpenID string `json:"next_openid"`
}

type FanOpenIDPage struct {
	Total      int
	Count      int
	OpenIDs    []string
	NextOpenID string
}

// ListFanPage returns the full pagination metadata needed by the durable sync
// worker. An empty NextOpenID is the only completion signal.
func (c *Client) ListFanPage(ctx context.Context, account *store.MpAccount, nextOpenID string) (FanOpenIDPage, error) {
	path := userGetPath
	if nextOpenID != "" {
		path += "?next_openid=" + url.QueryEscape(nextOpenID)
	}
	var out fanListResp
	if err := c.doJSON(ctx, account, http.MethodGet, path, nil, &out); err != nil {
		return FanOpenIDPage{}, err
	}
	return FanOpenIDPage{
		Total: out.Total, Count: out.Count, OpenIDs: out.Data.OpenID, NextOpenID: out.NextOpenID,
	}, nil
}

// ListFanOpenIDs 分页拉取粉丝 openid 列表。nextOpenID 为空时从头拉；返回本页 openid
// 数组和下一页游标 next（空表示已到最后一页）。
//
//	GET /cgi-bin/user/get?next_openid=X
func (c *Client) ListFanOpenIDs(ctx context.Context, account *store.MpAccount, nextOpenID string) ([]string, string, error) {
	page, err := c.ListFanPage(ctx, account, nextOpenID)
	if err != nil {
		return nil, "", err
	}
	return page.OpenIDs, page.NextOpenID, nil
}
