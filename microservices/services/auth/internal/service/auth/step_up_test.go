package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-admin-kit/services/shared/pkg/exportproof"
	"github.com/pquerna/otp/totp"
	goredis "github.com/redis/go-redis/v9"
)

func TestEdgeCertificateExportStepUpAlwaysVerifiesPassword(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	client := newStepUpTestRedis(t)
	service := NewEdgeCertificateExportStepUpServiceWithDeps(db, client)

	expectStepUpUser(mock, 7, mustHashPasswordForTest(t, "CurrentPass1"), false, "", true, 1)
	_, err := service.VerifyAndIssueContext(context.Background(), 7, "session-123", EdgeCertificateExportStepUpRequest{
		CurrentPassword: "WrongPass1",
		CertificateID:   42,
	})
	if !errors.Is(err, ErrStepUpVerificationFailed) {
		t.Fatalf("VerifyAndIssueContext() error = %v, want generic verification failure", err)
	}
}

func TestEdgeCertificateExportStepUpRequiresTOTPEnrollment(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	service := NewEdgeCertificateExportStepUpServiceWithDeps(db, newStepUpTestRedis(t))

	expectStepUpUser(mock, 7, mustHashPasswordForTest(t, "CurrentPass1"), false, "", true, 1)
	_, err := service.VerifyAndIssueContext(context.Background(), 7, "session-123", EdgeCertificateExportStepUpRequest{
		CurrentPassword: "CurrentPass1",
		TOTPCode:        "123456",
		CertificateID:   42,
	})
	if !errors.Is(err, ErrStepUpVerificationFailed) {
		t.Fatalf("VerifyAndIssueContext(TOTP disabled) error = %v, want generic verification failure", err)
	}
}

func TestEdgeCertificateExportStepUpRequiresTOTPWhenEnabled(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	client := newStepUpTestRedis(t)
	service := NewEdgeCertificateExportStepUpServiceWithDeps(db, client)
	secret := "JBSWY3DPEHPK3PXP"

	expectStepUpUser(mock, 7, mustHashPasswordForTest(t, "CurrentPass1"), true, secret, true, 1)
	_, err := service.VerifyAndIssueContext(context.Background(), 7, "session-123", EdgeCertificateExportStepUpRequest{
		CurrentPassword: "CurrentPass1",
		CertificateID:   42,
	})
	if !errors.Is(err, ErrStepUpVerificationFailed) {
		t.Fatalf("VerifyAndIssueContext(no TOTP) error = %v, want generic verification failure", err)
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}
	expectStepUpUser(mock, 7, mustHashPasswordForTest(t, "CurrentPass1"), true, secret, true, 1)
	response, err := service.VerifyAndIssueContext(context.Background(), 7, "session-123", EdgeCertificateExportStepUpRequest{
		CurrentPassword: "CurrentPass1",
		TOTPCode:        code,
		CertificateID:   42,
	})
	if err != nil {
		t.Fatalf("VerifyAndIssueContext(valid TOTP) error = %v", err)
	}
	if response.Proof == "" || response.ExpiresInSeconds != 120 {
		t.Fatalf("response = %#v, want two-minute opaque proof", response)
	}

	consumer := exportproof.NewStore(client)
	if err := consumer.Consume(context.Background(), response.Proof, exportproof.Binding{
		UserID:       7,
		SessionID:    "session-123",
		ResourceType: exportproof.ResourceTypeEdgeCertificate,
		ResourceID:   42,
		Audience:     exportproof.AudienceEdgeCertificateExport,
	}); err != nil {
		t.Fatalf("issued proof has wrong binding: %v", err)
	}
}

func TestEdgeCertificateExportStepUpRechecksPlatformAdminAndStatus(t *testing.T) {
	cases := []struct {
		name          string
		platformAdmin bool
		status        int8
	}{
		{name: "not platform admin", platformAdmin: false, status: 1},
		{name: "disabled", platformAdmin: true, status: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := setupAuthServiceContextTestDB(t)
			service := NewEdgeCertificateExportStepUpServiceWithDeps(db, newStepUpTestRedis(t))
			expectStepUpUser(mock, 7, mustHashPasswordForTest(t, "CurrentPass1"), false, "", tc.platformAdmin, tc.status)
			_, err := service.VerifyAndIssueContext(context.Background(), 7, "session-123", EdgeCertificateExportStepUpRequest{
				CurrentPassword: "CurrentPass1",
				CertificateID:   42,
			})
			if !errors.Is(err, ErrStepUpVerificationFailed) {
				t.Fatalf("VerifyAndIssueContext() error = %v, want generic verification failure", err)
			}
		})
	}
}

func TestEdgeCertificateExportStepUpFailsClosedWithoutDependencies(t *testing.T) {
	request := EdgeCertificateExportStepUpRequest{CurrentPassword: "CurrentPass1", CertificateID: 42}
	db, _ := setupAuthServiceContextTestDB(t)
	for name, service := range map[string]*EdgeCertificateExportStepUpService{
		"database": NewEdgeCertificateExportStepUpServiceWithDeps(nil, newStepUpTestRedis(t)),
		"redis":    NewEdgeCertificateExportStepUpServiceWithDeps(db, nil),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.VerifyAndIssueContext(context.Background(), 7, "session-123", request); !errors.Is(err, ErrStepUpUnavailable) {
				t.Fatalf("VerifyAndIssueContext() error = %v, want ErrStepUpUnavailable", err)
			}
		})
	}
}

func TestEdgeCertificateExportStepUpRateLimitsBeforeDatabaseAndRecovers(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	client, redisServer := newStepUpTestRedisWithServer(t)
	service := NewEdgeCertificateExportStepUpServiceWithDeps(db, client)
	request := EdgeCertificateExportStepUpRequest{
		CurrentPassword: "wrong",
		TOTPCode:        "000000",
		CertificateID:   42,
	}
	passwordHash := mustHashPasswordForTest(t, "CurrentPass1")

	// Calls 1-5 pass the limiter and reach the DB. Call 6 must be rejected
	// before another query, proving the expensive bcrypt path is not reached.
	for attempt := 1; attempt <= StepUpRateLimitAttempts; attempt++ {
		expectStepUpUser(mock, 7, passwordHash, true, "JBSWY3DPEHPK3PXP", true, 1)
		if _, err := service.VerifyAndIssueContext(context.Background(), 7, "session-rate-limit", request); !errors.Is(err, ErrStepUpVerificationFailed) {
			t.Fatalf("attempt %d error = %v, want credential failure after limiter", attempt, err)
		}
	}
	if _, err := service.VerifyAndIssueContext(context.Background(), 7, "session-rate-limit", request); !errors.Is(err, ErrStepUpRateLimited) {
		t.Fatalf("attempt %d error = %v, want ErrStepUpRateLimited", StepUpRateLimitAttempts+1, err)
	}

	redisServer.FastForward(StepUpRateLimitWindow + time.Second)
	expectStepUpUser(mock, 7, passwordHash, true, "JBSWY3DPEHPK3PXP", true, 1)
	if _, err := service.VerifyAndIssueContext(context.Background(), 7, "session-rate-limit", request); !errors.Is(err, ErrStepUpVerificationFailed) {
		t.Fatalf("attempt after window error = %v, want limiter reset and credential path", err)
	}
}

func TestEdgeCertificateExportStepUpFailsClosedWhenLimiterRedisFails(t *testing.T) {
	db, _ := setupAuthServiceContextTestDB(t)
	client, redisServer := newStepUpTestRedisWithServer(t)
	service := NewEdgeCertificateExportStepUpServiceWithDeps(db, client)
	redisServer.Close()

	_, err := service.VerifyAndIssueContext(context.Background(), 7, "session-redis-down", EdgeCertificateExportStepUpRequest{
		CurrentPassword: "CurrentPass1",
		TOTPCode:        "123456",
		CertificateID:   42,
	})
	if !errors.Is(err, ErrStepUpUnavailable) {
		t.Fatalf("VerifyAndIssueContext(Redis down) error = %v, want ErrStepUpUnavailable", err)
	}
}

func expectStepUpUser(mock sqlmock.Sqlmock, userID uint, passwordHash string, totpEnabled bool, totpSecret string, platformAdmin bool, status int8) {
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE "users"\."id" = \$\d+ ORDER BY "users"\."id" LIMIT \$\d+`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "is_platform_admin", "totp_enabled", "totp_secret"}).
			AddRow(userID, "alice", passwordHash, status, platformAdmin, totpEnabled, totpSecret))
}

func newStepUpTestRedis(t *testing.T) *goredis.Client {
	t.Helper()
	client, _ := newStepUpTestRedisWithServer(t)
	return client
}

func newStepUpTestRedisWithServer(t *testing.T) (*goredis.Client, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, server
}
