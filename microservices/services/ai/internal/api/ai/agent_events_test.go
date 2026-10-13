package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/agentcore"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

// 经HTTP handler验证游标不会被忽略；沿真实tenant上下文保证新分页参数不绕过隔离。
func TestAgentDetailsEventCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, ctx, definition := newAgentHandlerTestService(t)
	created, err := service.CreateRun(ctx, definition.ID, 9, agentcore.CreateRunInput{Input: "cursor", IdempotencyKey: "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	details, err := service.GetRunDetails(ctx, created.Run.ID)
	if err != nil || len(details.Events) != 1 {
		t.Fatalf("initial events: %v", err)
	}
	handler := &Handler{}
	handler.SetAgentService(service)
	for _, test := range []struct {
		name, query    string
		tenant         uint
		status, events int
	}{
		{"first", "", 7, 200, 1},
		{"after", fmt.Sprintf("?after_id=%d", details.Events[0].ID), 7, 200, 0},
		{"negative", "?after_id=-1", 7, 400, 0},
		{"malformed", "?after_id=bad", 7, 400, 0},
		{"overflow", "?after_id=9223372036854775808", 7, 400, 0},
		{"foreign", "?after_id=0", 8, 404, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/runs/:id", func(c *gin.Context) {
				c.Request = c.Request.WithContext(tenant.WithContext(ctx, test.tenant))
				handler.GetAgentRun(c)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/runs/%d%s", created.Run.ID, test.query), nil))
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d", recorder.Code, test.status)
			}
			if test.status == 200 {
				var payload struct {
					Data agentcore.RunDetails `json:"data"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload.Data.Events) != test.events || payload.Data.EventsHasMore {
					t.Fatalf("cursor ignored: %+v", payload.Data)
				}
			}
		})
	}
}
