package api

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/audittrail"
)

// 直接种入两个租户，避免通过待测创建接口写入错误租户后，读写测试自洽通过。
func seedTenantTags(t *testing.T, srv *Server) []model.Tag {
	t.Helper()
	tags := []model.Tag{
		{TenantID: 1, Name: "共同标签", Color: "#111111"},
		{TenantID: 2, Name: "共同标签", Color: "#222222"},
	}
	if err := srv.Store.DB().Create(&tags).Error; err != nil {
		t.Fatalf("seed tags: %v", err)
	}
	return tags
}

func TestTagsTenantList(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	tags := seedTenantTags(t, srv)
	for _, tc := range []struct {
		header string
		wantID uint64
	}{
		{header: "1", wantID: tags[0].ID},
		{header: "2", wantID: tags[1].ID},
		{header: "3"},
		{header: "", wantID: tags[0].ID}, // 保留既有默认租户约定。
	} {
		t.Run("tenant="+tc.header, func(t *testing.T) {
			w, env := call(t, r, http.MethodGet, "/api/v1/crm/tags", nil, tc.header)
			if w.Code != http.StatusOK || env.Code != http.StatusOK {
				t.Fatalf("list: %d %s", w.Code, w.Body.String())
			}
			var got []model.Tag
			decode(t, env.Data, &got)
			if tc.wantID == 0 {
				if len(got) != 0 || string(env.Data) != "[]" {
					t.Fatalf("empty tenant leaked tags: %s", env.Data)
				}
				return
			}
			if len(got) != 1 || got[0].ID != tc.wantID {
				t.Fatalf("tenant %q: want tag id=%d, got %s", tc.header, tc.wantID, env.Data)
			}
		})
	}
}

func TestTagsTenantCreate(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   uint64
	}{
		{header: "1", want: 1},
		{header: "2", want: 2},
		{header: "", want: 1},
	} {
		t.Run("tenant="+tc.header, func(t *testing.T) {
			r, srv := newTestRouterWithServer(t)
			// 请求体故意声明另一个租户，归属必须来自网关头，并以落库结果为准。
			w, env := call(t, r, http.MethodPost, "/api/v1/crm/tags", map[string]any{
				"name": "新标签", "color": "#123456", "tenant_id": 3 - tc.want,
			}, tc.header)
			if w.Code != http.StatusOK || env.Code != http.StatusOK {
				t.Fatalf("create: %d %s", w.Code, w.Body.String())
			}
			var responseTag, stored model.Tag
			decode(t, env.Data, &responseTag)
			if err := srv.Store.DB().First(&stored, responseTag.ID).Error; err != nil {
				t.Fatalf("read created tag: %v", err)
			}
			if responseTag.TenantID != tc.want || stored.TenantID != tc.want {
				t.Fatalf("create tenant: want %d, response=%d stored=%d", tc.want, responseTag.TenantID, stored.TenantID)
			}
		})
	}
}

func TestTagsTenantMutation(t *testing.T) {
	testTagsTenantMutation(t, false)
}

// 生产入口注册了审计插件；同一组反例必须连同审计执行，避免无插件夹具假绿。
func TestTagsTenantMutationWithAudit(t *testing.T) {
	testTagsTenantMutation(t, true)
}

func testTagsTenantMutation(t *testing.T, audited bool) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		for _, caller := range []uint64{1, 2} {
			t.Run(fmt.Sprintf("%s/tenant=%d", method, caller), func(t *testing.T) {
				r, srv := newTestRouterWithServer(t)
				if audited {
					registerTagAudit(t, srv)
				}
				tags := seedTenantTags(t, srv)
				own, foreign := tags[caller-1], tags[2-caller]
				body := map[string]any{"name": "已修改", "color": "#abcdef", "tenant_id": foreign.TenantID}
				if method == http.MethodDelete {
					body = nil
				}
				header := strconv.FormatUint(caller, 10)
				// 既有接口对零行写入仍回 200；必须回读数据库，不能把状态码当隔离证据。
				w, _ := call(t, r, method, fmt.Sprintf("/api/v1/crm/tags/%d", foreign.ID), body, header)
				if w.Code != http.StatusOK {
					t.Fatalf("foreign mutation: %d %s", w.Code, w.Body.String())
				}
				var preserved model.Tag
				if err := srv.Store.DB().First(&preserved, foreign.ID).Error; err != nil {
					t.Fatalf("foreign tag must remain: %v", err)
				}
				if preserved.TenantID != foreign.TenantID || preserved.Name != foreign.Name || preserved.Color != foreign.Color {
					t.Fatalf("foreign tag changed: before=%+v after=%+v", foreign, preserved)
				}

				// 同时证明本租户操作有效，拒绝“所有写入都不执行”的错误修复。
				w, ownResponse := call(t, r, method, fmt.Sprintf("/api/v1/crm/tags/%d", own.ID), body, header)
				if w.Code != http.StatusOK {
					t.Fatalf("own mutation: %d %s", w.Code, w.Body.String())
				}
				if method == http.MethodDelete {
					var remaining int64
					if err := srv.Store.DB().Model(&model.Tag{}).Where("id = ?", own.ID).Count(&remaining).Error; err != nil {
						t.Fatal(err)
					}
					if remaining != 0 {
						t.Fatalf("own tag was not deleted: id=%d", own.ID)
					}
				} else {
					var returned model.Tag
					decode(t, ownResponse.Data, &returned)
					var updated model.Tag
					if err := srv.Store.DB().First(&updated, own.ID).Error; err != nil {
						t.Fatal(err)
					}
					if updated.TenantID != caller || updated.Name != "已修改" || updated.Color != "#abcdef" {
						t.Fatalf("own update failed or changed tenant: %+v", updated)
					}
					if returned.UpdatedAt.IsZero() || !returned.UpdatedAt.Equal(updated.UpdatedAt) {
						t.Fatalf("update response timestamp differs from stored row: response=%s stored=%s", returned.UpdatedAt, updated.UpdatedAt)
					}
				}
				if audited {
					var records []struct {
						TenantID uint64
						TargetID string
						Action   string
					}
					if err := srv.Store.DB().Table("audit_logs").Limit(10).Find(&records).Error; err != nil {
						t.Fatal(err)
					}
					wantAction := "update"
					if method == http.MethodDelete {
						wantAction = "delete"
					}
					if len(records) != 1 || records[0].TenantID != caller || records[0].TargetID != strconv.FormatUint(own.ID, 10) || records[0].Action != wantAction {
						t.Fatalf("only the own mutation must be audited: %+v", records)
					}
				}
			})
		}
	}
}

func registerTagAudit(t *testing.T, srv *Server) {
	t.Helper()
	// SQLite 仅替代存储方言；审计回调和 Tag target 使用生产入口的配置。
	db := srv.Store.DB()
	if err := db.Exec(`CREATE TABLE audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT, tenant_id INTEGER NOT NULL,
		actor_type TEXT, actor_id TEXT, action TEXT, target_type TEXT, target_id TEXT,
		before_json TEXT, after_json TEXT, summary TEXT, created_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := audittrail.Register(db, audittrail.Config{Targets: []audittrail.Target{{
		Model: &model.Tag{}, Table: "crm_tags", TargetType: "crm_tag", TenantField: "tenant_id",
		SnapshotFields: []string{"ID", "TenantID", "Name", "Color", "CreatedAt", "UpdatedAt"},
	}}}); err != nil {
		t.Fatal(err)
	}
}
