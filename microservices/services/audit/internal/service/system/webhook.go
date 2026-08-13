package system

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	dao "github.com/go-admin-kit/services/audit/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/secretbox"
	"github.com/go-admin-kit/services/shared/pkg/webhookx"
	"gorm.io/gorm"
)

var (
	ErrWebhookValidation = errors.New("webhook validation failed")
	ErrWebhookSecret     = errors.New("webhook secret unavailable")
)

type WebhookMutation struct {
	Name         string
	EndpointURL  string
	EventActions []string
	Status       int8
	CreatedBy    uint
}

type WebhookSecretResult struct {
	Subscription *localmodel.WebhookSubscription `json:"subscription"`
	Secret       string                          `json:"secret"`
}

var webhookDefaults struct {
	sync.RWMutex
	keyring *secretbox.Keyring
	policy  webhookx.Policy
}

// ConfigureWebhookDefaults injects process-wide composition-root dependencies
// used by HTTP route construction. Tests may override it before SetupRoutes.
func ConfigureWebhookDefaults(keyring *secretbox.Keyring, policy webhookx.Policy) {
	webhookDefaults.Lock()
	defer webhookDefaults.Unlock()
	webhookDefaults.keyring, webhookDefaults.policy = keyring, policy
}
func DefaultWebhookKeyring() *secretbox.Keyring {
	webhookDefaults.RLock()
	defer webhookDefaults.RUnlock()
	return webhookDefaults.keyring
}
func DefaultWebhookPolicy() webhookx.Policy {
	webhookDefaults.RLock()
	defer webhookDefaults.RUnlock()
	return webhookDefaults.policy
}

type WebhookService struct {
	db      *gorm.DB
	dao     *dao.WebhookDAO
	keyring *secretbox.Keyring
	policy  webhookx.Policy
	random  io.Reader
}

func NewWebhookService(db *gorm.DB, keyring *secretbox.Keyring, policy webhookx.Policy) *WebhookService {
	return &WebhookService{db: db, dao: dao.NewWebhookDAO(db), keyring: keyring, policy: policy, random: rand.Reader}
}

func (s *WebhookService) List(ctx context.Context, page, pageSize int) ([]localmodel.WebhookSubscription, int64, error) {
	return s.dao.ListSubscriptions(ctx, page, pageSize)
}
func (s *WebhookService) ListDeliveries(ctx context.Context, subscriptionID uint, page, pageSize int) ([]localmodel.WebhookDelivery, int64, error) {
	if subscriptionID > 0 {
		if _, err := s.dao.GetSubscription(ctx, subscriptionID); err != nil {
			return nil, 0, err
		}
	}
	return s.dao.ListDeliveries(ctx, subscriptionID, page, pageSize)
}

func (s *WebhookService) Create(ctx context.Context, input WebhookMutation) (*WebhookSecretResult, error) {
	if err := s.validate(ctx, &input); err != nil {
		return nil, err
	}
	secret, hash, err := s.generateSecret()
	if err != nil {
		return nil, err
	}
	row := &localmodel.WebhookSubscription{Name: input.Name, EndpointURL: input.EndpointURL, EventActions: input.EventActions, SecretHash: hash, Status: input.Status, CreatedBy: input.CreatedBy}
	if err := s.dao.CreateSubscription(ctx, row); err != nil {
		return nil, err
	}
	ciphertext, err := s.keyring.Seal([]byte(secret), secretAAD(row.ID))
	if err != nil {
		_ = s.dao.DeleteSubscription(ctx, row.ID)
		return nil, err
	}
	if err := s.dao.UpdateSubscription(ctx, row.ID, map[string]any{"secret_ciphertext": ciphertext, "updated_at": time.Now()}); err != nil {
		_ = s.dao.DeleteSubscription(ctx, row.ID)
		return nil, err
	}
	row.SecretCiphertext = ""
	return &WebhookSecretResult{Subscription: row, Secret: secret}, nil
}

func (s *WebhookService) Update(ctx context.Context, id uint, input WebhookMutation) (*localmodel.WebhookSubscription, error) {
	if err := s.validate(ctx, &input); err != nil {
		return nil, err
	}
	if err := s.dao.UpdateSubscription(ctx, id, map[string]any{"name": input.Name, "endpoint_url": input.EndpointURL, "event_actions": input.EventActions, "status": input.Status, "consecutive_failures": 0, "last_error": "", "updated_at": time.Now()}); err != nil {
		return nil, err
	}
	return s.dao.GetSubscription(ctx, id)
}
func (s *WebhookService) Delete(ctx context.Context, id uint) error {
	return s.dao.DeleteSubscription(ctx, id)
}

func (s *WebhookService) ResetSecret(ctx context.Context, id uint) (*WebhookSecretResult, error) {
	if s.keyring == nil {
		return nil, ErrWebhookSecret
	}
	row, err := s.dao.GetSubscription(ctx, id)
	if err != nil {
		return nil, err
	}
	secret, hash, err := s.generateSecret()
	if err != nil {
		return nil, err
	}
	ciphertext, err := s.keyring.Seal([]byte(secret), secretAAD(row.ID))
	if err != nil {
		return nil, err
	}
	if err := s.dao.UpdateSubscription(ctx, id, map[string]any{"secret_hash": hash, "secret_ciphertext": ciphertext, "updated_at": time.Now()}); err != nil {
		return nil, err
	}
	row.SecretHash, row.SecretCiphertext = "", ""
	return &WebhookSecretResult{Subscription: row, Secret: secret}, nil
}

func (s *WebhookService) validate(ctx context.Context, input *WebhookMutation) error {
	input.Name = strings.TrimSpace(input.Name)
	input.EndpointURL = strings.TrimSpace(input.EndpointURL)
	if input.Name == "" || len(input.Name) > 128 {
		return fmt.Errorf("%w: name is required and must not exceed 128 characters", ErrWebhookValidation)
	}
	u, err := webhookx.ValidateEndpoint(ctx, input.EndpointURL, s.policy)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookValidation, err)
	}
	input.EndpointURL = u.String()
	input.EventActions = normalizeActions(input.EventActions)
	if len(input.EventActions) == 0 || len(input.EventActions) > 50 {
		return fmt.Errorf("%w: select between 1 and 50 event actions", ErrWebhookValidation)
	}
	if input.Status != localmodel.WebhookSubscriptionDisabled && input.Status != localmodel.WebhookSubscriptionEnabled {
		return fmt.Errorf("%w: invalid status", ErrWebhookValidation)
	}
	return nil
}

func normalizeActions(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (s *WebhookService) generateSecret() (string, string, error) {
	if s.keyring == nil {
		return "", "", ErrWebhookSecret
	}
	buf := make([]byte, 32)
	if _, err := io.ReadFull(s.random, buf); err != nil {
		return "", "", err
	}
	secret := "whsec_" + base64.RawURLEncoding.EncodeToString(buf)
	clear(buf)
	sum := sha256.Sum256([]byte(secret))
	return secret, hex.EncodeToString(sum[:]), nil
}
func secretAAD(id uint) string { return fmt.Sprintf("webhook:%d:secret", id) }

// WebhookWorkerOptions configures the audit fact fanout and HTTP worker.
type WebhookWorkerOptions struct {
	ScanInterval   time.Duration
	BatchSize      int
	MaxAttempts    int
	RequestTimeout time.Duration
	Policy         webhookx.Policy
}

func (o *WebhookWorkerOptions) defaults() {
	if o.ScanInterval <= 0 {
		o.ScanInterval = 2 * time.Second
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 50
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = 10 * time.Second
	}
}

type WebhookWorker struct {
	db      *gorm.DB
	dao     *dao.WebhookDAO
	keyring *secretbox.Keyring
	client  *http.Client
	opts    WebhookWorkerOptions
}

func StartWebhookWorker(ctx context.Context, db *gorm.DB, keyring *secretbox.Keyring, opts WebhookWorkerOptions) *WebhookWorker {
	if db == nil || keyring == nil {
		return nil
	}
	opts.defaults()
	worker := &WebhookWorker{db: db, dao: dao.NewWebhookDAO(db), keyring: keyring, client: webhookx.NewClient(opts.Policy, opts.RequestTimeout), opts: opts}
	go worker.run(ctx)
	return worker
}
func (w *WebhookWorker) run(ctx context.Context) {
	w.process(ctx)
	ticker := time.NewTicker(w.opts.ScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.process(ctx)
		}
	}
}
func (w *WebhookWorker) process(ctx context.Context) {
	_, err := w.dao.FanoutOnce(ctx, w.opts.BatchSize)
	if err != nil {
		logWebhookError("fanout", err)
		return
	}
	rows, err := w.dao.ClaimDeliveries(ctx, w.opts.BatchSize)
	if err != nil {
		logWebhookError("claim", err)
		return
	}
	for _, row := range rows {
		w.deliver(ctx, row)
	}
	jobbeat.Report(w.db, jobbeat.Run{Key: "audit.webhook_delivery", Service: "audit-service", Description: "开放平台 Webhook 审计事件投递", IntervalSec: int64(w.opts.ScanInterval.Seconds()), StartedAt: time.Now()})
}
func (w *WebhookWorker) deliver(ctx context.Context, delivery localmodel.WebhookDelivery) {
	sub, err := w.subscription(ctx, delivery)
	if err != nil {
		w.fail(ctx, delivery, nil, "", err)
		return
	}
	body, err := json.Marshal(delivery.Payload)
	if err != nil {
		w.fail(ctx, delivery, nil, "", err)
		return
	}
	plain, _, err := w.keyring.Open(sub.SecretCiphertext, secretAAD(sub.ID))
	if err != nil {
		w.fail(ctx, delivery, nil, "", err)
		return
	}
	defer clear(plain)
	timestamp := time.Now().Unix()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.EndpointURL, bytes.NewReader(body))
	if err != nil {
		w.fail(ctx, delivery, nil, "", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Go-Admin-Kit-Webhook/1.0")
	req.Header.Set("X-GAK-Event-ID", delivery.EventID)
	req.Header.Set("X-GAK-Timestamp", fmt.Sprint(timestamp))
	req.Header.Set("X-GAK-Signature", webhookx.Signature(plain, timestamp, body))
	resp, err := w.client.Do(req)
	if err != nil {
		w.fail(ctx, delivery, nil, "", err)
		return
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	status := resp.StatusCode
	if status < 200 || status >= 300 {
		w.fail(ctx, delivery, &status, string(responseBody), fmt.Errorf("webhook returned HTTP %d", status))
		return
	}
	if err := w.dao.MarkSent(ctx, delivery.ID, delivery.SubscriptionID, status, string(responseBody), time.Now()); err != nil {
		logWebhookError("mark sent", err)
	}
}
func (w *WebhookWorker) subscription(ctx context.Context, delivery localmodel.WebhookDelivery) (*localmodel.WebhookSubscription, error) {
	var row localmodel.WebhookSubscription
	err := w.db.WithContext(ctx).Where("id=? AND tenant_id=? AND status=?", delivery.SubscriptionID, delivery.TenantID, localmodel.WebhookSubscriptionEnabled).First(&row).Error
	return &row, err
}
func (w *WebhookWorker) fail(ctx context.Context, delivery localmodel.WebhookDelivery, status *int, body string, err error) {
	delay := retryDelay(delivery.Attempts)
	if markErr := w.dao.MarkFailed(ctx, delivery, status, body, truncateWebhook(err.Error(), 1024), w.opts.MaxAttempts, time.Now().Add(delay)); markErr != nil {
		logWebhookError("mark failed", markErr)
	}
}
func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * time.Minute
}
func truncateWebhook(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
func logWebhookError(stage string, err error) {
	if logger.Logger != nil {
		logger.Warn("webhook worker failed", logger.String("stage", stage), logger.Err(err))
	}
}
