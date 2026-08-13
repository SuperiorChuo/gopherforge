package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dao "github.com/go-admin-kit/services/audit/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/secretbox"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"github.com/go-admin-kit/services/shared/pkg/webhookx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func webhookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&localmodel.AuditLog{}, &localmodel.WebhookSubscription{}, &localmodel.WebhookDelivery{}, &localmodel.WebhookCursor{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func webhookTestRing(t *testing.T) *secretbox.Keyring {
	t.Helper()
	ring, err := secretbox.NewKeyring(secretbox.Key{ID: "test", Material: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

func TestWebhookCreateStoresOnlyEncryptedSecret(t *testing.T) {
	db := webhookTestDB(t)
	svc := NewWebhookService(db, webhookTestRing(t), webhookx.Policy{AllowHTTP: true, AllowPrivate: true})
	ctx := tenant.WithContext(context.Background(), 7)
	result, err := svc.Create(ctx, WebhookMutation{Name: "CRM", EndpointURL: "http://127.0.0.1/hook", EventActions: []string{"update"}, Status: 1, CreatedBy: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.Secret == "" {
		t.Fatal("secret not returned")
	}
	var stored localmodel.WebhookSubscription
	if err := db.First(&stored, result.Subscription.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SecretCiphertext == "" || stored.SecretCiphertext == result.Secret {
		t.Fatal("secret not encrypted")
	}
	sum := sha256.Sum256([]byte(result.Secret))
	if stored.SecretHash != hex.EncodeToString(sum[:]) {
		t.Fatal("secret hash mismatch")
	}
}

func TestWebhookCreateStartsCursorAtCurrentAuditTail(t *testing.T) {
	db := webhookTestDB(t)
	if err := db.Create(&localmodel.AuditLog{TenantID: 7, Action: "create", TargetType: "role", TargetID: "1", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewWebhookService(db, webhookTestRing(t), webhookx.Policy{AllowHTTP: true, AllowPrivate: true})
	ctx := tenant.WithContext(context.Background(), 7)
	result, err := svc.Create(ctx, WebhookMutation{Name: "CRM", EndpointURL: "http://127.0.0.1/hook", EventActions: []string{"*"}, Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	var cursor localmodel.WebhookCursor
	if err := db.First(&cursor, "tenant_id = ?", 7).Error; err != nil {
		t.Fatal(err)
	}
	if cursor.LastAuditLogID == 0 || result.Subscription.StartAuditLogID != cursor.LastAuditLogID {
		t.Fatalf("cursor=%+v start=%d, want current audit tail", cursor, result.Subscription.StartAuditLogID)
	}
	created, err := dao.NewWebhookDAO(db).FanoutOnce(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Fatalf("created=%d, want no historical replay for subscription %d", created, result.Subscription.ID)
	}
}

func TestWebhookUpdatePersistsJSONActions(t *testing.T) {
	db := webhookTestDB(t)
	svc := NewWebhookService(db, webhookTestRing(t), webhookx.Policy{AllowHTTP: true, AllowPrivate: true})
	ctx := tenant.WithContext(context.Background(), 7)
	created, err := svc.Create(ctx, WebhookMutation{Name: "CRM", EndpointURL: "http://127.0.0.1/hook", EventActions: []string{"create"}, Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Update(ctx, created.Subscription.ID, WebhookMutation{Name: "CRM 2", EndpointURL: "http://127.0.0.1/hook2", EventActions: []string{"delete", "update"}, Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.EventActions) != 2 || updated.EventActions[0] != "delete" || updated.EventActions[1] != "update" {
		t.Fatalf("actions=%v", updated.EventActions)
	}
}

func TestWebhookFanoutDeliverSignsAndPersists(t *testing.T) {
	db := webhookTestDB(t)
	ring := webhookTestRing(t)
	received := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts, _ := time.ParseDuration("0s")
		_ = ts
		timestamp := int64(0)
		for _, c := range r.Header.Get("X-GAK-Timestamp") {
			timestamp = timestamp*10 + int64(c-'0')
		}
		plain := []byte("whsec_test")
		if !webhookx.Verify(plain, timestamp, body, r.Header.Get("X-GAK-Signature")) {
			t.Error("invalid signature")
		}
		var payload map[string]any
		if json.Unmarshal(body, &payload) != nil || payload["action"] != "update" {
			t.Error("bad payload")
		}
		received <- true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	cipher, err := ring.Seal([]byte("whsec_test"), secretAAD(1))
	if err != nil {
		t.Fatal(err)
	}
	sub := localmodel.WebhookSubscription{TenantID: 1, Name: "test", EndpointURL: server.URL, EventActions: []string{"update"}, SecretHash: "x", SecretCiphertext: cipher, Status: 1}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	if sub.ID != 1 {
		t.Fatalf("subscription id=%d", sub.ID)
	}
	if err := db.Create(&localmodel.WebhookCursor{TenantID: 1, LastAuditLogID: 0, UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	log := localmodel.AuditLog{TenantID: 1, ActorType: "user", ActorID: "admin", Action: "update", TargetType: "role", TargetID: "2", Summary: "role update", CreatedAt: time.Now()}
	if err := db.Create(&log).Error; err != nil {
		t.Fatal(err)
	}
	worker := &WebhookWorker{db: db, dao: dao.NewWebhookDAO(db), keyring: ring, client: server.Client(), opts: WebhookWorkerOptions{BatchSize: 10, MaxAttempts: 3, RequestTimeout: time.Second, Policy: webhookx.Policy{AllowHTTP: true, AllowPrivate: true}}}
	worker.process(context.Background())
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("receiver not called")
	}
	var delivery localmodel.WebhookDelivery
	if err := db.First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != localmodel.WebhookDeliverySent || delivery.Attempts != 1 {
		t.Fatalf("delivery=%+v", delivery)
	}
}

func TestWebhookDeleteRemovesDeliveries(t *testing.T) {
	db := webhookTestDB(t)
	svc := NewWebhookService(db, webhookTestRing(t), webhookx.Policy{AllowHTTP: true, AllowPrivate: true})
	ctx := tenant.WithContext(context.Background(), 7)
	created, err := svc.Create(ctx, WebhookMutation{Name: "CRM", EndpointURL: "http://127.0.0.1/hook", EventActions: []string{"*"}, Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&localmodel.WebhookDelivery{TenantID: 7, SubscriptionID: created.Subscription.ID, AuditLogID: 1, EventID: "audit_7_1", EventAction: "update", Payload: map[string]any{}, Status: localmodel.WebhookDeliveryPending, NextAttemptAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, created.Subscription.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&localmodel.WebhookDelivery{}).Where("subscription_id = ?", created.Subscription.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("delivery count=%d", count)
	}
}

func TestWebhookRetryDoesNotInflateTerminalFailureCounter(t *testing.T) {
	db := webhookTestDB(t)
	sub := localmodel.WebhookSubscription{TenantID: 1, Name: "test", EndpointURL: "http://127.0.0.1:1", EventActions: []string{"*"}, SecretHash: "x", SecretCiphertext: "x", Status: 1}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	delivery := localmodel.WebhookDelivery{TenantID: 1, SubscriptionID: sub.ID, AuditLogID: 1, EventID: "audit_1_1", EventAction: "update", Payload: map[string]any{"event_id": "audit_1_1"}, Status: localmodel.WebhookDeliveryRetrying, Attempts: 1, NextAttemptAt: time.Now()}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.NewWebhookDAO(db).MarkFailed(context.Background(), delivery, nil, "", "boom", 3, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&sub, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if sub.Status != 1 || sub.ConsecutiveFailures != 0 {
		t.Fatalf("subscription=%+v, want active until terminal task failure", sub)
	}
}

func TestWebhookTerminalFailureDisablesSubscription(t *testing.T) {
	db := webhookTestDB(t)
	ring := webhookTestRing(t)
	cipher, _ := ring.Seal([]byte("whsec_test"), secretAAD(1))
	sub := localmodel.WebhookSubscription{TenantID: 1, Name: "test", EndpointURL: "http://127.0.0.1:1", EventActions: []string{"*"}, SecretHash: "x", SecretCiphertext: cipher, Status: 1}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	delivery := localmodel.WebhookDelivery{TenantID: 1, SubscriptionID: sub.ID, AuditLogID: 1, EventID: "audit_1_1", EventAction: "update", Payload: map[string]any{"event_id": "audit_1_1"}, Status: localmodel.WebhookDeliveryRetrying, Attempts: 3, NextAttemptAt: time.Now()}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	d := dao.NewWebhookDAO(db)
	if err := d.MarkFailed(context.Background(), delivery, nil, "", "boom", 3, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&sub, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if sub.Status != 0 {
		t.Fatalf("status=%d", sub.Status)
	}
}
