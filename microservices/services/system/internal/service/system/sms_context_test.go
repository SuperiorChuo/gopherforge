package system

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/sms"
	"gorm.io/gorm"
)

func TestMaskSmsChannelConfig(t *testing.T) {
	config := map[string]any{
		"access_key_id":     "placeholder-key",
		"access_key_secret": "placeholder-secret",
		"secret_key":        "placeholder-secret-2",
		"sign_name":         "测试签名",
	}
	masked := maskSmsChannelConfig(config)

	if masked["access_key_secret"] != smsSecretMask {
		t.Fatalf("access_key_secret = %v, want masked", masked["access_key_secret"])
	}
	if masked["secret_key"] != smsSecretMask {
		t.Fatalf("secret_key = %v, want masked", masked["secret_key"])
	}
	if masked["access_key_id"] != "placeholder-key" || masked["sign_name"] != "测试签名" {
		t.Fatalf("non-secret keys should stay intact: %v", masked)
	}
	// 原 map 不能被改动
	if config["access_key_secret"] != "placeholder-secret" {
		t.Fatal("maskSmsChannelConfig must not mutate the original map")
	}
	if maskSmsChannelConfig(nil) != nil {
		t.Fatal("nil config should stay nil")
	}
}

func TestMergeSmsChannelSecrets(t *testing.T) {
	existing := map[string]any{
		"access_key_id":     "old-key",
		"access_key_secret": "old-secret",
		"sign_name":         "旧签名",
	}

	// 回传脱敏占位 → 保留旧密钥；其余字段以新值为准
	merged := mergeSmsChannelSecrets(map[string]any{
		"access_key_id":     "new-key",
		"access_key_secret": smsSecretMask,
		"sign_name":         "新签名",
	}, existing)
	if merged["access_key_secret"] != "old-secret" {
		t.Fatalf("masked secret should keep old value, got %v", merged["access_key_secret"])
	}
	if merged["access_key_id"] != "new-key" || merged["sign_name"] != "新签名" {
		t.Fatalf("non-secret keys should take new values: %v", merged)
	}

	// 提供了新密钥 → 覆盖
	merged = mergeSmsChannelSecrets(map[string]any{"access_key_secret": "brand-new"}, existing)
	if merged["access_key_secret"] != "brand-new" {
		t.Fatalf("new secret should win, got %v", merged["access_key_secret"])
	}

	// 密钥留空 → 保留旧值
	merged = mergeSmsChannelSecrets(map[string]any{"access_key_secret": ""}, existing)
	if merged["access_key_secret"] != "old-secret" {
		t.Fatalf("empty secret should keep old value, got %v", merged["access_key_secret"])
	}

	// config 缺省 → 沿用旧 config
	if got := mergeSmsChannelSecrets(nil, existing); got["access_key_secret"] != "old-secret" {
		t.Fatalf("nil incoming should return existing, got %v", got)
	}
}

// fakeSmsSender 是可注入的假发送器。
type fakeSmsSender struct {
	result  *sms.SendResult
	err     error
	lastReq sms.SendRequest
}

func (f *fakeSmsSender) Provider() string { return "fake" }

func (f *fakeSmsSender) Send(_ context.Context, req sms.SendRequest) (*sms.SendResult, error) {
	f.lastReq = req
	return f.result, f.err
}

func newSmsSendServiceForTest(db *gorm.DB, sender sms.Sender, senderErr error) SmsSendService {
	return SmsSendService{
		channelDAO:  *systemdao.NewSmsChannelDAO(db),
		templateDAO: *systemdao.NewSmsTemplateDAO(db),
		logDAO:      *systemdao.NewSmsLogDAO(db),
		newSender: func(string, map[string]any) (sms.Sender, error) {
			return sender, senderErr
		},
	}
}

func expectSmsTemplateByCode(mock sqlmock.Sqlmock, status int8) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "sms_templates" WHERE tenant_id = $1 AND code = $2 ORDER BY "sms_templates"."id" LIMIT $3`)).
		WithArgs(uint(1), "user_register", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "code", "name", "channel_id", "content", "type", "provider_template_id", "status"}).
			AddRow(3, 1, "user_register", "注册验证码", 7, "您好 {name}，验证码 {code}", 1, "SMS_0000", status))
}

func expectSmsChannelByID(mock sqlmock.Sqlmock, status int8) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "sms_channels" WHERE tenant_id = $1 AND "sms_channels"."id" = $2 ORDER BY "sms_channels"."id" LIMIT $3`)).
		WithArgs(uint(1), uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "provider", "config", "status"}).
			AddRow(7, 1, "调试渠道", "debug", `{"sign_name":"测试签名"}`, status))
}

func expectSmsLogInsert(mock sqlmock.Sqlmock, logID uint) {
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "sms_logs"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(logID))
	mock.ExpectCommit()
}

func expectSmsLogResultUpdate(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "sms_logs" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestSmsSendServiceSendContextSuccess(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	expectSmsTemplateByCode(mock, 1)
	expectSmsChannelByID(mock, 1)
	expectSmsLogInsert(mock, 11)
	expectSmsLogResultUpdate(mock)

	sender := &fakeSmsSender{result: &sms.SendResult{MessageID: "biz-1"}}
	svc := newSmsSendServiceForTest(db, sender, nil)

	result, err := svc.SendContext(context.Background(), SendSmsRequest{
		Mobile:       "13800000000",
		TemplateCode: "user_register",
		Params:       map[string]string{"name": "张三", "code": "123456"},
	})
	if err != nil {
		t.Fatalf("SendContext() error = %v", err)
	}
	if result.Status != model.SmsStatusSuccess || result.LogID != 11 || result.ProviderMsgID != "biz-1" {
		t.Fatalf("result = %#v, want success/log 11/biz-1", result)
	}
	if result.Content != "您好 张三，验证码 123456" {
		t.Fatalf("rendered content = %q", result.Content)
	}
	if sender.lastReq.ProviderTemplateID != "SMS_0000" {
		t.Fatalf("sender got template id %q, want SMS_0000", sender.lastReq.ProviderTemplateID)
	}
}

func TestSmsSendServiceSendContextProviderFailureIsLogged(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	expectSmsTemplateByCode(mock, 1)
	expectSmsChannelByID(mock, 1)
	expectSmsLogInsert(mock, 12)
	expectSmsLogResultUpdate(mock)

	sender := &fakeSmsSender{err: errors.New("provider rejected")}
	svc := newSmsSendServiceForTest(db, sender, nil)

	result, err := svc.SendContext(context.Background(), SendSmsRequest{
		Mobile:       "13800000000",
		TemplateCode: "user_register",
		Params:       map[string]string{"name": "张三", "code": "123456"},
	})
	if err != nil {
		t.Fatalf("SendContext() error = %v, provider failure should be a business result", err)
	}
	if result.Status != model.SmsStatusFailure || result.Error == "" {
		t.Fatalf("result = %#v, want failure with error message", result)
	}
}

func TestSmsSendServiceSendContextMissingParams(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	expectSmsTemplateByCode(mock, 1)
	expectSmsChannelByID(mock, 1)
	// 缺参在发送前拦截：不应有日志写入

	svc := newSmsSendServiceForTest(db, &fakeSmsSender{}, nil)

	_, err := svc.SendContext(context.Background(), SendSmsRequest{
		Mobile:       "13800000000",
		TemplateCode: "user_register",
		Params:       map[string]string{"name": "张三"}, // 缺 code
	})
	if !errors.Is(err, ErrSmsParamsMissing) {
		t.Fatalf("SendContext() error = %v, want ErrSmsParamsMissing", err)
	}
}

func TestSmsSendServiceSendContextTemplateDisabled(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)

	expectSmsTemplateByCode(mock, 0)

	svc := newSmsSendServiceForTest(db, &fakeSmsSender{}, nil)

	_, err := svc.SendContext(context.Background(), SendSmsRequest{
		Mobile:       "13800000000",
		TemplateCode: "user_register",
	})
	if !errors.Is(err, ErrSmsTemplateDisabled) {
		t.Fatalf("SendContext() error = %v, want ErrSmsTemplateDisabled", err)
	}
}

func TestSmsChannelServiceGetListContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewSmsChannelServiceWithDB(db)
	_, _, err := svc.GetListContext(ctx, SmsChannelListRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetListContext() error = %v, want context.Canceled", err)
	}
}
