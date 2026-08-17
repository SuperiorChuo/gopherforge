package system

import (
	"context"
	"errors"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Tests drive the service with context.Background(), i.e. tenant id 0, so the
// sharded key resolves to the ":0" suffix.
const departmentTreeCacheTestKey = "authz:department_tree:0"

func TestDepartmentServiceCreateInvalidatesDepartmentTreeCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)
	seedDepartmentTreeCache(t)

	dao := &fakeDepartmentDAO{getByCodeErr: gorm.ErrRecordNotFound}
	service := DepartmentService{deptDAO: dao}

	_, err := service.CreateContext(context.Background(), CreateDepartmentRequest{
		Name:   "Engineering",
		Code:   "rd",
		Status: 1,
	})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	assertDepartmentTreeCacheRemoved(t)
}

func TestDepartmentServiceCreateContextHonorsCanceledContext(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewDepartmentServiceWithDB(db)
	_, err := (&svc).CreateContext(ctx, CreateDepartmentRequest{
		Name: "Engineering",
		Code: "eng",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateContext() error = %v, want context.Canceled", err)
	}
}

func TestDepartmentServiceCreateContextReturnsCodeLookupError(t *testing.T) {
	lookupErr := errors.New("database lookup failed")
	service := DepartmentService{deptDAO: &fakeDepartmentDAO{getByCodeErr: lookupErr}}

	_, err := service.CreateContext(context.Background(), CreateDepartmentRequest{
		Name: "Engineering",
		Code: "eng",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateContext() error = %v, want code lookup error", err)
	}
}

func TestDepartmentServiceCreateContextStillInvalidatesRemoteCacheAfterRequestCancellation(t *testing.T) {
	setupDepartmentServiceTestRedis(t)
	seedDepartmentTreeCache(t)

	ctx, cancel := context.WithCancel(context.Background())
	dao := &fakeDepartmentDAO{
		getByCodeErr: gorm.ErrRecordNotFound,
		createHook: func(context.Context) {
			cancel()
		},
	}
	service := DepartmentService{deptDAO: dao}

	dept, err := service.CreateContext(ctx, CreateDepartmentRequest{
		Name:   "Engineering",
		Code:   "rd",
		Status: 1,
	})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if dept == nil || dept.ID == 0 {
		t.Fatalf("CreateContext() department = %#v, want persisted department", dept)
	}

	exists, err := redisstore.Client.Exists(context.Background(), departmentTreeCacheTestKey).Result()
	if err != nil {
		t.Fatalf("check department tree cache: %v", err)
	}
	if exists != 0 {
		t.Fatalf("department tree cache existence = %d, want 0 after best-effort invalidation", exists)
	}
}

func TestDepartmentServiceUpdateInvalidatesDepartmentTreeCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)
	seedDepartmentTreeCache(t)

	dept := &localmodel.Department{ID: 10, Name: "Engineering", Code: "rd", Status: 1}
	dao := &fakeDepartmentDAO{byID: map[uint]*localmodel.Department{10: dept}}
	service := DepartmentService{deptDAO: dao}

	_, err := service.UpdateContext(context.Background(), 10, UpdateDepartmentRequest{Name: "Product Engineering"})
	if err != nil {
		t.Fatalf("UpdateContext() error = %v", err)
	}
	assertDepartmentTreeCacheRemoved(t)
}

func TestDepartmentServiceDeleteInvalidatesDepartmentTreeCache(t *testing.T) {
	setupDepartmentServiceTestRedis(t)
	seedDepartmentTreeCache(t)

	dao := &fakeDepartmentDAO{}
	service := DepartmentService{deptDAO: dao}

	if err := service.DeleteContext(context.Background(), 10); err != nil {
		t.Fatalf("DeleteContext() error = %v", err)
	}
	assertDepartmentTreeCacheRemoved(t)
}

func seedDepartmentTreeCache(t *testing.T) {
	t.Helper()

	if err := redisstore.Client.Set(context.Background(), departmentTreeCacheTestKey, "[]", time.Hour).Err(); err != nil {
		t.Fatalf("seed department tree cache: %v", err)
	}
}

func assertDepartmentTreeCacheRemoved(t *testing.T) {
	t.Helper()

	exists, err := redisstore.Client.Exists(context.Background(), departmentTreeCacheTestKey).Result()
	if err != nil {
		t.Fatalf("check department tree cache: %v", err)
	}
	if exists != 0 {
		t.Fatal("department tree cache should be removed")
	}
}

func setupDepartmentServiceTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	oldClient := redisstore.Client
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client
	authz.SetRemoteCache(authz.NewGoRedisRemoteCache(client))

	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = oldClient
		authz.SetRemoteCache(nil)
		store.Close()
	})

	return store
}

type fakeDepartmentDAO struct {
	byID         map[uint]*localmodel.Department
	getByCodeErr error
	createErr    error
	updateErr    error
	deleteErr    error
	createHook   func(context.Context)
}

func (d *fakeDepartmentDAO) GetByIDContext(_ context.Context, id uint) (*localmodel.Department, error) {
	if dept, ok := d.byID[id]; ok {
		cp := *dept
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (d *fakeDepartmentDAO) GetByCodeContext(_ context.Context, _ string) (*localmodel.Department, error) {
	if d.getByCodeErr != nil {
		return nil, d.getByCodeErr
	}
	return &localmodel.Department{}, nil
}

func (d *fakeDepartmentDAO) GetListContext(context.Context, pagination.PageRequest, string, *int8) ([]localmodel.Department, int64, error) {
	return nil, 0, nil
}

func (d *fakeDepartmentDAO) GetAllContext(context.Context, *int8) ([]localmodel.Department, error) {
	return nil, nil
}

func (d *fakeDepartmentDAO) GetTreeContext(context.Context, *int8) ([]localmodel.Department, error) {
	return nil, nil
}

func (d *fakeDepartmentDAO) CreateContext(ctx context.Context, dept *localmodel.Department) error {
	if d.createErr != nil {
		return d.createErr
	}
	dept.ID = 1
	if d.createHook != nil {
		d.createHook(ctx)
	}
	return nil
}

func (d *fakeDepartmentDAO) UpdateContext(context.Context, *localmodel.Department) error {
	return d.updateErr
}

func (d *fakeDepartmentDAO) DeleteContext(context.Context, uint) error {
	return d.deleteErr
}

func (d *fakeDepartmentDAO) GetChildrenIDsContext(context.Context, uint) ([]uint, error) {
	return nil, nil
}
