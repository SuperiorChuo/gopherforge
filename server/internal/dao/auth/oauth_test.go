package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOAuthBindingDAOGetByProviderUser(t *testing.T) {
	mock := setupAuthDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `oauth_bindings` WHERE provider = ? AND provider_user_id = ? ORDER BY `oauth_bindings`.`id` LIMIT ?")).
		WithArgs("github", "123", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "provider", "provider_user_id"}).
			AddRow(7, 42, "github", "123"))

	binding, err := (OAuthBindingDAO{}).GetByProviderUser("github", "123")
	if err != nil {
		t.Fatalf("GetByProviderUser() error = %v", err)
	}
	if binding.UserID != 42 {
		t.Fatalf("binding user id = %d, want 42", binding.UserID)
	}
}

func TestOAuthBindingDAOGetByProviderUserContextHonorsCanceledContext(t *testing.T) {
	setupAuthDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (OAuthBindingDAO{}).GetByProviderUserContext(ctx, "github", "123")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByProviderUserContext() error = %v, want context.Canceled", err)
	}
}
