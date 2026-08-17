package system

import (
	"encoding/json"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

type memoryRepositorySource map[string]string

func newMemoryRepositorySource(files map[string]string) memoryRepositorySource {
	return memoryRepositorySource(files)
}

func (s memoryRepositorySource) ReadFile(path string) ([]byte, error) {
	content, ok := s[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return []byte(content), nil
}

func (s memoryRepositorySource) ListFiles(prefix string) ([]string, error) {
	paths := make([]string, 0)
	for path := range s {
		if strings.HasPrefix(path, prefix) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func assetPlanRequest() GenerateRequest {
	return GenerateRequest{
		Table:  "demo_assets",
		Module: "assets",
		Title:  "资产管理",
		Fields: []FieldConfig{
			{Name: "name", Label: "名称", InList: true, InSearch: true, InForm: true},
			{Name: "amount_cents", Label: "金额", InList: true, InForm: true},
		},
	}
}

func validIntegrationFiles() map[string]string {
	return map[string]string{
		"microservices/services/system/internal/api/routes.go": `package api

func setup() {
	var codegenAPI *system.CodegenAPI
	if deps.DB != nil {
		codegenAPI = system.NewCodegenAPIWithOptions(systemsvc.NewCodegenServiceWithDB(deps.DB), codegenAPIOptions())
	}
	{
		if codegenAPI != nil {
			protected.GET("/codegen/tables", middleware.PermissionMiddleware("system:codegen:list"), codegenAPI.GetTables)
			protected.GET("/codegen/tables/:name/columns", middleware.PermissionMiddleware("system:codegen:list"), codegenAPI.GetColumns)
			protected.POST("/codegen/preview", middleware.PermissionMiddleware("system:codegen:generate"), codegenAPI.Preview)
			protected.POST("/codegen/download", middleware.PermissionMiddleware("system:codegen:generate"), codegenAPI.Download)
		}
	}
}
`,
		"microservices/web/src/router/index.tsx": `const routes = [
  { path: 'system/codegen', element: lazyLoad(() => import('@/pages/system/codegen')) },
]
`,
		"microservices/web/src/layouts/menu-defs.tsx": `const MENU_DEFS = [
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
	}
}

func mustBuildAssetPlan(t *testing.T, source RepositorySource) GenerationPlan {
	t.Helper()
	plan, err := NewCodegenServiceWithDB(newCodegenTestDB(t)).BuildPlan(assetPlanRequest(), source)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	return plan
}

func findArtifact(t *testing.T, plan GenerationPlan, path string) PlannedArtifact {
	t.Helper()
	for _, artifact := range plan.Artifacts {
		if artifact.Path == path {
			return artifact
		}
	}
	t.Fatalf("artifact %q missing from %#v", path, plan.Artifacts)
	return PlannedArtifact{}
}

func TestBuildPlanIsDeterministic(t *testing.T) {
	source := newMemoryRepositorySource(validIntegrationFiles())
	first := mustBuildAssetPlan(t, source)
	second := mustBuildAssetPlan(t, source)
	if first.Digest == "" || first.Digest != second.Digest {
		t.Fatalf("digest changed: %q != %q", first.Digest, second.Digest)
	}
	if !sort.SliceIsSorted(first.Artifacts, func(i, j int) bool {
		return first.Artifacts[i].Path < first.Artifacts[j].Path
	}) {
		t.Fatal("artifacts are not sorted")
	}
}

func TestBuildPlanDigestExcludesDigestField(t *testing.T) {
	plan := mustBuildAssetPlan(t, newMemoryRepositorySource(validIntegrationFiles()))
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	delete(payload, "digest")
	canonical, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Digest != contentHashBytes(canonical) {
		t.Fatalf("digest = %s, want payload-only digest %s", plan.Digest, contentHashBytes(canonical))
	}
}

func TestBuildPlanMarksExistingCreateAsConflict(t *testing.T) {
	files := validIntegrationFiles()
	files["microservices/web/src/api/assets.ts"] = "owned by developer\n"
	plan := mustBuildAssetPlan(t, newMemoryRepositorySource(files))
	artifact := findArtifact(t, plan, "microservices/web/src/api/assets.ts")
	if artifact.Operation != ArtifactCreate || artifact.Status != ArtifactConflict {
		t.Fatalf("artifact = %#v", artifact)
	}
	found := false
	for _, diagnostic := range plan.Diagnostics {
		if diagnostic.Code == "create_target_exists" && diagnostic.Path == artifact.Path {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", plan.Diagnostics)
	}
}

func TestBuildPlanAllocatesNextPermissionMigration(t *testing.T) {
	files := validIntegrationFiles()
	files["microservices/services/monitor/migrations/000028_add_ai_platform.sql"] = "-- existing\n"
	plan := mustBuildAssetPlan(t, newMemoryRepositorySource(files))
	artifact := findArtifact(t, plan, "microservices/services/monitor/migrations/000029_codegen_assets.sql")
	if artifact.Operation != ArtifactCreate || artifact.Status != ArtifactReady {
		t.Fatalf("migration artifact = %#v", artifact)
	}
}

func TestBuildPlanCreatesHashGuardedIntegrationPatches(t *testing.T) {
	plan := mustBuildAssetPlan(t, newMemoryRepositorySource(validIntegrationFiles()))
	for _, path := range []string{
		"microservices/services/system/internal/api/routes.go",
		"microservices/web/src/router/index.tsx",
		"microservices/web/src/layouts/menu-defs.tsx",
		"microservices/services/system/internal/service/system/menu_seed.go",
	} {
		artifact := findArtifact(t, plan, path)
		if artifact.Operation != ArtifactPatch || artifact.Status != ArtifactReady {
			t.Fatalf("patch artifact %s = %#v", path, artifact)
		}
		if artifact.ExpectedHash == "" || artifact.ResultHash == "" || artifact.ExpectedHash == artifact.ResultHash {
			t.Fatalf("patch hashes for %s = %#v", path, artifact)
		}
		if !strings.Contains(artifact.Diff, "assets") {
			t.Fatalf("patch diff for %s does not mention module:\n%s", path, artifact.Diff)
		}
	}
}

func TestBuildPlanRoutePatchToleratesAdditionalCodegenRoutes(t *testing.T) {
	files := validIntegrationFiles()
	path := "microservices/services/system/internal/api/routes.go"
	anchor := "\t\t\tprotected.GET(\"/codegen/tables\", middleware.PermissionMiddleware(\"system:codegen:list\"), codegenAPI.GetTables)"
	files[path] = strings.Replace(files[path], anchor,
		"\t\t\tprotected.GET(\"/codegen/capabilities\", codegenAPI.Capabilities)\n"+anchor, 1)
	plan := mustBuildAssetPlan(t, newMemoryRepositorySource(files))
	artifact := findArtifact(t, plan, path)
	if artifact.Status != ArtifactReady {
		t.Fatalf("route patch should tolerate adjacent codegen routes: %#v", artifact)
	}
}

func TestBuildPlanHashesOriginalIntegrationBytes(t *testing.T) {
	files := validIntegrationFiles()
	path := "microservices/web/src/router/index.tsx"
	files[path] = strings.ReplaceAll(files[path], "\n", "\r\n")
	plan := mustBuildAssetPlan(t, newMemoryRepositorySource(files))
	artifact := findArtifact(t, plan, path)
	if artifact.ExpectedHash != contentHash(files[path]) {
		t.Fatalf("expected hash = %s, want raw source hash %s", artifact.ExpectedHash, contentHash(files[path]))
	}
}

func TestBuildPlanMarksMissingIntegrationFileInvalid(t *testing.T) {
	files := validIntegrationFiles()
	delete(files, "microservices/web/src/router/index.tsx")
	plan := mustBuildAssetPlan(t, newMemoryRepositorySource(files))
	artifact := findArtifact(t, plan, "microservices/web/src/router/index.tsx")
	if artifact.Status != ArtifactInvalid {
		t.Fatalf("artifact = %#v", artifact)
	}
	found := false
	for _, diagnostic := range plan.Diagnostics {
		if diagnostic.Path == artifact.Path && strings.Contains(diagnostic.Message, "不存在") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing-file diagnostic not found: %#v", plan.Diagnostics)
	}
}
