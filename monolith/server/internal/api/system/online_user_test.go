package system

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	resp "github.com/go-admin-kit/server/internal/pkg/response"
	service "github.com/go-admin-kit/server/internal/service/system"
)

func TestOnlineUserAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("online_user.go")
	if err != nil {
		t.Fatalf("read online_user.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("online_user.go contains non-English source text")
	}
}

func TestOnlineUserAPIInternalErrorsDoNotExposeDetails(t *testing.T) {
	content, err := os.ReadFile("online_user.go")
	if err != nil {
		t.Fatalf("read online_user.go: %v", err)
	}

	if regexp.MustCompile(`InternalServerError\(c,\s*.*err\.Error\(\)`).Find(content) != nil {
		t.Fatal("online_user.go exposes internal error details in 500 responses")
	}
}

func TestGetOnlineUsersMasksSensitiveFieldsForNonSuperAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/online-users", nil)
	c.Set("user_id", uint(7))

	api := &OnlineUserAPI{
		onlineUserService: fakeOnlineUserService{
			users: []service.OnlineUser{{
				UserID:    9,
				Username:  "alice",
				Nickname:  "Alice",
				IP:        "192.168.10.25",
				Browser:   "Chrome",
				OS:        "Windows",
				LoginTime: time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC),
				TokenID:   "abcd1234wxyz9876",
			}},
		},
		roleLoader: fakeOnlineUserRoleLoader{roleCodes: []string{"operator"}},
	}

	api.GetOnlineUsers(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	body := decodeOnlineUserResponse(t, recorder)
	data := body.Data.(map[string]any)
	list := data["list"].([]any)
	item := list[0].(map[string]any)
	if item["ip"] != "192.168.*.*" {
		t.Fatalf("masked ip = %#v, want masked value", item["ip"])
	}
	if item["token_id"] != "abcd***9876" {
		t.Fatalf("masked token_id = %#v, want masked value", item["token_id"])
	}
}

func TestGetOnlineUsersKeepsSensitiveFieldsForSuperAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/online-users", nil)
	c.Set("user_id", uint(7))

	api := &OnlineUserAPI{
		onlineUserService: fakeOnlineUserService{
			users: []service.OnlineUser{{
				UserID:    9,
				Username:  "alice",
				Nickname:  "Alice",
				IP:        "192.168.10.25",
				Browser:   "Chrome",
				OS:        "Windows",
				LoginTime: time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC),
				TokenID:   "abcd1234wxyz9876",
			}},
		},
		roleLoader: fakeOnlineUserRoleLoader{roleCodes: []string{"super_admin"}},
	}

	api.GetOnlineUsers(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	body := decodeOnlineUserResponse(t, recorder)
	data := body.Data.(map[string]any)
	list := data["list"].([]any)
	item := list[0].(map[string]any)
	if item["ip"] != "192.168.10.25" {
		t.Fatalf("ip = %#v, want original value", item["ip"])
	}
	if item["token_id"] != "abcd1234wxyz9876" {
		t.Fatalf("token_id = %#v, want original value", item["token_id"])
	}
}

func decodeOnlineUserResponse(t *testing.T, recorder *httptest.ResponseRecorder) resp.Response {
	t.Helper()

	var body resp.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type fakeOnlineUserService struct {
	users []service.OnlineUser
	err   error
}

func (f fakeOnlineUserService) GetOnlineUsersContext(context.Context) ([]service.OnlineUser, error) {
	return f.users, f.err
}

func (f fakeOnlineUserService) GetOnlineUserCountContext(context.Context) (int64, error) {
	return int64(len(f.users)), f.err
}

func (f fakeOnlineUserService) ForceLogoutContext(context.Context, string) error {
	return f.err
}

type fakeOnlineUserRoleLoader struct {
	roleCodes []string
	err       error
}

func (f fakeOnlineUserRoleLoader) GetRoleCodesContext(context.Context, uint) ([]string, error) {
	return append([]string(nil), f.roleCodes...), f.err
}
