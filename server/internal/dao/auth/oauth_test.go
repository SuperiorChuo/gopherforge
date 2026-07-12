package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOAuthBindingDAOGetByProviderUserContext(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `oauth_bindings` WHERE provider = ? AND provider_user_id = ? ORDER BY `oauth_bindings`.`id` LIMIT ?")).
		WithArgs("github", "123", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "provider", "provider_user_id"}).
			AddRow(7, 42, "github", "123"))

	binding, err := NewOAuthBindingDAO(db).GetByProviderUserContext(context.Background(), "github", "123")
	if err != nil {
		t.Fatalf("GetByProviderUserContext() error = %v", err)
	}
	if binding.UserID != 42 {
		t.Fatalf("binding user id = %d, want 42", binding.UserID)
	}
}

func TestOAuthBindingDAOGetByProviderUserContextHonorsCanceledContext(t *testing.T) {
	db, _ := newAuthDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewOAuthBindingDAO(db).GetByProviderUserContext(ctx, "github", "123")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByProviderUserContext() error = %v, want context.Canceled", err)
	}
}

func TestOAuthBindingDAOGetByUserProviderContext(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `oauth_bindings` WHERE user_id = ? AND provider = ? ORDER BY `oauth_bindings`.`id` LIMIT ?")).
		WithArgs(42, "github", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "provider", "provider_user_id"}).
			AddRow(7, 42, "github", "123"))

	binding, err := NewOAuthBindingDAO(db).GetByUserProviderContext(context.Background(), 42, "github")
	if err != nil {
		t.Fatalf("GetByUserProviderContext() error = %v", err)
	}
	if binding.ProviderUserID != "123" {
		t.Fatalf("provider user id = %q, want 123", binding.ProviderUserID)
	}
}

func TestOAuthBindingDAODeleteByUserProviderContextConstrainsCurrentUser(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `oauth_bindings` WHERE user_id = ? AND provider = ?")).
		WithArgs(42, "github").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := NewOAuthBindingDAO(db).DeleteByUserProviderContext(context.Background(), 42, "github")
	if err != nil {
		t.Fatalf("DeleteByUserProviderContext() error = %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows affected = %d, want 1", rows)
	}
}

func TestOAuthBindingDAOUsesInjectedDB(t *testing.T) {
	db, mock := newAuthDAOTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `oauth_bindings` WHERE provider = ? AND provider_user_id = ? ORDER BY `oauth_bindings`.`id` LIMIT ?")).
		WithArgs("github", "123", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "provider", "provider_user_id"}).
			AddRow(7, 42, "github", "123"))

	binding, err := NewOAuthBindingDAO(db).GetByProviderUserContext(context.Background(), "github", "123")
	if err != nil {
		t.Fatalf("GetByProviderUserContext() error = %v", err)
	}
	if binding.UserID != 42 {
		t.Fatalf("binding user id = %d, want 42", binding.UserID)
	}
}
