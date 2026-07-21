package sms

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TencentConfig 腾讯云短信配置（来自渠道 config JSON，密钥只落库不进代码）。
type TencentConfig struct {
	SecretID  string // secret_id
	SecretKey string // secret_key
	SdkAppID  string // sdk_app_id：短信应用 ID
	SignName  string // sign_name：审核通过的短信签名
	Region    string // region：默认 ap-guangzhou
	Endpoint  string // 可覆盖，默认 https://sms.tencentcloudapi.com/（单测注入用）
}

// TencentSender 腾讯云短信发送器：POST JSON 到 sms.tencentcloudapi.com，
// TC3-HMAC-SHA256 签名，不引第三方 SDK。
// 注意：腾讯云模板参数是 {1}{2} 位次形式，这里把 Params 按 key 排序后依序传入
// TemplateParamSet；建议腾讯渠道的模板占位命名 {1}{2}... 与云端模板位次对齐。
type TencentSender struct {
	cfg        TencentConfig
	httpClient *http.Client
	now        func() time.Time // 可注入，签名单测需要确定性输出
}

const (
	tencentDefaultEndpoint = "https://sms.tencentcloudapi.com/"
	tencentHost            = "sms.tencentcloudapi.com"
	tencentService         = "sms"
	tencentAction          = "SendSms"
	tencentVersion         = "2021-01-11"
)

// NewTencentSender 构造腾讯云发送器。
func NewTencentSender(cfg TencentConfig) *TencentSender {
	if cfg.Region == "" {
		cfg.Region = "ap-guangzhou"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = tencentDefaultEndpoint
	}
	return &TencentSender{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		now:        time.Now,
	}
}

func (s *TencentSender) Provider() string { return ProviderTencent }

// tencentResponse 是 SendSms 的响应体。
type tencentResponse struct {
	Response struct {
		SendStatusSet []struct {
			SerialNo string `json:"SerialNo"`
			Code     string `json:"Code"`
			Message  string `json:"Message"`
		} `json:"SendStatusSet"`
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
		RequestID string `json:"RequestId"`
	} `json:"Response"`
}

// Send 调用 SendSms。腾讯云按「模板号 + 位次参数」发送，Content 仅本地留档。
func (s *TencentSender) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	if s.cfg.SecretID == "" || s.cfg.SecretKey == "" {
		return nil, fmt.Errorf("tencent sms: secret not configured")
	}
	if s.cfg.SdkAppID == "" {
		return nil, fmt.Errorf("tencent sms: sdk_app_id is required")
	}
	if req.ProviderTemplateID == "" {
		return nil, fmt.Errorf("tencent sms: provider_template_id is required")
	}

	mobile := req.Mobile
	if !strings.HasPrefix(mobile, "+") {
		mobile = "+86" + mobile // 国内手机号默认加 +86 前缀
	}

	sign := req.Sign
	if sign == "" {
		sign = s.cfg.SignName
	}

	// 位次参数：按占位 key 排序依序取值（ExtractParams 已稳定排序）。
	paramSet := make([]string, 0, len(req.Params))
	for _, key := range ExtractParams(req.Content) {
		if v, ok := req.Params[key]; ok {
			paramSet = append(paramSet, v)
		}
	}

	payload := map[string]any{
		"PhoneNumberSet":   []string{mobile},
		"SmsSdkAppId":      s.cfg.SdkAppID,
		"SignName":         sign,
		"TemplateId":       req.ProviderTemplateID,
		"TemplateParamSet": paramSet,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("tencent sms: marshal payload: %w", err)
	}

	now := s.now().UTC()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tencent sms: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Host", tencentHost)
	httpReq.Header.Set("X-TC-Action", tencentAction)
	httpReq.Header.Set("X-TC-Version", tencentVersion)
	httpReq.Header.Set("X-TC-Region", s.cfg.Region)
	httpReq.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", now.Unix()))
	httpReq.Header.Set("Authorization", tencentAuthorization(s.cfg.SecretID, s.cfg.SecretKey, string(body), now))

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tencent sms: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("tencent sms: read response: %w", err)
	}

	var parsed tencentResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("tencent sms: unexpected response (http %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	if parsed.Response.Error != nil {
		return nil, fmt.Errorf("tencent sms: %s (%s)", parsed.Response.Error.Code, parsed.Response.Error.Message)
	}
	if len(parsed.Response.SendStatusSet) == 0 {
		return nil, fmt.Errorf("tencent sms: empty send status set")
	}
	status := parsed.Response.SendStatusSet[0]
	if status.Code != "Ok" {
		return nil, fmt.Errorf("tencent sms: %s (%s)", status.Code, status.Message)
	}
	return &SendResult{MessageID: status.SerialNo}, nil
}

// tencentAuthorization 计算 TC3-HMAC-SHA256 的 Authorization 头。
func tencentAuthorization(secretID, secretKey, payload string, t time.Time) string {
	const algorithm = "TC3-HMAC-SHA256"
	const signedHeaders = "content-type;host;x-tc-action"
	date := t.UTC().Format("2006-01-02")

	// 1. 规范请求串
	canonicalHeaders := "content-type:application/json; charset=utf-8\n" +
		"host:" + tencentHost + "\n" +
		"x-tc-action:" + strings.ToLower(tencentAction) + "\n"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"", // query string 为空
		canonicalHeaders,
		signedHeaders,
		sha256Hex([]byte(payload)),
	}, "\n")

	// 2. 待签名串
	credentialScope := date + "/" + tencentService + "/tc3_request"
	stringToSign := strings.Join([]string{
		algorithm,
		fmt.Sprintf("%d", t.Unix()),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// 3. 派生签名密钥并签名
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, tencentService)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	return algorithm + " Credential=" + secretID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
