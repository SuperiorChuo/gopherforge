package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/bpm/internal/callback"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"github.com/go-admin-kit/services/bpm/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var workerDBSeq atomic.Int64

func openWorkerStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := fmt.Sprintf("file:bpmworker%d?mode=memory&cache=shared", workerDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	s, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return s
}

func seedCallbackJob(t *testing.T, st *store.Store, tenantID uint64) model.CallbackJob {
	t.Helper()
	now := time.Now()
	inst := model.ProcessInstance{
		TenantID: tenantID, DefinitionID: 1, DefinitionKey: "contract_approval",
		Title: "合同审批", BizType: "crm_contract", BizID: "42",
		Status: model.InstApproved, FormSnapshot: model.JSONB(`{"amount":100}`),
		InitiatorID: 1, FinishedAt: &now,
	}
	if err := st.DB().Create(&inst).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}
	job := model.CallbackJob{
		TenantID: tenantID, InstanceID: inst.ID, Status: "pending", NextAt: time.Now().Add(-time.Second),
	}
	if err := st.DB().Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job
}

func TestDeliverCallbacksOnceDeletesSuccessfulJob(t *testing.T) {
	st := openWorkerStore(t)
	job := seedCallbackJob(t, st, 7)
	var gotTenant atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant.Store(r.Header.Get("X-Tenant-ID"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	d := callback.New(map[string]string{"crm_contract": srv.URL}, "test-token")
	if err := deliverCallbacksOnce(context.Background(), st, d); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	var count int64
	if err := st.DB().Model(&model.CallbackJob{}).Where("id = ?", job.ID).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 || gotTenant.Load() != "7" {
		t.Fatalf("job=%d tenant=%v", count, gotTenant.Load())
	}
}

func TestDeliverCallbacksOnceKeepsFailureForRetry(t *testing.T) {
	st := openWorkerStore(t)
	job := seedCallbackJob(t, st, 9)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	d := callback.New(map[string]string{"crm_contract": srv.URL}, "test-token")
	if err := deliverCallbacksOnce(context.Background(), st, d); err == nil {
		t.Fatal("failed delivery should be reported")
	}
	var got model.CallbackJob
	if err := st.DB().First(&got, job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if got.Status != "pending" || got.Attempts != 1 || got.LastError == "" || !got.NextAt.After(time.Now()) {
		t.Fatalf("retry state: %+v", got)
	}
}

func TestDeliverCallbacksOnceKeepsUnregisteredTargetForRetry(t *testing.T) {
	st := openWorkerStore(t)
	job := seedCallbackJob(t, st, 11)
	if err := deliverCallbacksOnce(context.Background(), st, callback.New(nil, "token")); err == nil {
		t.Fatal("unregistered target should be reported")
	}
	var got model.CallbackJob
	if err := st.DB().First(&got, job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if got.Status != "pending" || got.Attempts != 1 || got.LastError == "" {
		t.Fatalf("retry state: %+v", got)
	}
}
