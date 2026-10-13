package dao

import (
	"context"
	"fmt"
	"testing"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

func TestCompleteSetAIModelsFloorsQuotasAndDispatchTargets(t *testing.T) {
	db := testdb.Open(t, &localmodel.AIModel{}, &localmodel.AIQuota{}, &localmodel.HermesOfficeFloor{}, &localmodel.HermesInstance{})
	const count = fullread.BatchSize*2 + 1
	models := make([]localmodel.AIModel, 0, count)
	floors := make([]localmodel.HermesOfficeFloor, 0, count)
	instances := make([]localmodel.HermesInstance, 0, count)
	quotas := make([]localmodel.AIQuota, 0, count)
	for i := 1; i <= count; i++ {
		id := uint64(i)
		models = append(models, localmodel.AIModel{Name: fmt.Sprintf("model-%d", i), Sort: count - i, Status: 1, Capabilities: []string{"chat"}})
		floors = append(floors, localmodel.HermesOfficeFloor{TenantID: 7, Name: fmt.Sprint(i), Sort: count - i})
		instances = append(instances, localmodel.HermesInstance{TenantID: 7, ClusterID: 42, Name: fmt.Sprint(i), Enabled: true})
		quotas = append(quotas, localmodel.AIQuota{Scope: "user", ScopeID: &id})
	}
	for _, rows := range []any{&models, &floors, &instances, &quotas} {
		if err := db.CreateInBatches(rows, fullread.BatchSize).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, other := range []any{
		&localmodel.AIModel{Name: "disabled", Status: 2},
		&localmodel.HermesOfficeFloor{TenantID: 8, Name: "other"},
		&localmodel.HermesInstance{TenantID: 8, ClusterID: 42, Name: "other", Enabled: true},
		&localmodel.HermesInstance{TenantID: 7, ClusterID: 43, Name: "different-cluster", Enabled: true},
		&localmodel.HermesInstance{TenantID: 7, ClusterID: 42, Name: "disabled", Enabled: false},
	} {
		if err := db.Create(other).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tenant.Register(db); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithContext(context.Background(), 7)
	testdb.Batches(t, db, func() {
		got, err := NewAIModelDAO(db).ListEnabled(ctx)
		if err != nil || len(got) != count {
			t.Fatalf("models=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.Sort != i || len(row.Capabilities) != 1 || row.Capabilities[0] != "chat" {
				t.Fatalf("model %d: %+v", i, row)
			}
		}
	})
	testdb.Batches(t, db, func() {
		got, err := NewHermesOfficeDAO(db).ListFloors(ctx)
		if err != nil || len(got) != count {
			t.Fatalf("floors=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.Sort != i || row.TenantID != 7 {
				t.Fatalf("floor %d: %+v", i, row)
			}
		}
	})
	testdb.Batches(t, db, func() {
		got, err := NewHermesDAO(db).ListInstancesByCluster(ctx, 42)
		if err != nil || len(got) != count {
			t.Fatalf("targets=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.ID != uint64(i+1) || row.TenantID != 7 || row.ClusterID != 42 || !row.Enabled || row.DeviceID != "" {
				t.Fatalf("target %d: %+v", i, row)
			}
		}
	})
	// Quotas retain the user-owner subquery, not just platform-admin full reads.
	if err := db.Exec("CREATE TABLE users (id BIGINT PRIMARY KEY, tenant_id BIGINT, deleted_at TIMESTAMP)").Error; err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= count; i++ {
		if err := db.Exec("INSERT INTO users (id,tenant_id) VALUES (?,7)", i).Error; err != nil {
			t.Fatal(err)
		}
	}
	otherID := uint64(count + 1)
	if err := db.Exec("INSERT INTO users (id,tenant_id) VALUES (?,8)", otherID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&localmodel.AIQuota{Scope: "user", ScopeID: &otherID}).Error; err != nil {
		t.Fatal(err)
	}
	testdb.Batches(t, db, func() {
		got, err := NewAIQuotaDAO(db).List(ctx, "user")
		if err != nil || len(got) != count {
			t.Fatalf("quotas=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.Scope != "user" || row.ScopeID == nil || *row.ScopeID != uint64(i+1) {
				t.Fatalf("quota %d: %+v", i, row)
			}
		}
	})
}
