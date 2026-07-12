package system

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestPermissionRequestsExposeDescription(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		fieldKind reflect.Kind
	}{
		{name: "create", value: CreatePermissionRequest{}, fieldKind: reflect.String},
		{name: "update", value: UpdatePermissionRequest{}, fieldKind: reflect.Pointer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, ok := reflect.TypeOf(tt.value).FieldByName("Description")
			if !ok {
				t.Fatal("request should expose a Description field")
			}
			if field.Type.Kind() != tt.fieldKind {
				t.Fatalf("Description kind = %s, want %s", field.Type.Kind(), tt.fieldKind)
			}
			if got := field.Tag.Get("json"); !strings.HasPrefix(got, "description") {
				t.Fatalf("Description json tag = %q, want description", got)
			}
		})
	}
}

func TestPermissionServiceCreatePermissionContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewPermissionServiceWithDB(db)
	_, err := (&svc).CreatePermissionContext(ctx, CreatePermissionRequest{
		Name: "List Users",
		Code: "system:user:list",
		Type: 2,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreatePermissionContext() error = %v, want context.Canceled", err)
	}
}

func TestPermissionServiceCreatePermissionContextReturnsCodeLookupError(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	lookupErr := errors.New("database lookup failed")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `permissions` WHERE code = ? ORDER BY `permissions`.`id` LIMIT ?")).
		WithArgs("system:user:list", 1).
		WillReturnError(lookupErr)

	svc := NewPermissionServiceWithDB(db)
	_, err := (&svc).CreatePermissionContext(context.Background(), CreatePermissionRequest{
		Name: "List Users",
		Code: "system:user:list",
		Type: 2,
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreatePermissionContext() error = %v, want code lookup error", err)
	}
}
