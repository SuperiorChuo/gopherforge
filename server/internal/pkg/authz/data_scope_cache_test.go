package authz

import (
	"context"
	"errors"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestResolveDepartmentTreeIDsUsesCachedDepartmentTree(t *testing.T) {
	setupAuthzCacheTestRedis(t)
	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10).
			AddRow(12, 11))

	first := resolveDepartmentTreeIDs(10)
	second := resolveDepartmentTreeIDs(10)

	if !slices.Equal(first, []uint{10, 11, 12}) {
		t.Fatalf("first department ids = %#v, want [10 11 12]", first)
	}
	if !slices.Equal(second, []uint{10, 11, 12}) {
		t.Fatalf("second department ids = %#v, want [10 11 12]", second)
	}
}

func TestInvalidateDepartmentTreeCacheRemovesCachedTree(t *testing.T) {
	setupAuthzCacheTestRedis(t)

	ctx := context.Background()
	if err := redisstore.Client.Set(ctx, departmentTreeCacheKey, "[]", 0).Err(); err != nil {
		t.Fatalf("seed department tree cache: %v", err)
	}

	if err := InvalidateDepartmentTreeCache(); err != nil {
		t.Fatalf("invalidate department tree cache: %v", err)
	}

	if redisstore.Client.Exists(ctx, departmentTreeCacheKey).Val() != 0 {
		t.Fatal("department tree cache should be removed")
	}
}

func TestResolveDepartmentTreeIDsFallsBackWhenRedisIsUnavailable(t *testing.T) {
	oldClient := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldClient
	})

	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10).
			AddRow(12, 11))

	got := resolveDepartmentTreeIDs(10)
	if !slices.Equal(got, []uint{10, 11, 12}) {
		t.Fatalf("department ids = %#v, want [10 11 12]", got)
	}
}

func TestResolveUserDataScopeContextPropagatesDepartmentTreeCancellation(t *testing.T) {
	oldClient := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldClient
	})
	setupAuthzCacheTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ResolveUserDataScopeContext(ctx, &model.User{
		ID:           7,
		DepartmentID: 10,
		Roles: []model.Role{{
			ID:        3,
			Code:      "dept_admin",
			DataScope: string(DataScopeDepartmentTree),
		}},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveUserDataScopeContext() error = %v, want context.Canceled", err)
	}
}

func TestApplyOwnerScopeUsesCurrentQueryDBForDepartmentSubquery(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	mock.ExpectQuery("SELECT \\* FROM `files` WHERE user_id IN \\(SELECT `id` FROM `users` WHERE department_id IN \\(\\?,\\?\\)\\)").
		WithArgs(uint(10), uint(11)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	var files []model.File
	err = ApplyOwnerScope(
		db.Model(&model.File{}),
		UserDataScope{Scope: DataScopeDepartment, DepartmentIDs: []uint{10, 11}},
		"user_id",
	).Find(&files).Error
	if err != nil {
		t.Fatalf("ApplyOwnerScope query error = %v", err)
	}
}

func TestResolveUserDataScopeFromContextPropagatesRequestCancellation(t *testing.T) {
	setupAuthzCacheTestDB(t)

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	ginCtx, _ := gin.CreateTestContext(nil)
	ginCtx.Request = httptest.NewRequestWithContext(requestCtx, "GET", "/", nil)
	ginCtx.Set("user_id", uint(7))

	_, err := ResolveUserDataScopeFromContext(ginCtx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveUserDataScopeFromContext() error = %v, want context.Canceled", err)
	}
}

func TestUserHasPermissionFromContextPropagatesRequestCancellation(t *testing.T) {
	setupAuthzCacheTestDB(t)

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	ginCtx, _ := gin.CreateTestContext(nil)
	ginCtx.Request = httptest.NewRequestWithContext(requestCtx, "GET", "/", nil)
	ginCtx.Set("user_id", uint(7))

	_, err := UserHasPermissionFromContext(ginCtx, "system:user:list")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("UserHasPermissionFromContext() error = %v, want context.Canceled", err)
	}
}

func setupAuthzCacheTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	oldClient := redisstore.Client
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client

	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = oldClient
		store.Close()
	})

	return store
}

func setupAuthzCacheTestDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()

	oldDB := database.DB
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
		database.DB = oldDB
	})

	return mock
}
