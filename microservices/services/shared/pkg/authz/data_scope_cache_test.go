package authz

import (
	"context"
	"errors"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	sharedauthdao "github.com/go-admin-kit/services/shared/pkg/authdao"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestResolveDepartmentTreeIDsUsesCachedDepartmentTree(t *testing.T) {
	setupAuthzCacheTestRedis(t)
	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM \"departments\"").
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

func TestResolveDepartmentTreeIDsUsesLocalCacheWhenRedisBecomesUnavailable(t *testing.T) {
	setupAuthzCacheTestRedis(t)
	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM \"departments\"").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10).
			AddRow(12, 11))

	first := resolveDepartmentTreeIDs(10)

	oldCache := currentRemoteCache()
	SetRemoteCache(nil)
	t.Cleanup(func() {
		SetRemoteCache(oldCache)
	})

	second := resolveDepartmentTreeIDs(10)

	if !slices.Equal(first, []uint{10, 11, 12}) {
		t.Fatalf("first department ids = %#v, want [10 11 12]", first)
	}
	if !slices.Equal(second, []uint{10, 11, 12}) {
		t.Fatalf("second department ids = %#v, want [10 11 12]", second)
	}
}

func TestInvalidateDepartmentTreeCacheRemovesCachedTree(t *testing.T) {
	_, redisClient := setupAuthzCacheTestRedis(t)

	ctx := context.Background()
	if err := redisClient.Set(ctx, departmentTreeCacheKey(0), "[]", 0).Err(); err != nil {
		t.Fatalf("seed department tree cache: %v", err)
	}

	if err := InvalidateDepartmentTreeCacheContext(context.Background()); err != nil {
		t.Fatalf("invalidate department tree cache: %v", err)
	}

	if redisClient.Exists(ctx, departmentTreeCacheKey(0)).Val() != 0 {
		t.Fatal("department tree cache should be removed")
	}
}

// 保护部门树缓存的多租户隔离性。缓存行来自按租户过滤的查询，
// 因此跨租户共用一个 key 会让先填充缓存的租户决定其他每个租户的部门范围，
// 从而静默地扩大或缩小数据权限。
func TestDepartmentTreeCacheIsolatesTenants(t *testing.T) {
	setupAuthzCacheTestRedis(t)

	cache := &layeredDepartmentTreeCache{localTTL: time.Minute}
	ctxA := tenant.WithContext(context.Background(), 1)
	ctxB := tenant.WithContext(context.Background(), 2)

	if err := cache.SetDepartmentTree(ctxA, []model.Department{{ID: 10}, {ID: 11, ParentID: 10}}); err != nil {
		t.Fatalf("SetDepartmentTree(tenant 1): %v", err)
	}

	if _, ok := cache.GetDepartmentTree(ctxB); ok {
		t.Fatal("tenant 2 must not read tenant 1's cached department tree")
	}

	if err := cache.SetDepartmentTree(ctxB, []model.Department{{ID: 90}}); err != nil {
		t.Fatalf("SetDepartmentTree(tenant 2): %v", err)
	}

	got, ok := cache.GetDepartmentTree(ctxA)
	if !ok {
		t.Fatal("tenant 1 lost its cached tree after tenant 2 wrote")
	}
	if len(got) != 2 || got[0].ID != 10 || got[1].ID != 11 {
		t.Fatalf("tenant 1 tree = %#v, want the two rows it stored", got)
	}

	// 使一个租户的缓存失效时，不得逐出另一个租户的条目。
	if err := cache.InvalidateDepartmentTree(ctxA); err != nil {
		t.Fatalf("InvalidateDepartmentTree(tenant 1): %v", err)
	}
	if _, ok := cache.GetDepartmentTree(ctxA); ok {
		t.Fatal("tenant 1 cache should be gone after invalidation")
	}
	if _, ok := cache.GetDepartmentTree(ctxB); !ok {
		t.Fatal("tenant 2 cache must survive tenant 1's invalidation")
	}
}

func TestInvalidateDepartmentTreeCacheContextClearsLocalCacheWhenContextCanceled(t *testing.T) {
	setupAuthzCacheTestRedis(t)
	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM \"departments\"").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10))

	got := resolveDepartmentTreeIDs(10)
	if !slices.Equal(got, []uint{10, 11}) {
		t.Fatalf("department ids = %#v, want [10 11]", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := InvalidateDepartmentTreeCacheContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("InvalidateDepartmentTreeCacheContext() error = %v, want context.Canceled", err)
	}

	oldCache := currentRemoteCache()
	SetRemoteCache(nil)
	t.Cleanup(func() {
		SetRemoteCache(oldCache)
	})

	mock.ExpectQuery("SELECT .* FROM \"departments\"").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10))

	refreshed := resolveDepartmentTreeIDs(10)
	if !slices.Equal(refreshed, []uint{10, 11}) {
		t.Fatalf("refreshed department ids = %#v, want [10 11]", refreshed)
	}
}

func TestInvalidateDepartmentTreeCacheContextClearsLocalCacheWhenDeleteFails(t *testing.T) {
	_, redisClient := setupAuthzCacheTestRedis(t)

	cache, ok := NewDataScopeResolver(nil).departmentTreeCache().(*layeredDepartmentTreeCache)
	if !ok {
		t.Fatal("expected layeredDepartmentTreeCache")
	}
	cache.setLocalRows(0, []departmentTreeCacheRow{{ID: 10}, {ID: 11, ParentID: 10}})

	ctx := context.Background()
	if err := redisClient.Set(ctx, departmentTreeCacheKey(0), "[]", time.Hour).Err(); err != nil {
		t.Fatalf("seed department tree cache: %v", err)
	}

	injectedErr := errors.New("delete failed")
	redisClient.AddHook(redisCommandErrorHook{command: "del", err: injectedErr})

	err := InvalidateDepartmentTreeCacheContext(ctx)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("InvalidateDepartmentTreeCacheContext() error = %v, want injected error", err)
	}
	if _, cached := cache.getLocalRows(0); cached {
		t.Fatal("local department tree cache should be cleared on delete failure")
	}
	if redisClient.Exists(ctx, departmentTreeCacheKey(0)).Val() != 1 {
		t.Fatal("remote department tree cache should remain when delete fails")
	}
}

func TestInvalidateDepartmentTreeCacheContextClearsLocalCacheWhenPublishFails(t *testing.T) {
	_, redisClient := setupAuthzCacheTestRedis(t)

	cache, ok := NewDataScopeResolver(nil).departmentTreeCache().(*layeredDepartmentTreeCache)
	if !ok {
		t.Fatal("expected layeredDepartmentTreeCache")
	}
	cache.setLocalRows(0, []departmentTreeCacheRow{{ID: 10}, {ID: 11, ParentID: 10}})

	ctx := context.Background()
	if err := redisClient.Set(ctx, departmentTreeCacheKey(0), "[]", time.Hour).Err(); err != nil {
		t.Fatalf("seed department tree cache: %v", err)
	}

	injectedErr := errors.New("publish failed")
	redisClient.AddHook(redisCommandErrorHook{command: "publish", err: injectedErr})

	err := InvalidateDepartmentTreeCacheContext(ctx)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("InvalidateDepartmentTreeCacheContext() error = %v, want injected error", err)
	}
	if _, cached := cache.getLocalRows(0); cached {
		t.Fatal("local department tree cache should be cleared on publish failure")
	}
	if redisClient.Exists(ctx, departmentTreeCacheKey(0)).Val() != 0 {
		t.Fatal("remote department tree cache should be removed before publish failure")
	}
}

func TestDepartmentTreeInvalidationListenerClearsLocalCache(t *testing.T) {
	_, redisClient := setupAuthzCacheTestRedis(t)
	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM \"departments\"").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10))

	initial := resolveDepartmentTreeIDs(10)
	if !slices.Equal(initial, []uint{10, 11}) {
		t.Fatalf("initial department ids = %#v, want [10 11]", initial)
	}

	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := StartDepartmentTreeInvalidationListener(listenerCtx)
	if err != nil {
		t.Fatalf("StartDepartmentTreeInvalidationListener() error = %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Fatalf("listener.Close() error = %v", err)
		}
	}()

	if err := redisClient.Publish(context.Background(), departmentTreeInvalidateChannel, departmentTreeInvalidatePayloadClear).Err(); err != nil {
		t.Fatalf("publish invalidation message: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cache, ok := NewDataScopeResolver(nil).departmentTreeCache().(*layeredDepartmentTreeCache)
		if ok {
			if _, cached := cache.getLocalRows(0); !cached {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	oldCache := currentRemoteCache()
	SetRemoteCache(nil)
	t.Cleanup(func() {
		SetRemoteCache(oldCache)
	})

	mock.ExpectQuery("SELECT .* FROM \"departments\"").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10))

	refreshed := resolveDepartmentTreeIDs(10)
	if !slices.Equal(refreshed, []uint{10, 11}) {
		t.Fatalf("refreshed department ids = %#v, want [10 11]", refreshed)
	}
}

func TestResolveDepartmentTreeIDsFallsBackWhenRedisIsUnavailable(t *testing.T) {
	oldCache := currentRemoteCache()
	SetRemoteCache(nil)
	t.Cleanup(func() {
		SetRemoteCache(oldCache)
	})

	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM \"departments\"").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10).
			AddRow(12, 11))

	got := resolveDepartmentTreeIDs(10)
	if !slices.Equal(got, []uint{10, 11, 12}) {
		t.Fatalf("department ids = %#v, want [10 11 12]", got)
	}
}

func TestResolveUserDataScopeContextPropagatesDepartmentTreeCancellation(t *testing.T) {
	oldCache := currentRemoteCache()
	SetRemoteCache(nil)
	t.Cleanup(func() {
		SetRemoteCache(oldCache)
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
	oldDB := currentDefaultDB()
	SetDefaultDB(nil)
	t.Cleanup(func() {
		SetDefaultDB(oldDB)
	})

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
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

	mock.ExpectQuery("SELECT \\* FROM \"files\" WHERE user_id IN \\(SELECT id FROM users WHERE department_id IN \\(\\$\\d+,\\$\\d+\\)\\)").
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

func setupAuthzCacheTestRedis(t *testing.T) (*miniredis.Miniredis, *goredis.Client) {
	t.Helper()

	resetDefaultDepartmentTreeCache()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	oldCache := currentRemoteCache()
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	SetRemoteCache(NewGoRedisRemoteCache(client))

	t.Cleanup(func() {
		resetDefaultDepartmentTreeCache()
		_ = client.Close()
		SetRemoteCache(oldCache)
		store.Close()
	})

	return store, client
}

func setupAuthzCacheTestDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()

	resetDefaultDepartmentTreeCache()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	oldDB := currentDefaultDB()
	SetDefaultDB(db)
	restorePersistence := SetPersistence(Persistence{
		Users:       stubUserWithRolesStore{db: db},
		Permissions: sharedauthdao.NewPermissionDAO(db),
		DataScope:   NewDatabaseDataScopeStore(db),
	})
	t.Cleanup(func() {
		restorePersistence()
		resetDefaultDepartmentTreeCache()
		SetDefaultDB(oldDB)
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	return mock
}

type redisCommandErrorHook struct {
	command string
	err     error
}

func (h redisCommandErrorHook) DialHook(next goredis.DialHook) goredis.DialHook {
	return next
}

func (h redisCommandErrorHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if strings.EqualFold(cmd.Name(), h.command) {
			return h.err
		}
		return next(ctx, cmd)
	}
}

func (h redisCommandErrorHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		for _, cmd := range cmds {
			if strings.EqualFold(cmd.Name(), h.command) {
				return h.err
			}
		}
		return next(ctx, cmds)
	}
}

// 锁定 admin 快速路径。resolveRoleDataScope 仅凭角色编码就向 super_admin/admin
// 授予 DataScopeAll，因此一旦 PermissionMiddleware 缓存了这些编码，
// 再进行 user+roles 往返查询就是纯粹的浪费。sqlmock 未注册任何期望，
// 因此这里发出的任何查询都会使测试失败。
func TestResolveUserDataScopeFromContextSkipsQueriesForMemoizedAdmins(t *testing.T) {
	for _, code := range []string{"super_admin", "admin"} {
		t.Run(code, func(t *testing.T) {
			mock := setupAuthzCacheTestDB(t)

			ginCtx, _ := gin.CreateTestContext(nil)
			ginCtx.Request = httptest.NewRequest("GET", "/", nil)
			ginCtx.Set("user_id", uint(7))
			ginCtx.Set(RoleCodesContextKey, []string{"viewer", code})

			scope, err := ResolveUserDataScopeFromContext(ginCtx)
			if err != nil {
				t.Fatalf("ResolveUserDataScopeFromContext() error = %v", err)
			}
			if scope.Scope != DataScopeAll {
				t.Fatalf("scope = %q, want %q", scope.Scope, DataScopeAll)
			}
			if scope.UserID != 7 {
				t.Fatalf("scope.UserID = %d, want 7", scope.UserID)
			}
			if !scope.CanAccessAll() {
				t.Fatal("CanAccessAll() = false, want true")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected database access on the admin fast path: %v", err)
			}
		})
	}
}

// 非 admin 编码不得走快速路径——它们仍需要数据库来解析部门范围，
// 因此请求上下文必须能到达存储层。
func TestResolveUserDataScopeFromContextStillQueriesForNonAdmins(t *testing.T) {
	setupAuthzCacheTestDB(t)

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	ginCtx, _ := gin.CreateTestContext(nil)
	ginCtx.Request = httptest.NewRequestWithContext(requestCtx, "GET", "/", nil)
	ginCtx.Set("user_id", uint(7))
	ginCtx.Set(RoleCodesContextKey, []string{"viewer", "dept_admin"})

	if _, err := ResolveUserDataScopeFromContext(ginCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled (proving the store was reached)", err)
	}
}
