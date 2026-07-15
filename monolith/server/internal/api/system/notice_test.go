package system

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/model"
	systemsvc "github.com/go-admin-kit/server/internal/service/system"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestResolveNoticeCreatorDefaultsToEnglishSystemActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	creatorID, creatorName := resolveNoticeCreator(ctx)
	if creatorID != 0 {
		t.Fatalf("creatorID = %d, want 0", creatorID)
	}
	if creatorName != "system" {
		t.Fatalf("creatorName = %q, want %q", creatorName, "system")
	}
}

func TestResolveNoticeCreatorUsesAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("user_id", uint(7))
	ctx.Set("username", "alice")

	creatorID, creatorName := resolveNoticeCreator(ctx)
	if creatorID != 7 {
		t.Fatalf("creatorID = %d, want 7", creatorID)
	}
	if creatorName != "alice" {
		t.Fatalf("creatorName = %q, want %q", creatorName, "alice")
	}
}

func TestNoticeAPIMessagesUseEnglish(t *testing.T) {
	content, err := os.ReadFile("notice.go")
	if err != nil {
		t.Fatalf("read notice.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("notice.go contains non-English source text")
	}
}

func TestCreateNoticeEmailFailureDoesNotFailRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := setupNoticeAPITestDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO \"notices\"").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectCommit()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/notices", bytes.NewBufferString(`{
		"title": "Maintenance",
		"content": "Maintenance window tonight",
		"type": 1,
		"status": 1
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	emailNotifier := &failingNoticeEmailNotifier{err: errors.New("smtp unavailable")}
	api := &NoticeAPI{
		noticeService: systemsvc.NewNoticeServiceWithDB(db),
		emailNotifier: emailNotifier,
	}

	api.CreateNotice(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want 200", recorder.Code, recorder.Body.String())
	}
	waitForNoticeEmailCalls(t, emailNotifier, 1)
}

func TestUpdateNoticeStatusEmailFailureDoesNotFailRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := setupNoticeAPITestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE \"notices\" SET \"status\"=").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "notices" WHERE "notices"."id" = $1 ORDER BY "notices"."id" LIMIT $2`)).
		WithArgs(7, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "content", "type", "status"}).
			AddRow(uint(7), "Maintenance", "Maintenance window tonight", int8(1), int8(1)))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = []gin.Param{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/notices/7/status", bytes.NewBufferString(`{"status": 1}`))
	c.Request.Header.Set("Content-Type", "application/json")

	emailNotifier := &failingNoticeEmailNotifier{err: errors.New("smtp unavailable")}
	api := &NoticeAPI{
		noticeService: systemsvc.NewNoticeServiceWithDB(db),
		broadcaster:   systemsvc.NewNotificationBroadcaster(),
		emailNotifier: emailNotifier,
	}

	api.UpdateNoticeStatus(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want 200", recorder.Code, recorder.Body.String())
	}
	waitForNoticeEmailCalls(t, emailNotifier, 1)
}

func TestUpdateNoticeSendsEmailWhenNoticeBecomesEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := setupNoticeAPITestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "notices" WHERE "notices"."id" = $1 ORDER BY "notices"."id" LIMIT $2`)).
		WithArgs(7, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "content", "type", "status"}).
			AddRow(uint(7), "Draft", "Draft content", int8(1), int8(0)))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE \"notices\" SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = []gin.Param{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/notices/7", bytes.NewBufferString(`{
		"title": "Maintenance",
		"content": "Maintenance window tonight",
		"type": 1,
		"status": 1
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	emailNotifier := &failingNoticeEmailNotifier{err: errors.New("smtp unavailable")}
	api := &NoticeAPI{
		noticeService: systemsvc.NewNoticeServiceWithDB(db),
		broadcaster:   systemsvc.NewNotificationBroadcaster(),
		emailNotifier: emailNotifier,
	}

	api.UpdateNotice(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want 200", recorder.Code, recorder.Body.String())
	}
	waitForNoticeEmailCalls(t, emailNotifier, 1)
}

func TestCreateNoticeReturnsBeforeBlockedEmailNotifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := setupNoticeAPITestDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO \"notices\"").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectCommit()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/notices", bytes.NewBufferString(`{
		"title": "Maintenance",
		"content": "Maintenance window tonight",
		"type": 1,
		"status": 1
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	emailNotifier := &failingNoticeEmailNotifier{
		block:   make(chan struct{}),
		started: make(chan struct{}),
	}
	api := &NoticeAPI{
		noticeService: systemsvc.NewNoticeServiceWithDB(db),
		broadcaster:   systemsvc.NewNotificationBroadcaster(),
		emailNotifier: emailNotifier,
	}

	done := make(chan struct{})
	go func() {
		api.CreateNotice(c)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		close(emailNotifier.block)
		t.Fatal("CreateNotice blocked on email notification")
	}
	select {
	case <-emailNotifier.started:
		close(emailNotifier.block)
	case <-time.After(time.Second):
		t.Fatal("email notification was not attempted")
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want 200", recorder.Code, recorder.Body.String())
	}
}

type failingNoticeEmailNotifier struct {
	calls   int32
	err     error
	started chan struct{}
	once    sync.Once
	block   chan struct{}
}

func (f *failingNoticeEmailNotifier) SendNoticeEnabledContext(ctx context.Context, notice *model.Notice) error {
	atomic.AddInt32(&f.calls, 1)
	if f.started != nil {
		f.once.Do(func() { close(f.started) })
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.err
}

func waitForNoticeEmailCalls(t *testing.T, notifier *failingNoticeEmailNotifier, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&notifier.calls) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("email notifier calls = %d, want %d", atomic.LoadInt32(&notifier.calls), want)
}

func setupNoticeAPITestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

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

	return db, mock
}
