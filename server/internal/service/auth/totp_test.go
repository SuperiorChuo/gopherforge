package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/pquerna/otp/totp"
)

func TestLoginPasswordContextReturnsTOTPChallengeWhenEnabled(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.JWT.Secret = "local-dev-secret-for-totp-tests-32"
	config.Cfg.JWT.Issuer = "go-admin-kit-test"
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	currentHash := mustHashPasswordForTest(t, "CurrentPass1")
	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE username = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs("alice", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "must_change_password", "totp_enabled"}).
			AddRow(uint(7), "alice", currentHash, int8(1), false, true))

	resp, err := (&svc).LoginPasswordContext(context.Background(), "alice", "CurrentPass1")
	if err != nil {
		t.Fatalf("LoginPasswordContext() error = %v", err)
	}
	if !resp.RequiresTOTP || resp.TOTPChallengeID == "" {
		t.Fatalf("LoginPasswordContext() requires_totp=%v challenge=%q, want challenge", resp.RequiresTOTP, resp.TOTPChallengeID)
	}
	if resp.AccessToken != "" || resp.RefreshToken != "" {
		t.Fatal("TOTP challenge response must not include session tokens")
	}
	claims, err := jwt.ParseTOTPChallenge(resp.TOTPChallengeID)
	if err != nil {
		t.Fatalf("ParseTOTPChallenge() error = %v", err)
	}
	if claims.UserID != 7 || claims.Username != "alice" {
		t.Fatalf("challenge claims = %#v, want alice/7", claims)
	}
}

func TestVerifyTOTPLoginContextIssuesTokensForValidCode(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	restoreStore := jwt.SetTokenBlacklistStore(newTOTPTestBlacklistStore())
	t.Cleanup(restoreStore)
	oldCfg := config.Cfg
	config.Cfg.JWT.Secret = "local-dev-secret-for-totp-tests-32"
	config.Cfg.JWT.Issuer = "go-admin-kit-test"
	config.Cfg.JWT.AccessTokenExpire = 3600
	config.Cfg.JWT.RefreshTokenExpire = 86400
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	secret := "JBSWY3DPEHPK3PXP"
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}
	challenge, err := jwt.GenerateTOTPChallenge(7, "alice", 5*time.Minute)
	if err != nil {
		t.Fatalf("GenerateTOTPChallenge() error = %v", err)
	}

	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE \"users\".\"id\" = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "status", "totp_enabled", "totp_secret"}).
			AddRow(uint(7), "alice", int8(1), true, secret))

	resp, err := (&svc).VerifyTOTPLoginContext(context.Background(), VerifyTOTPLoginRequest{
		ChallengeID: challenge,
		Code:        code,
	})
	if err != nil {
		t.Fatalf("VerifyTOTPLoginContext() error = %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("VerifyTOTPLoginContext() did not issue session tokens")
	}
	_, err = jwt.ParseTOTPChallenge(challenge)
	if !errors.Is(err, jwt.ErrRevokedToken) {
		t.Fatalf("ParseTOTPChallenge() after successful login error = %v, want ErrRevokedToken", err)
	}
}

func TestVerifyTOTPLoginContextAcceptsRecoveryCodeOnce(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	restoreStore := jwt.SetTokenBlacklistStore(newTOTPTestBlacklistStore())
	t.Cleanup(restoreStore)
	oldCfg := config.Cfg
	config.Cfg.JWT.Secret = "local-dev-secret-for-totp-tests-32"
	config.Cfg.JWT.Issuer = "go-admin-kit-test"
	config.Cfg.JWT.AccessTokenExpire = 3600
	config.Cfg.JWT.RefreshTokenExpire = 86400
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	recoveryCode := "ABCDE-FGHIJ-KLMNO"
	codeHash := mustHashPasswordForTest(t, "ABCDEFGHIJKLMNO")
	challenge, err := jwt.GenerateTOTPChallenge(7, "alice", 5*time.Minute)
	if err != nil {
		t.Fatalf("GenerateTOTPChallenge() error = %v", err)
	}

	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE \"users\".\"id\" = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "status", "totp_enabled", "totp_secret"}).
			AddRow(uint(7), "alice", int8(1), true, "JBSWY3DPEHPK3PXP"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "totp_recovery_codes" WHERE user_id = $1 AND used_at IS NULL ORDER BY id ASC`)).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "code_hash", "used_at"}).
			AddRow(uint(11), uint(7), codeHash, nil))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "totp_recovery_codes" SET "used_at"=$1,"updated_at"=$2 WHERE user_id = $3 AND id = $4 AND used_at IS NULL`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7), uint(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	resp, err := (&svc).VerifyTOTPLoginContext(context.Background(), VerifyTOTPLoginRequest{
		ChallengeID: challenge,
		Code:        recoveryCode,
	})
	if err != nil {
		t.Fatalf("VerifyTOTPLoginContext() error = %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("VerifyTOTPLoginContext() did not issue session tokens for recovery code")
	}
}

func TestVerifyTOTPLoginContextRejectsInvalidRecoveryCodeWithoutMarkingUsed(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.JWT.Secret = "local-dev-secret-for-totp-tests-32"
	config.Cfg.JWT.Issuer = "go-admin-kit-test"
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	codeHash := mustHashPasswordForTest(t, "ABCDEFGHIJKLMNO")
	challenge, err := jwt.GenerateTOTPChallenge(7, "alice", 5*time.Minute)
	if err != nil {
		t.Fatalf("GenerateTOTPChallenge() error = %v", err)
	}

	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE \"users\".\"id\" = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "status", "totp_enabled", "totp_secret"}).
			AddRow(uint(7), "alice", int8(1), true, "JBSWY3DPEHPK3PXP"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "totp_recovery_codes" WHERE user_id = $1 AND used_at IS NULL ORDER BY id ASC`)).
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "code_hash", "used_at"}).
			AddRow(uint(11), uint(7), codeHash, nil))

	_, err = (&svc).VerifyTOTPLoginContext(context.Background(), VerifyTOTPLoginRequest{
		ChallengeID: challenge,
		Code:        "ZZZZZ-ZZZZZ-ZZZZZ",
	})
	if !errors.Is(err, ErrTOTPInvalid) {
		t.Fatalf("VerifyTOTPLoginContext() error = %v, want ErrTOTPInvalid", err)
	}
}

func TestVerifyTOTPLoginContextRejectsInvalidTOTPWithoutRecoveryLookup(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.JWT.Secret = "local-dev-secret-for-totp-tests-32"
	config.Cfg.JWT.Issuer = "go-admin-kit-test"
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	challenge, err := jwt.GenerateTOTPChallenge(7, "alice", 5*time.Minute)
	if err != nil {
		t.Fatalf("GenerateTOTPChallenge() error = %v", err)
	}

	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE \"users\".\"id\" = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "status", "totp_enabled", "totp_secret"}).
			AddRow(uint(7), "alice", int8(1), true, "JBSWY3DPEHPK3PXP"))

	_, err = (&svc).VerifyTOTPLoginContext(context.Background(), VerifyTOTPLoginRequest{
		ChallengeID: challenge,
		Code:        "000000",
	})
	if !errors.Is(err, ErrTOTPInvalid) {
		t.Fatalf("VerifyTOTPLoginContext() error = %v, want ErrTOTPInvalid", err)
	}
}

func TestGenerateTOTPSetupContextStoresSecretDisabled(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)
	oldCfg := config.Cfg
	config.Cfg.App.Name = "go-admin-kit"
	t.Cleanup(func() {
		config.Cfg = oldCfg
	})

	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE \"users\".\"id\" = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "totp_enabled"}).
			AddRow(uint(7), "alice", mustHashPasswordForTest(t, "CurrentPass1"), int8(1), false))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE \"users\" SET \"totp_enabled\"=\\$\\d+,\"totp_secret\"=\\$\\d+,\"updated_at\"=\\$\\d+ WHERE id = \\$\\d+").
		WithArgs(false, sqlmock.AnyArg(), sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	setup, err := (&svc).GenerateTOTPSetupContext(context.Background(), 7, TOTPSetupRequest{
		CurrentPassword: "CurrentPass1",
	})
	if err != nil {
		t.Fatalf("GenerateTOTPSetupContext() error = %v", err)
	}
	if setup.Secret == "" || setup.OTPAuthURL == "" {
		t.Fatalf("setup = %#v, want secret and URL", setup)
	}
}

func TestEnableTOTPContextReturnsRecoveryCodes(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)

	secret := "JBSWY3DPEHPK3PXP"
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}

	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE \"users\".\"id\" = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "totp_enabled", "totp_secret"}).
			AddRow(uint(7), "alice", mustHashPasswordForTest(t, "CurrentPass1"), int8(1), false, secret))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE \"users\" SET \"totp_enabled\"=\\$\\d+,\"updated_at\"=\\$\\d+ WHERE id = \\$\\d+").
		WithArgs(true, sqlmock.AnyArg(), uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "totp_recovery_codes" WHERE user_id = $1`)).
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 8))
	mock.ExpectQuery("INSERT INTO \"totp_recovery_codes\"").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(1).AddRow(2).AddRow(3).AddRow(4).AddRow(5).AddRow(6).AddRow(7).AddRow(8))
	mock.ExpectCommit()

	resp, err := (&svc).EnableTOTPContext(context.Background(), 7, TOTPVerifyRequest{
		Code:            code,
		CurrentPassword: "CurrentPass1",
	})
	if err != nil {
		t.Fatalf("EnableTOTPContext() error = %v", err)
	}
	if len(resp.RecoveryCodes) != 8 {
		t.Fatalf("RecoveryCodes len = %d, want 8", len(resp.RecoveryCodes))
	}
	for _, code := range resp.RecoveryCodes {
		if !regexp.MustCompile(`^[A-Z2-7]{5}-[A-Z2-7]{5}-[A-Z2-7]{5}$`).MatchString(code) {
			t.Fatalf("recovery code %q does not match display format", code)
		}
	}
}

func TestEnableTOTPContextRejectsMissingCurrentPassword(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)

	secret := "JBSWY3DPEHPK3PXP"
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}

	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE \"users\".\"id\" = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "totp_enabled", "totp_secret"}).
			AddRow(uint(7), "alice", mustHashPasswordForTest(t, "CurrentPass1"), int8(1), false, secret))

	_, err = (&svc).EnableTOTPContext(context.Background(), 7, TOTPVerifyRequest{Code: code})
	if !errors.Is(err, ErrOldPasswordIncorrect) {
		t.Fatalf("EnableTOTPContext() error = %v, want ErrOldPasswordIncorrect", err)
	}
}

func TestRegenerateTOTPRecoveryCodesContextReturnsNewCodes(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	svc := NewUserServiceWithDB(db)

	secret := "JBSWY3DPEHPK3PXP"
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}

	mock.ExpectQuery("SELECT \\* FROM \"users\" WHERE \"users\".\"id\" = \\$\\d+ ORDER BY \"users\".\"id\" LIMIT \\$\\d+").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password", "status", "totp_enabled", "totp_secret"}).
			AddRow(uint(7), "alice", mustHashPasswordForTest(t, "CurrentPass1"), int8(1), true, secret))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "totp_recovery_codes" WHERE user_id = $1`)).
		WithArgs(uint(7)).
		WillReturnResult(sqlmock.NewResult(0, 8))
	mock.ExpectQuery("INSERT INTO \"totp_recovery_codes\"").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(1).AddRow(2).AddRow(3).AddRow(4).AddRow(5).AddRow(6).AddRow(7).AddRow(8))
	mock.ExpectCommit()

	resp, err := (&svc).RegenerateTOTPRecoveryCodesContext(context.Background(), 7, TOTPVerifyRequest{
		Code:            code,
		CurrentPassword: "CurrentPass1",
	})
	if err != nil {
		t.Fatalf("RegenerateTOTPRecoveryCodesContext() error = %v", err)
	}
	if len(resp.RecoveryCodes) != 8 {
		t.Fatalf("RecoveryCodes len = %d, want 8", len(resp.RecoveryCodes))
	}
}

type totpTestBlacklistStore struct {
	values map[string]time.Duration
}

func newTOTPTestBlacklistStore() *totpTestBlacklistStore {
	return &totpTestBlacklistStore{values: make(map[string]time.Duration)}
}

func (s *totpTestBlacklistStore) SetTokenID(_ context.Context, tokenID string, ttl time.Duration) error {
	s.values[tokenID] = ttl
	return nil
}

func (s *totpTestBlacklistStore) HasTokenID(_ context.Context, tokenID string) (bool, error) {
	_, ok := s.values[tokenID]
	return ok, nil
}
