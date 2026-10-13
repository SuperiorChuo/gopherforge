package dao

import (
	"context"
	"encoding/json"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/identityclient"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompleteSetEnabledCronJobs(t *testing.T) {
	db := testdb.Open(t, &localmodel.HermesCronJob{})
	const n = fullread.BatchSize*2 + 1
	rows := make([]localmodel.HermesCronJob, n)
	for i := range rows {
		rows[i] = localmodel.HermesCronJob{TenantID: 7, Name: fmt.Sprint(i), Enabled: true}
	}
	if err := db.CreateInBatches(&rows, 128).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range []localmodel.HermesCronJob{{TenantID: 8, Name: "foreign", Enabled: true}, {TenantID: 7, Name: "disabled", Enabled: false}} {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tenant.Register(db); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithContext(context.Background(), 7)
	d := NewHermesDAO(db)
	testdb.Batches(t, db, func() {
		got, err := d.ListEnabledCronJobs(ctx)
		if err != nil || len(got) != n {
			t.Fatalf("jobs=%d err=%v", len(got), err)
		}
		for i, r := range got {
			if r.ID != rows[i].ID || r.TenantID != 7 || !r.Enabled {
				t.Fatalf("job %d: %+v", i, r)
			}
		}
	})
	testdb.FailLater(t, db, func() {
		got, err := d.ListEnabledCronJobs(ctx)
		if err == nil || got != nil {
			t.Fatalf("partial jobs=%d err=%v", len(got), err)
		}
	})
}

func TestCompleteSetSeatProjectionPostgres(t *testing.T) {
	db := testdb.Open(t, &localmodel.HermesOfficeSeat{})
	if db.Dialector.Name() != "postgres" {
		t.Skip("DISTINCT ON projection requires isolated PostgreSQL")
	}
	const n = fullread.BatchSize*2 + 1
	rows := make([]localmodel.HermesOfficeSeat, n)
	for i := range rows {
		rows[i] = localmodel.HermesOfficeSeat{TenantID: 7, FloorID: 9, UserID: 17, Label: fmt.Sprint(i), X: float64(i)}
	}
	rows[n-1].UserID = 0
	if err := db.CreateInBatches(&rows, 128).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range []localmodel.HermesOfficeSeat{{TenantID: 8, FloorID: 9, UserID: 17}, {TenantID: 7, FloorID: 10, UserID: 17}} {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`CREATE TABLE hermes_devices (id BIGINT PRIMARY KEY, tenant_id BIGINT, user_id BIGINT, device_id TEXT, actual_state TEXT, health_status TEXT, hermes_version TEXT, last_seen_at TIMESTAMP, revoked_at TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, r := range []struct {
		id, tenant int
		device     string
		seen       *time.Time
		revoked    *time.Time
	}{{1, 7, "null-heartbeat", nil, nil}, {2, 7, "same-time-lower-id", &now, nil}, {3, 7, "winner", &now, nil}, {4, 7, "revoked", &now, &now}, {5, 8, "foreign", &now, nil}} {
		if err := db.Exec(`INSERT INTO hermes_devices VALUES (?, ?, 17, ?, 'running', 'healthy', 'fixture-version', ?, ?)`, r.id, r.tenant, r.device, r.seen, r.revoked).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tenant.Register(db); err != nil {
		t.Fatal(err)
	}
	var nameCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/internal/users/display-names" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			IDs []uint64 `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) != 1 || body.IDs[0] != 17 {
			t.Errorf("name lookup=%+v err=%v", body, err)
			w.WriteHeader(400)
			return
		}
		nameCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"names": map[string]string{"17": "fixture employee"}})
	}))
	defer server.Close()
	// Keep service discovery inside this test; never contact a developer Consul.
	t.Setenv("CONSUL_HTTP_ADDR", server.URL)
	client, err := identityclient.New(server.URL, "isolated-fixture")
	if err != nil {
		t.Fatal(err)
	}
	d := NewHermesOfficeDAO(db)
	d.SetIdentity(client)
	ctx := tenant.WithContext(context.Background(), 7)
	testdb.Batches(t, db, func() {
		got, err := d.ListSeatViews(ctx, 9)
		if err != nil || len(got) != n {
			t.Fatalf("seats=%d err=%v", len(got), err)
		}
		for i, r := range got {
			if r.ID != rows[i].ID || r.TenantID != 7 || r.FloorID != 9 || r.Label != fmt.Sprint(i) || r.X != float64(i) {
				t.Fatalf("seat %d: %+v", i, r)
			}
			if i == n-1 {
				if r.DeviceID != "" || r.DeviceSeenAt != nil || r.Username != "" {
					t.Fatalf("empty seat=%+v", r)
				}
				continue
			}
			if r.DeviceID != "winner" || r.ActualState != "running" || r.HealthStatus != "healthy" || r.HermesVersion != "fixture-version" || r.DeviceSeenAt == nil || !r.DeviceSeenAt.Equal(now) || r.Username != "fixture employee" || r.Nickname != r.Username {
				t.Fatalf("projection %d: %+v", i, r)
			}
		}
	})
	if nameCalls.Load() != 1 {
		t.Fatalf("name calls=%d", nameCalls.Load())
	}
	testdb.FailLater(t, db, func() {
		got, err := d.ListSeatViews(ctx, 9)
		if err == nil || got != nil {
			t.Fatalf("partial seats=%d err=%v", len(got), err)
		}
	})
	if nameCalls.Load() != 1 {
		t.Fatal("partial reads must not trigger name enrichment")
	}
}
