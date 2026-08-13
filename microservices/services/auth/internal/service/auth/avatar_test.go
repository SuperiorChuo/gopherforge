package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUpdateProfileAcceptsManagedAvatarURL(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(uint(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "status"}).AddRow(42, "alice", 1))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "avatar"=$1,"updated_at"=$2 WHERE id = $3`)).
		WithArgs("/api/v1/files/avatars/abcdefghijklmnopqrstuvwxyzABCDE1234567890_", sqlmock.AnyArg(), uint(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(uint(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "avatar", "status"}).AddRow(42, "alice", "/api/v1/files/avatars/abcdefghijklmnopqrstuvwxyzABCDE1234567890_", 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_roles" WHERE "user_roles"."user_id" = $1`)).
		WithArgs(uint(42)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id"}))

	avatar := "/api/v1/files/avatars/abcdefghijklmnopqrstuvwxyzABCDE1234567890_"
	service := NewUserServiceWithDB(db)
	user, err := service.UpdateProfileContext(context.Background(), 42, UpdateProfileRequest{Avatar: &avatar})
	if err != nil {
		t.Fatalf("UpdateProfileContext() error = %v", err)
	}
	if user.Avatar != avatar {
		t.Fatalf("avatar = %q, want %q", user.Avatar, avatar)
	}
}

func TestUpdateProfileRejectsUnmanagedRelativeAvatarURL(t *testing.T) {
	db, mock := setupAuthServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 ORDER BY "users"."id" LIMIT $2`)).
		WithArgs(uint(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "status"}).AddRow(42, "alice", 1))

	avatar := "/uploads/unmanaged.png"
	service := NewUserServiceWithDB(db)
	_, err := service.UpdateProfileContext(context.Background(), 42, UpdateProfileRequest{Avatar: &avatar})
	if err == nil {
		t.Fatal("UpdateProfileContext() error = nil, want validation error")
	}
	var validationErr ProfileValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("UpdateProfileContext() error = %T %v, want ProfileValidationError", err, err)
	}
}
