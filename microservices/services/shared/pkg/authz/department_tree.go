package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

func departmentIDs(departmentID uint) []uint {
	if departmentID == 0 {
		return nil
	}
	return []uint{departmentID}
}

func resolveDepartmentTreeIDs(departmentID uint) []uint {
	ids, err := resolveDepartmentTreeIDsContext(context.Background(), departmentID)
	if err != nil {
		return departmentIDs(departmentID)
	}
	return ids
}

func resolveDepartmentTreeIDsContext(ctx context.Context, departmentID uint) ([]uint, error) {
	return NewDataScopeResolver(nil).resolveDepartmentTreeIDsContext(ctx, departmentID)
}

func (r *DataScopeResolver) resolveDepartmentTreeIDsContext(ctx context.Context, departmentID uint) ([]uint, error) {
	ids := departmentIDs(departmentID)
	if departmentID == 0 {
		return ids, nil
	}

	depts, err := r.loadDepartmentTreeContext(ctx)
	if err != nil {
		return nil, err
	}

	collectChildDepartmentIDs(depts, departmentID, &ids)
	return ids, nil
}

func (r *DataScopeResolver) loadDepartmentTreeContext(ctx context.Context) ([]model.Department, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cache := r.departmentTreeCache()
	if depts, ok := cache.GetDepartmentTree(ctx); ok {
		return depts, nil
	}

	depts, err := r.dataScopeStore().ListDepartments(ctx)
	if err != nil {
		return nil, err
	}

	_ = cache.SetDepartmentTree(ctx, depts)
	return depts, nil
}

func departmentRowsToModels(rows []departmentTreeCacheRow) []model.Department {
	depts := make([]model.Department, 0, len(rows))
	for _, row := range rows {
		depts = append(depts, model.Department{
			ID:       row.ID,
			ParentID: row.ParentID,
		})
	}
	return depts
}

func departmentModelsToRows(depts []model.Department) []departmentTreeCacheRow {
	rows := make([]departmentTreeCacheRow, 0, len(depts))
	for _, dept := range depts {
		rows = append(rows, departmentTreeCacheRow{
			ID:       dept.ID,
			ParentID: dept.ParentID,
		})
	}
	return rows
}

func cloneDepartmentTreeRows(rows []departmentTreeCacheRow) []departmentTreeCacheRow {
	if len(rows) == 0 {
		if rows == nil {
			return nil
		}
		return []departmentTreeCacheRow{}
	}
	cloned := make([]departmentTreeCacheRow, len(rows))
	copy(cloned, rows)
	return cloned
}

func (c *layeredDepartmentTreeCache) GetDepartmentTree(ctx context.Context) ([]model.Department, bool) {
	tenantID := tenant.FromContext(ctx)
	if rows, ok := c.getLocalRows(tenantID); ok {
		return departmentRowsToModels(rows), true
	}

	rc := currentRemoteCache()
	if rc == nil {
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	data, err := rc.Get(ctx, departmentTreeCacheKey(tenantID))
	if err != nil {
		return nil, false
	}

	var rows []departmentTreeCacheRow
	if err := json.Unmarshal(data, &rows); err != nil {
		_ = c.deleteRemote(ctx, tenantID)
		return nil, false
	}

	c.setLocalRows(tenantID, rows)
	return departmentRowsToModels(rows), true
}

func (c *layeredDepartmentTreeCache) SetDepartmentTree(ctx context.Context, depts []model.Department) error {
	tenantID := tenant.FromContext(ctx)
	rows := departmentModelsToRows(depts)
	c.setLocalRows(tenantID, rows)

	rc := currentRemoteCache()
	if rc == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return rc.Set(ctx, departmentTreeCacheKey(tenantID), data, departmentTreeCacheTTL)
}

func InvalidateDepartmentTreeCacheContext(ctx context.Context) error {
	return defaultDepartmentTreeCache.InvalidateDepartmentTree(ctx)
}

func StartDepartmentTreeInvalidationListener(ctx context.Context) (io.Closer, error) {
	rc := currentRemoteCache()
	if rc == nil {
		return nil, fmt.Errorf("authz remote cache not configured")
	}
	return rc.StartSubscriber(ctx, departmentTreeInvalidateChannel, func(_ context.Context, payload string) {
		// "clear" is still honoured so that a peer running the pre-sharding build
		// can invalidate this process during a rolling deploy.
		if payload == departmentTreeInvalidatePayloadClear {
			defaultDepartmentTreeCache.clearAllLocal()
			return
		}
		tenantID, err := strconv.ParseUint(payload, 10, 64)
		if err != nil {
			return
		}
		defaultDepartmentTreeCache.clearLocal(uint(tenantID))
	})
}

func (c *layeredDepartmentTreeCache) InvalidateDepartmentTree(ctx context.Context) error {
	tenantID := tenant.FromContext(ctx)
	c.clearLocal(tenantID)

	rc := currentRemoteCache()
	if rc == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if err := c.deleteRemote(ctx, tenantID); err != nil {
		return err
	}
	return rc.PublishString(ctx, departmentTreeInvalidateChannel, strconv.FormatUint(uint64(tenantID), 10))
}

func (c *layeredDepartmentTreeCache) getLocalRows(tenantID uint) ([]departmentTreeCacheRow, bool) {
	if c == nil {
		return nil, false
	}

	now := time.Now()
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.byTenant[tenantID]
	if !ok {
		return nil, false
	}
	if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
		return nil, false
	}
	return cloneDepartmentTreeRows(entry.rows), true
}

func (c *layeredDepartmentTreeCache) setLocalRows(tenantID uint, rows []departmentTreeCacheRow) {
	if c == nil {
		return
	}

	ttl := c.localTTL
	if ttl <= 0 {
		ttl = departmentTreeLocalCacheTTL
	}

	c.mu.Lock()
	if c.byTenant == nil {
		c.byTenant = make(map[uint]departmentTreeLocalEntry)
	}
	c.byTenant[tenantID] = departmentTreeLocalEntry{
		rows:      cloneDepartmentTreeRows(rows),
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

func (c *layeredDepartmentTreeCache) clearLocal(tenantID uint) {
	if c == nil {
		return
	}

	c.mu.Lock()
	delete(c.byTenant, tenantID)
	c.mu.Unlock()
}

func (c *layeredDepartmentTreeCache) clearAllLocal() {
	if c == nil {
		return
	}

	c.mu.Lock()
	c.byTenant = nil
	c.mu.Unlock()
}

func (c *layeredDepartmentTreeCache) deleteRemote(ctx context.Context, tenantID uint) error {
	rc := currentRemoteCache()
	if rc == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return rc.Del(ctx, departmentTreeCacheKey(tenantID))
}

func collectChildDepartmentIDs(depts []model.Department, parentID uint, ids *[]uint) {
	for _, dept := range depts {
		if dept.ParentID == parentID {
			*ids = append(*ids, dept.ID)
			collectChildDepartmentIDs(depts, dept.ID, ids)
		}
	}
}
