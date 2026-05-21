# Department Tree L1+L2 Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `authz` 的部门树数据权限解析增加进程内 L1 缓存、Redis Pub/Sub 失效广播和 context-aware 失效入口，在不改变现有业务接口成功语义的前提下提升热点查询性能。

**Architecture:** 保持 `authz.DepartmentTreeCache` 接口不变，将默认实现从纯 Redis 替换为共享的 `L1 + L2` 组合缓存；在 `redis` 包增加最小的 publish/subscribe helper；在 `main.go` 显式启动失效监听；部门写接口继续 best-effort 失效缓存，不因广播失败回滚数据库写入。

**Tech Stack:** Go 1.26、Gin、GORM、go-redis/v9、miniredis、sqlmock

---

### Task 1: Write Failing Cache Behavior Tests

**Files:**
- Modify: `server/internal/pkg/authz/data_scope_cache_test.go`
- Modify: `server/internal/service/system/department_test.go`
- Test: `server/internal/pkg/authz/data_scope_cache_test.go`
- Test: `server/internal/service/system/department_test.go`

- [ ] **Step 1: Add failing authz cache tests**

```go
func TestResolveDepartmentTreeIDsUsesLocalCacheWhenRedisBecomesUnavailable(t *testing.T) {
	setupAuthzCacheTestRedis(t)
	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10).
			AddRow(12, 11))

	first := resolveDepartmentTreeIDs(10)

	oldClient := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldClient
	})

	second := resolveDepartmentTreeIDs(10)

	if !slices.Equal(first, []uint{10, 11, 12}) {
		t.Fatalf("first department ids = %#v, want [10 11 12]", first)
	}
	if !slices.Equal(second, []uint{10, 11, 12}) {
		t.Fatalf("second department ids = %#v, want [10 11 12]", second)
	}
}

func TestInvalidateDepartmentTreeCacheContextClearsLocalCacheWhenContextCanceled(t *testing.T) {
	setupAuthzCacheTestRedis(t)
	mock := setupAuthzCacheTestDB(t)
	mock.ExpectQuery("SELECT .* FROM `departments`").
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

	oldClient := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldClient
	})

	mock.ExpectQuery("SELECT .* FROM `departments`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id"}).
			AddRow(11, 10))

	refreshed := resolveDepartmentTreeIDs(10)
	if !slices.Equal(refreshed, []uint{10, 11}) {
		t.Fatalf("refreshed department ids = %#v, want [10 11]", refreshed)
	}
}
```

- [ ] **Step 2: Add failing department service contract test**

```go
func TestDepartmentServiceCreateContextIgnoresDepartmentTreeInvalidationFailure(t *testing.T) {
	setupDepartmentServiceTestRedis(t)
	seedDepartmentTreeCache(t)

	oldClient := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldClient
	})

	dao := &fakeDepartmentDAO{getByCodeErr: gorm.ErrRecordNotFound}
	service := DepartmentService{deptDAO: dao}

	dept, err := service.CreateContext(context.Background(), CreateDepartmentRequest{
		Name:   "Engineering",
		Code:   "rd",
		Status: 1,
	})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if dept == nil || dept.ID == 0 {
		t.Fatalf("CreateContext() returned invalid department: %#v", dept)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```powershell
cd server
go test ./internal/pkg/authz -run "TestResolveDepartmentTreeIDsUsesLocalCacheWhenRedisBecomesUnavailable|TestInvalidateDepartmentTreeCacheContextClearsLocalCacheWhenContextCanceled" -count=1
go test ./internal/service/system -run TestDepartmentServiceCreateContextIgnoresDepartmentTreeInvalidationFailure -count=1
```

Expected:

- `authz` tests fail because local L1 cache and `InvalidateDepartmentTreeCacheContext` do not exist yet.
- `department service` test fails because the context-aware invalidation path is not wired yet.

- [ ] **Step 4: Commit the failing tests**

```bash
git add server/internal/pkg/authz/data_scope_cache_test.go server/internal/service/system/department_test.go
git commit -m "test: add failing department tree cache tests"
```

### Task 2: Add Redis Pub/Sub Helper With Lifecycle Tests

**Files:**
- Modify: `server/internal/pkg/redis/redis.go`
- Create: `server/internal/pkg/redis/pubsub_test.go`
- Test: `server/internal/pkg/redis/pubsub_test.go`

- [ ] **Step 1: Write failing redis helper tests**

```go
func TestPublishStringDeliversMessageToSubscriber(t *testing.T) {
	store := miniredis.RunT(t)
	oldClient := Client
	Client = redis.NewClient(&redis.Options{Addr: store.Addr()})
	t.Cleanup(func() {
		_ = Client.Close()
		Client = oldClient
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	delivered := make(chan string, 1)
	sub, err := StartSubscriber(ctx, "authz:test", func(_ context.Context, payload string) {
		delivered <- payload
	})
	if err != nil {
		t.Fatalf("StartSubscriber() error = %v", err)
	}
	defer func() { _ = sub.Close() }()

	if err := PublishString(context.Background(), "authz:test", "clear"); err != nil {
		t.Fatalf("PublishString() error = %v", err)
	}

	select {
	case got := <-delivered:
		if got != "clear" {
			t.Fatalf("payload = %q, want %q", got, "clear")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pubsub delivery")
	}
}

func TestStartSubscriberReturnsErrorWhenRedisClientMissing(t *testing.T) {
	oldClient := Client
	Client = nil
	t.Cleanup(func() {
		Client = oldClient
	})

	_, err := StartSubscriber(context.Background(), "authz:test", func(context.Context, string) {})
	if err == nil {
		t.Fatal("StartSubscriber() error = nil, want non-nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
cd server
go test ./internal/pkg/redis -run "TestPublishStringDeliversMessageToSubscriber|TestStartSubscriberReturnsErrorWhenRedisClientMissing" -count=1
```

Expected:

- Tests fail because `PublishString`, `StartSubscriber`, subscriber lifecycle, and package `Close` helper do not exist yet.

- [ ] **Step 3: Write minimal redis helper implementation**

```go
type StringSubscriber struct {
	pubsub *redis.PubSub
	cancel context.CancelFunc
	done   chan struct{}
}

func PublishString(ctx context.Context, channel, payload string) error {
	if Client == nil {
		return fmt.Errorf("redis client is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return Client.Publish(ctx, channel, payload).Err()
}

func StartSubscriber(ctx context.Context, channel string, handler func(context.Context, string)) (*StringSubscriber, error) {
	if Client == nil {
		return nil, fmt.Errorf("redis client is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	pubsub := Client.Subscribe(ctx, channel)
	if _, err := pubsub.Receive(ctx); err != nil {
		cancel()
		_ = pubsub.Close()
		return nil, err
	}

	sub := &StringSubscriber{
		pubsub: pubsub,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(sub.done)
		defer pubsub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-pubsub.Channel():
				if !ok {
					return
				}
				handler(ctx, msg.Payload)
			}
		}
	}()

	return sub, nil
}

func (s *StringSubscriber) Close() error {
	if s == nil {
		return nil
	}
	s.cancel()
	<-s.done
	return nil
}

func Close() error {
	if Client == nil {
		return nil
	}
	client := Client
	Client = nil
	return client.Close()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```powershell
cd server
go test ./internal/pkg/redis -run "TestPublishStringDeliversMessageToSubscriber|TestStartSubscriberReturnsErrorWhenRedisClientMissing" -count=1
```

Expected:

- Both tests pass.

- [ ] **Step 5: Commit**

```bash
git add server/internal/pkg/redis/redis.go server/internal/pkg/redis/pubsub_test.go
git commit -m "feat: add redis pubsub helper for cache invalidation"
```

### Task 3: Implement L1+L2 Department Tree Cache

**Files:**
- Modify: `server/internal/pkg/authz/data_scope.go`
- Modify: `server/internal/pkg/authz/data_scope_cache_test.go`
- Test: `server/internal/pkg/authz/data_scope_cache_test.go`
- Test: `server/internal/pkg/authz/data_scope_store_test.go`

- [ ] **Step 1: Implement the shared layered cache**

```go
const (
	departmentTreeCacheKey               = "authz:department_tree"
	departmentTreeCacheTTL               = 5 * time.Minute
	departmentTreeLocalCacheTTL          = 30 * time.Second
	departmentTreeInvalidateChannel      = "authz:department_tree:invalidate"
	departmentTreeInvalidatePayloadClear = "clear"
)

type layeredDepartmentTreeCache struct {
	mu        sync.RWMutex
	localRows []departmentTreeCacheRow
	localTTL  time.Duration
	expiresAt time.Time
}

var defaultDepartmentTreeCache = &layeredDepartmentTreeCache{
	localTTL: departmentTreeLocalCacheTTL,
}
```

- [ ] **Step 2: Add local cache read/write helpers and context-aware invalidation**

```go
func InvalidateDepartmentTreeCacheContext(ctx context.Context) error {
	return defaultDepartmentTreeCache.InvalidateDepartmentTree(ctx)
}

func InvalidateDepartmentTreeCache() error {
	return InvalidateDepartmentTreeCacheContext(context.Background())
}

func (c *layeredDepartmentTreeCache) clearLocal() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localRows = nil
	c.expiresAt = time.Time{}
}
```

- [ ] **Step 3: Wire Redis L2 + publish semantics into `InvalidateDepartmentTree`**

```go
func (c *layeredDepartmentTreeCache) InvalidateDepartmentTree(ctx context.Context) error {
	c.clearLocal()
	if redisstore.Client == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := redisstore.Client.Del(ctx, departmentTreeCacheKey).Err(); err != nil {
		return err
	}
	return redisstore.PublishString(ctx, departmentTreeInvalidateChannel, departmentTreeInvalidatePayloadClear)
}
```

- [ ] **Step 4: Add listener entrypoint**

```go
func StartDepartmentTreeInvalidationListener(ctx context.Context) (*redisstore.StringSubscriber, error) {
	return redisstore.StartSubscriber(ctx, departmentTreeInvalidateChannel, func(_ context.Context, payload string) {
		if payload == departmentTreeInvalidatePayloadClear {
			defaultDepartmentTreeCache.clearLocal()
		}
	})
}
```

- [ ] **Step 5: Run tests**

Run:

```powershell
cd server
go test ./internal/pkg/authz -run "TestResolveDepartmentTreeIDsUsesCachedDepartmentTree|TestResolveDepartmentTreeIDsUsesLocalCacheWhenRedisBecomesUnavailable|TestInvalidateDepartmentTreeCacheRemovesCachedTree|TestInvalidateDepartmentTreeCacheContextClearsLocalCacheWhenContextCanceled" -count=1
```

Expected:

- All targeted authz cache tests pass.

- [ ] **Step 6: Commit**

```bash
git add server/internal/pkg/authz/data_scope.go server/internal/pkg/authz/data_scope_cache_test.go
git commit -m "feat: add layered department tree cache"
```

### Task 4: Wire Service And Main Startup

**Files:**
- Modify: `server/internal/service/system/department.go`
- Modify: `server/internal/service/system/department_test.go`
- Modify: `server/cmd/main.go`
- Test: `server/internal/service/system/department_test.go`
- Test: `server/cmd/main_test.go`

- [ ] **Step 1: Update department service to use context-aware invalidation without changing success contract**

```go
if err := authz.InvalidateDepartmentTreeCacheContext(ctx); err != nil {
	logger.Error("department tree cache invalidation failed", logger.Err(err))
}
```

- [ ] **Step 2: Add startup wiring in `main.go`**

```go
if err := redis.InitRedis(); err != nil {
	logger.Fatal("redis initialization failed", logger.Err(err))
}
defer func() {
	if err := redis.Close(); err != nil {
		logger.Error("redis close failed", logger.Err(err))
	}
}()

cacheListenerCtx, cancelCacheListener := context.WithCancel(context.Background())
defer cancelCacheListener()

cacheListener, err := authz.StartDepartmentTreeInvalidationListener(cacheListenerCtx)
if err != nil {
	logger.Warn("department tree cache invalidation listener disabled", logger.Err(err))
} else {
	defer func() {
		if err := cacheListener.Close(); err != nil {
			logger.Error("department tree cache listener close failed", logger.Err(err))
		}
	}()
}
```

- [ ] **Step 3: Run focused tests**

Run:

```powershell
cd server
go test ./internal/service/system -run "TestDepartmentServiceCreateInvalidatesDepartmentTreeCache|TestDepartmentServiceCreateContextIgnoresDepartmentTreeInvalidationFailure|TestDepartmentServiceUpdateInvalidatesDepartmentTreeCache|TestDepartmentServiceDeleteInvalidatesDepartmentTreeCache" -count=1
go test ./internal/pkg/redis ./internal/pkg/authz ./internal/service/system ./cmd -count=1
```

Expected:

- Department service tests pass.
- Package-level regression tests for redis/authz/system/cmd pass.

- [ ] **Step 4: Run broader verification**

Run:

```powershell
cd server
go test ./...
go vet ./...
```

Expected:

- Full backend test suite passes.
- `go vet` passes.

- [ ] **Step 5: Commit**

```bash
git add server/internal/service/system/department.go server/internal/service/system/department_test.go server/cmd/main.go
git commit -m "feat: wire department tree invalidation lifecycle"
```
