package sms

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		params  map[string]string
		want    string
	}{
		{
			name:    "basic replacement",
			content: "您好 {name}，验证码 {code}，5 分钟内有效。",
			params:  map[string]string{"name": "张三", "code": "123456"},
			want:    "您好 张三，验证码 123456，5 分钟内有效。",
		},
		{
			name:    "repeated placeholder",
			content: "{code} 与 {code} 相同",
			params:  map[string]string{"code": "9"},
			want:    "9 与 9 相同",
		},
		{
			name:    "missing param keeps placeholder",
			content: "您好 {name}，余额 {balance}",
			params:  map[string]string{"name": "李四"},
			want:    "您好 李四，余额 {balance}",
		},
		{
			name:    "no placeholders",
			content: "纯文本内容",
			params:  map[string]string{"name": "x"},
			want:    "纯文本内容",
		},
		{
			name:    "empty params",
			content: "您好 {name}",
			params:  nil,
			want:    "您好 {name}",
		},
		{
			name:    "positional style for tencent",
			content: "验证码 {1}，{2} 分钟内有效",
			params:  map[string]string{"1": "8888", "2": "5"},
			want:    "验证码 8888，5 分钟内有效",
		},
		{
			name:    "unmatched braces left intact",
			content: "字面量 {not closed 和 {ok}",
			params:  map[string]string{"ok": "好"},
			want:    "字面量 {not closed 和 好",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RenderTemplate(tt.content, tt.params); got != tt.want {
				t.Fatalf("RenderTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractParams(t *testing.T) {
	got := ExtractParams("您好 {name}，验证码 {code}，{name} 请查收 {order_no}")
	want := []string{"code", "name", "order_no"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractParams() = %v, want %v", got, want)
	}

	if got := ExtractParams("无占位"); got != nil {
		t.Fatalf("ExtractParams() = %v, want nil", got)
	}
}

func TestMissingParams(t *testing.T) {
	missing := MissingParams("{a} {b} {c}", map[string]string{"a": "1", "c": "  "})
	want := []string{"b", "c"}
	if !reflect.DeepEqual(missing, want) {
		t.Fatalf("MissingParams() = %v, want %v", missing, want)
	}

	if missing := MissingParams("{a}", map[string]string{"a": "1"}); missing != nil {
		t.Fatalf("MissingParams() = %v, want nil", missing)
	}
}

func TestNewSenderFromConfig(t *testing.T) {
	tests := []struct {
		provider string
		wantErr  bool
		want     string
	}{
		{provider: "debug", want: ProviderDebug},
		{provider: "aliyun", want: ProviderAliyun},
		{provider: "Tencent", want: ProviderTencent}, // 大小写不敏感
		{provider: "unknown", wantErr: true},
		{provider: "", wantErr: true},
	}
	for _, tt := range tests {
		sender, err := NewSenderFromConfig(tt.provider, map[string]any{"access_key_id": "placeholder"})
		if tt.wantErr {
			if err == nil {
				t.Fatalf("NewSenderFromConfig(%q) expected error", tt.provider)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NewSenderFromConfig(%q) error = %v", tt.provider, err)
		}
		if sender.Provider() != tt.want {
			t.Fatalf("Provider() = %q, want %q", sender.Provider(), tt.want)
		}
	}
}

func TestDebugSenderAlwaysSucceeds(t *testing.T) {
	result, err := NewDebugSender().Send(context.Background(), SendRequest{
		Mobile:  "13800000000",
		Content: "您好 张三",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result == nil {
		t.Fatal("Send() result is nil")
	}
}

func TestAliyunSignatureDeterministicAndBase64(t *testing.T) {
	params := map[string]string{
		"AccessKeyId":      "testid",
		"Action":           "SendSms",
		"Format":           "JSON",
		"PhoneNumbers":     "13800000000",
		"RegionId":         "cn-hangzhou",
		"SignName":         "测试签名",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   "fixed-nonce",
		"SignatureVersion": "1.0",
		"TemplateCode":     "SMS_0000",
		"TemplateParam":    `{"code":"1234"}`,
		"Timestamp":        "2026-01-01T00:00:00Z",
		"Version":          "2017-05-25",
	}

	sig1 := aliyunSignature("testsecret", http.MethodPost, params)
	sig2 := aliyunSignature("testsecret", http.MethodPost, params)
	if sig1 != sig2 {
		t.Fatalf("signature not deterministic: %q vs %q", sig1, sig2)
	}
	if sig1 == aliyunSignature("othersecret", http.MethodPost, params) {
		t.Fatal("signature should change with secret")
	}
	raw, err := base64.StdEncoding.DecodeString(sig1)
	if err != nil {
		t.Fatalf("signature is not base64: %v", err)
	}
	if len(raw) != 20 { // HMAC-SHA1 输出 20 字节
		t.Fatalf("signature length = %d, want 20", len(raw))
	}
}

func TestAliyunPercentEncode(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a b", "a%20b"},
		{"a*b", "a%2Ab"},
		{"a~b", "a~b"},
		{"a/b", "a%2Fb"},
	}
	for _, tt := range tests {
		if got := aliyunPercentEncode(tt.in); got != tt.want {
			t.Fatalf("aliyunPercentEncode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAliyunSenderSend(t *testing.T) {
	var captured capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured.raw = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Code":"OK","Message":"OK","BizId":"biz-123","RequestId":"req-1"}`))
	}))
	defer server.Close()

	sender := NewAliyunSender(AliyunConfig{
		AccessKeyID:     "placeholder-key",
		AccessKeySecret: "placeholder-secret",
		SignName:        "测试签名",
		Endpoint:        server.URL,
	})
	sender.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	sender.nonce = func() string { return "fixed-nonce" }

	result, err := sender.Send(context.Background(), SendRequest{
		Mobile:             "13800000000",
		Params:             map[string]string{"code": "123456"},
		ProviderTemplateID: "SMS_0000",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.MessageID != "biz-123" {
		t.Fatalf("MessageID = %q, want biz-123", result.MessageID)
	}
	for _, expect := range []string{"PhoneNumbers=13800000000", "TemplateCode=SMS_0000", "Signature="} {
		if !strings.Contains(captured.raw, expect) {
			t.Fatalf("request body missing %q: %s", expect, captured.raw)
		}
	}
}

// capturedRequest 只是捕获请求体的小容器。
type capturedRequest struct{ raw string }

func TestAliyunSenderSendFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Code":"isv.MOBILE_NUMBER_ILLEGAL","Message":"手机号非法"}`))
	}))
	defer server.Close()

	sender := NewAliyunSender(AliyunConfig{
		AccessKeyID:     "placeholder-key",
		AccessKeySecret: "placeholder-secret",
		Endpoint:        server.URL,
	})
	_, err := sender.Send(context.Background(), SendRequest{Mobile: "bad", ProviderTemplateID: "SMS_0000"})
	if err == nil || !strings.Contains(err.Error(), "MOBILE_NUMBER_ILLEGAL") {
		t.Fatalf("Send() error = %v, want provider code surfaced", err)
	}
}

func TestAliyunSenderRequiresTemplateID(t *testing.T) {
	sender := NewAliyunSender(AliyunConfig{AccessKeyID: "k", AccessKeySecret: "s"})
	if _, err := sender.Send(context.Background(), SendRequest{Mobile: "13800000000"}); err == nil {
		t.Fatal("Send() expected error when provider_template_id missing")
	}
}

func TestTencentAuthorizationFormat(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	auth1 := tencentAuthorization("placeholder-id", "placeholder-key", `{"a":1}`, at)
	auth2 := tencentAuthorization("placeholder-id", "placeholder-key", `{"a":1}`, at)
	if auth1 != auth2 {
		t.Fatalf("authorization not deterministic")
	}
	if !strings.HasPrefix(auth1, "TC3-HMAC-SHA256 Credential=placeholder-id/2026-01-01/sms/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature=") {
		t.Fatalf("authorization format unexpected: %s", auth1)
	}
	if auth1 == tencentAuthorization("placeholder-id", "other-key", `{"a":1}`, at) {
		t.Fatal("authorization should change with secret key")
	}
}

func TestTencentSenderSend(t *testing.T) {
	var capturedBody map[string]any
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"SendStatusSet":[{"SerialNo":"sn-1","Code":"Ok","Message":"send success"}],"RequestId":"req-1"}}`))
	}))
	defer server.Close()

	sender := NewTencentSender(TencentConfig{
		SecretID:  "placeholder-id",
		SecretKey: "placeholder-key",
		SdkAppID:  "1400000000",
		SignName:  "测试签名",
		Endpoint:  server.URL,
	})
	sender.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

	result, err := sender.Send(context.Background(), SendRequest{
		Mobile:             "13800000000",
		Content:            "验证码 {1}，{2} 分钟内有效",
		Params:             map[string]string{"1": "8888", "2": "5"},
		ProviderTemplateID: "1234567",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.MessageID != "sn-1" {
		t.Fatalf("MessageID = %q, want sn-1", result.MessageID)
	}

	if got := capturedHeaders.Get("X-TC-Action"); got != "SendSms" {
		t.Fatalf("X-TC-Action = %q", got)
	}
	if got := capturedHeaders.Get("Authorization"); !strings.HasPrefix(got, "TC3-HMAC-SHA256 ") {
		t.Fatalf("Authorization = %q", got)
	}

	phones, _ := capturedBody["PhoneNumberSet"].([]any)
	if len(phones) != 1 || phones[0] != "+8613800000000" {
		t.Fatalf("PhoneNumberSet = %v, want [+8613800000000]", phones)
	}
	params, _ := capturedBody["TemplateParamSet"].([]any)
	if len(params) != 2 || params[0] != "8888" || params[1] != "5" {
		t.Fatalf("TemplateParamSet = %v, want [8888 5]", params)
	}
}

func TestTencentSenderSendFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"AuthFailure.SignatureFailure","Message":"签名错误"},"RequestId":"req-1"}}`))
	}))
	defer server.Close()

	sender := NewTencentSender(TencentConfig{
		SecretID:  "placeholder-id",
		SecretKey: "placeholder-key",
		SdkAppID:  "1400000000",
		Endpoint:  server.URL,
	})
	_, err := sender.Send(context.Background(), SendRequest{Mobile: "13800000000", ProviderTemplateID: "1"})
	if err == nil || !strings.Contains(err.Error(), "AuthFailure.SignatureFailure") {
		t.Fatalf("Send() error = %v, want provider error surfaced", err)
	}
}
