package sms

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AliyunConfig 阿里云短信配置（来自渠道 config JSON，密钥只落库不进代码）。
type AliyunConfig struct {
	AccessKeyID     string // access_key_id
	AccessKeySecret string // access_key_secret
	SignName        string // sign_name：审核通过的短信签名
	RegionID        string // region_id：默认 cn-hangzhou
	Endpoint        string // 可覆盖，默认 https://dysmsapi.aliyuncs.com/（单测注入用）
}

// AliyunSender 阿里云短信发送器：走 dysmsapi 的 RPC 风格 HTTP 接口，
// HMAC-SHA1 签名（POP 协议），不引第三方 SDK。
type AliyunSender struct {
	cfg        AliyunConfig
	httpClient *http.Client
	// now / nonce 可注入，签名单测需要确定性输出。
	now   func() time.Time
	nonce func() string
}

const aliyunDefaultEndpoint = "https://dysmsapi.aliyuncs.com/"

// NewAliyunSender 构造阿里云发送器。
func NewAliyunSender(cfg AliyunConfig) *AliyunSender {
	if cfg.RegionID == "" {
		cfg.RegionID = "cn-hangzhou"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = aliyunDefaultEndpoint
	}
	return &AliyunSender{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		now:        time.Now,
		nonce:      randomNonce,
	}
}

func (s *AliyunSender) Provider() string { return ProviderAliyun }

// aliyunResponse 是 SendSms 的响应体（Code == "OK" 表示受理成功）。
type aliyunResponse struct {
	Code      string `json:"Code"`
	Message   string `json:"Message"`
	BizID     string `json:"BizId"`
	RequestID string `json:"RequestId"`
}

// Send 调用 SendSms。阿里云按「模板号 + JSON 参数」发送，Content 仅本地留档。
func (s *AliyunSender) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	if s.cfg.AccessKeyID == "" || s.cfg.AccessKeySecret == "" {
		return nil, fmt.Errorf("aliyun sms: access key not configured")
	}
	if req.ProviderTemplateID == "" {
		return nil, fmt.Errorf("aliyun sms: provider_template_id is required")
	}

	templateParam := "{}"
	if len(req.Params) > 0 {
		raw, err := json.Marshal(req.Params)
		if err != nil {
			return nil, fmt.Errorf("aliyun sms: marshal template param: %w", err)
		}
		templateParam = string(raw)
	}

	sign := req.Sign
	if sign == "" {
		sign = s.cfg.SignName
	}

	params := map[string]string{
		// 公共参数
		"AccessKeyId":      s.cfg.AccessKeyID,
		"Action":           "SendSms",
		"Format":           "JSON",
		"RegionId":         s.cfg.RegionID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   s.nonce(),
		"SignatureVersion": "1.0",
		"Timestamp":        s.now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2017-05-25",
		// 业务参数
		"PhoneNumbers":  req.Mobile,
		"SignName":      sign,
		"TemplateCode":  req.ProviderTemplateID,
		"TemplateParam": templateParam,
	}
	params["Signature"] = aliyunSignature(s.cfg.AccessKeySecret, http.MethodPost, params)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("aliyun sms: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("aliyun sms: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("aliyun sms: read response: %w", err)
	}

	var parsed aliyunResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("aliyun sms: unexpected response (http %d): %s", resp.StatusCode, truncate(string(body), 200))
	}
	if parsed.Code != "OK" {
		return nil, fmt.Errorf("aliyun sms: %s (%s)", parsed.Code, parsed.Message)
	}
	return &SendResult{MessageID: parsed.BizID}, nil
}

// aliyunSignature 按阿里云 POP RPC 规范签名：
// 参数名排序 → 特殊 percent 编码拼 query → 构造 StringToSign → HMAC-SHA1(secret + "&")。
func aliyunSignature(secret, method string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "Signature" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var query strings.Builder
	for i, k := range keys {
		if i > 0 {
			query.WriteByte('&')
		}
		query.WriteString(aliyunPercentEncode(k))
		query.WriteByte('=')
		query.WriteString(aliyunPercentEncode(params[k]))
	}

	stringToSign := method + "&" + aliyunPercentEncode("/") + "&" + aliyunPercentEncode(query.String())
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// aliyunPercentEncode 是阿里云要求的 URL 编码变体：空格→%20、*→%2A、%7E→~。
func aliyunPercentEncode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

// randomNonce 生成签名去重用的随机串。
func randomNonce() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// truncate 截断过长的错误回显，避免日志爆炸。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
