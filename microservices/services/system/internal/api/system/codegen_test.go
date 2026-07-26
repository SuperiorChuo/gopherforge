package system

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
	"gorm.io/gorm"
)

type codegenMemorySource map[string]string

func (s codegenMemorySource) ReadFile(path string) ([]byte, error) {
	content, ok := s[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return []byte(content), nil
}

func (s codegenMemorySource) ListFiles(prefix string) ([]string, error) {
	paths := make([]string, 0)
	for path := range s {
		if strings.HasPrefix(path, prefix) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func TestCodegenCapabilitiesAndSchema(t *testing.T) {
	api := newCodegenAPIFixture(t, CodegenAPIOptions{RepositorySource: validCodegenSource(), WriteEnabled: false})
	capabilities := performCodegenRequest(t, api.Capabilities, nil, nil)
	if capabilities.Code != http.StatusOK || !strings.Contains(capabilities.Body.String(), `"write_enabled":false`) {
		t.Fatalf("capabilities = %d %s", capabilities.Code, capabilities.Body.String())
	}
	schema := performCodegenRequest(t, api.GetSchema, nil, gin.Params{{Key: "name", Value: "demo_assets"}})
	if schema.Code != http.StatusOK || !strings.Contains(schema.Body.String(), `"primary_key":"id"`) {
		t.Fatalf("schema = %d %s", schema.Code, schema.Body.String())
	}
}

func TestCodegenPreviewReturnsGenerationPlan(t *testing.T) {
	api := newCodegenAPIFixture(t, CodegenAPIOptions{RepositorySource: validCodegenSource()})
	response := performCodegenRequest(t, api.Preview, validCodegenRequest(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	plan := decodeCodegenPlan(t, response)
	if plan.Digest == "" || len(plan.Artifacts) == 0 {
		t.Fatalf("plan = %#v", plan)
	}
	if !strings.Contains(response.Body.String(), `"diagnostics":[]`) {
		t.Fatalf("empty diagnostics must be encoded as an array: %s", response.Body.String())
	}
}

func TestCodegenDownloadReturnsZIPAndRejectsStaleDigest(t *testing.T) {
	api := newCodegenAPIFixture(t, CodegenAPIOptions{RepositorySource: validCodegenSource()})
	plan := previewCodegenPlan(t, api)

	success := performCodegenRequest(t, api.Download, gin.H{
		"request": validCodegenRequest(), "expected_digest": plan.Digest,
	}, nil)
	if success.Code != http.StatusOK || success.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("download = %d %s %s", success.Code, success.Header().Get("Content-Type"), success.Body.String())
	}
	if disposition := success.Header().Get("Content-Disposition"); !strings.Contains(disposition, "codegen-assets.zip") {
		t.Fatalf("Content-Disposition = %q", disposition)
	}
	if _, err := zip.NewReader(bytes.NewReader(success.Body.Bytes()), int64(success.Body.Len())); err != nil {
		t.Fatalf("download is not ZIP: %v", err)
	}

	stale := performCodegenRequest(t, api.Download, gin.H{
		"request": validCodegenRequest(), "expected_digest": "stale",
	}, nil)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("stale = %d %s %s", stale.Code, stale.Header().Get("Content-Type"), stale.Body.String())
	}
}

func TestCodegenDownloadReturnsJSONForInvalidPlan(t *testing.T) {
	source := validCodegenSource()
	delete(source, "microservices/web/src/router/index.tsx")
	api := newCodegenAPIFixture(t, CodegenAPIOptions{RepositorySource: source})
	plan := previewCodegenPlan(t, api)
	response := performCodegenRequest(t, api.Download, gin.H{
		"request": validCodegenRequest(), "expected_digest": plan.Digest,
	}, nil)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("response = %d %s %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}

func TestCodegenWriteReturnsForbiddenWhenDisabled(t *testing.T) {
	api := newCodegenAPIFixture(t, CodegenAPIOptions{RepositorySource: validCodegenSource(), WriteEnabled: false})
	response := performCodegenRequest(t, api.Write, gin.H{
		"request": validCodegenRequest(), "expected_digest": "digest", "confirmation": "assets",
	}, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestCodegenWriteUsesDigestAndConfirmation(t *testing.T) {
	root := t.TempDir()
	for path, content := range validCodegenSource() {
		writeCodegenFixtureFile(t, root, path, content)
	}
	writer, err := systemsvc.NewRepositoryWriter(root)
	if err != nil {
		t.Fatal(err)
	}
	api := newCodegenAPIFixture(t, CodegenAPIOptions{
		RepositorySource: writer, RepositoryWriter: writer, WriteEnabled: true,
	})
	plan := previewCodegenPlan(t, api)

	wrong := performCodegenRequest(t, api.Write, gin.H{
		"request": validCodegenRequest(), "expected_digest": plan.Digest, "confirmation": "wrong",
	}, nil)
	if wrong.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation = %d %s", wrong.Code, wrong.Body.String())
	}

	success := performCodegenRequest(t, api.Write, gin.H{
		"request": validCodegenRequest(), "expected_digest": plan.Digest, "confirmation": "assets",
	}, nil)
	if success.Code != http.StatusOK || !strings.Contains(success.Body.String(), `"digest":"`+plan.Digest+`"`) {
		t.Fatalf("write = %d %s", success.Code, success.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "microservices/web/src/api/assets.ts")); err != nil {
		t.Fatalf("generated file missing: %v", err)
	}
}

func newCodegenAPIFixture(t *testing.T, options CodegenAPIOptions) *CodegenAPI {
	t.Helper()
	dsn := "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE demo_assets (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		amount_cents INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	return NewCodegenAPIWithOptions(systemsvc.NewCodegenServiceWithDB(db), options)
}

func validCodegenRequest() systemsvc.GenerateRequest {
	return systemsvc.GenerateRequest{
		Table: "demo_assets", Module: "assets", Title: "资产管理",
		Fields: []systemsvc.FieldConfig{
			{Name: "name", Label: "名称", InList: true, InSearch: true, InForm: true},
			{Name: "amount_cents", Label: "金额", InList: true, InForm: true},
		},
	}
}

func validCodegenSource() codegenMemorySource {
	return codegenMemorySource{
		"microservices/services/system/internal/api/routes.go": `package api
func setup() {
	var codegenAPI *system.CodegenAPI
	if deps.DB != nil {
		codegenAPI = system.NewCodegenAPIWithOptions(systemsvc.NewCodegenServiceWithDB(deps.DB), codegenAPIOptions())
	}
	{
		if codegenAPI != nil {
			protected.POST("/codegen/download", middleware.PermissionMiddleware("system:codegen:generate"), codegenAPI.Download)
		}
	}
}
`,
		"microservices/web/src/router/index.tsx": `const routes = [
  { path: 'system/codegen', element: lazyLoad(() => import('@/pages/system/codegen')) },
]
`,
		"microservices/web/src/layouts/MainLayout.tsx": `const MENU_DEFS = [
      { label: '代码生成', key: '/system/codegen', icon: <CodeOutlined /> },
]
const pathBreadcrumbMap = {
  '/system/codegen': '代码生成',
}
`,
		"microservices/services/system/internal/service/system/menu_seed.go": `var defaultMenuSeed = []model.Menu{
	{ID: 112, Name: "codegen", Title: "代码生成", Icon: "code", Path: "/system/codegen", Component: "system/codegen/index", ParentID: 10, Sort: 15, Status: 1, Hidden: 0, Permission: "system:codegen:list"},
}
`,
		"microservices/services/monitor/migrations/000028_existing.sql": "-- existing\n",
	}
}

func performCodegenRequest(t *testing.T, handler gin.HandlerFunc, body any, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = params
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/codegen", bytes.NewReader(payload))
	context.Request.Header.Set("Content-Type", "application/json")
	handler(context)
	return recorder
}

func previewCodegenPlan(t *testing.T, api *CodegenAPI) systemsvc.GenerationPlan {
	t.Helper()
	return decodeCodegenPlan(t, performCodegenRequest(t, api.Preview, validCodegenRequest(), nil))
}

func decodeCodegenPlan(t *testing.T, recorder *httptest.ResponseRecorder) systemsvc.GenerationPlan {
	t.Helper()
	var envelope struct {
		Data systemsvc.GenerationPlan `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v, body = %s", err, recorder.Body.String())
	}
	return envelope.Data
}

func writeCodegenFixtureFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
