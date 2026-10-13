// Package wxclient 是微信公众号 API 的通用封装层：access_token 中控（token.go）、
// 统一请求/上传（client.go）、素材上传（material.go）。所有对外方法都收
// *store.MpAccount，appid/appsecret 经 SecretResolver 解析，不硬编码密钥。
package wxclient

import "fmt"

func successfulHTTPStatus(status int) bool {
	return status >= 200 && status < 300
}

func unexpectedHTTPStatus(op string, status int) error {
	return fmt.Errorf("%s: unexpected HTTP status %d", op, status)
}

// requestError keeps the transport cause available to errors.Is/As while
// preventing query-string credentials from leaking through error messages.
type requestError struct {
	op  string
	err error
}

func (e *requestError) Error() string { return e.op + ": request failed" }
func (e *requestError) Unwrap() error { return e.err }

func safeRequestError(op string, err error) error {
	if err == nil {
		return nil
	}
	return &requestError{op: op, err: err}
}

// WxError 表示微信统一响应里的业务错误（errcode != 0）。
type WxError struct {
	Code int
	Msg  string
}

func (e *WxError) Error() string {
	return fmt.Sprintf("wx api error %d: %s", e.Code, e.Msg)
}

// wxCommonResp 是微信几乎所有 JSON 响应共有的 {errcode, errmsg} 头。
// 成功时 errcode 省略或为 0。
type wxCommonResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// asWxError 把 {errcode,errmsg} 转成 *WxError；errcode==0 返回 nil。
func (r wxCommonResp) asWxError() error {
	if r.ErrCode == 0 {
		return nil
	}
	return &WxError{Code: r.ErrCode, Msg: r.ErrMsg}
}

// IsTokenExpired 判定错误码是否属于 access_token 失效/过期，供上层失效重试。
//
//	40001 invalid credential / access_token isinvalid
//	42001 access_token expired
func IsTokenExpired(code int) bool {
	return code == 40001 || code == 42001
}

// asWxTokenExpired 从任意 error 里探测是否为可重试的 token 失效。
func asWxTokenExpired(err error) bool {
	if we, ok := err.(*WxError); ok {
		return IsTokenExpired(we.Code)
	}
	return false
}
