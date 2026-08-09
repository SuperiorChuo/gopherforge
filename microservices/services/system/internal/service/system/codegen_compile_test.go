package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedCodegenGoPackagesCompile(t *testing.T) {
	files, err := NewCodegenServiceWithDB(newCodegenTestDB(t)).Generate(assetPlanRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	treeFiles, err := NewCodegenServiceWithDB(newTreeSubTestDB(t)).Generate(treeRequest())
	if err != nil {
		t.Fatalf("Generate tree: %v", err)
	}
	subFiles, err := NewCodegenServiceWithDB(newTreeSubTestDB(t)).Generate(subRequest())
	if err != nil {
		t.Fatalf("Generate sub: %v", err)
	}
	m2mFiles, err := NewCodegenServiceWithDB(newCodegenSchemaDB(t)).Generate(GenerateRequest{
		Table:  "projects",
		Module: "projects",
		Fields: []FieldConfig{{Name: "name", InList: true, InForm: true}},
		M2Ms: []M2MConfig{{
			Name: "users", JoinTable: "project_members", FKField: "project_id",
			TargetTable: "users", TargetFK: "user_id", DisplayField: "username", Label: "成员",
		}},
	})
	if err != nil {
		t.Fatalf("Generate m2m: %v", err)
	}
	dateDB := newCodegenTestDB(t)
	if err := dateDB.Exec(`CREATE TABLE IF NOT EXISTS dated_assets (
		id INTEGER PRIMARY KEY,
		published_on DATE NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	dateFiles, err := NewCodegenServiceWithDB(dateDB).Generate(GenerateRequest{
		Table: "dated_assets", Module: "datedassets",
		Fields: []FieldConfig{{Name: "published_on", InList: true, InForm: true}},
	})
	if err != nil {
		t.Fatalf("Generate date: %v", err)
	}
	files = append(files, treeFiles...)
	files = append(files, subFiles...)
	files = append(files, m2mFiles...)
	files = append(files, dateFiles...)
	sourceRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	targetServices := filepath.Join(t.TempDir(), "services")
	targetRoot := filepath.Join(targetServices, "system")
	if err := os.CopyFS(targetRoot, os.DirFS(sourceRoot)); err != nil {
		t.Fatalf("copy system module: %v", err)
	}
	sharedRoot := filepath.Clean(filepath.Join(sourceRoot, "..", "shared"))
	if err := os.CopyFS(filepath.Join(targetServices, "shared"), os.DirFS(sharedRoot)); err != nil {
		t.Fatalf("copy shared module: %v", err)
	}
	// Phase 1 起 system 引用 api/gen 契约包（grpc_demo），临时目录需一并复制才能编译。
	apiRoot := filepath.Clean(filepath.Join(sourceRoot, "..", "api"))
	if err := os.CopyFS(filepath.Join(targetServices, "api"), os.DirFS(apiRoot)); err != nil {
		t.Fatalf("copy api module: %v", err)
	}
	// 模块收敛（17→1 go.mod）后，生成代码的临时目录需要 go.mod 和 go.sum
	// 才能编译（此前各服务有独立 go.mod，system/ 目录自带）。
	moduleRoot := filepath.Clean(filepath.Join(sourceRoot, ".."))
	for _, fn := range []string{"go.mod", "go.sum"} {
		src, err := os.ReadFile(filepath.Join(moduleRoot, fn))
		if err != nil {
			t.Fatalf("read %s from module root: %v", fn, err)
		}
		if err := os.WriteFile(filepath.Join(targetServices, fn), src, 0o644); err != nil {
			t.Fatalf("write %s: %v", fn, err)
		}
	}
	const prefix = "microservices/services/system/"
	for _, file := range files {
		if !strings.HasPrefix(file.Path, prefix) || !strings.HasSuffix(file.Path, ".go") {
			continue
		}
		target := filepath.Join(targetRoot, filepath.FromSlash(strings.TrimPrefix(file.Path, prefix)))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("go", "test", "-mod=mod", "-run", "^$", "./internal/model", "./internal/dao/system", "./internal/service/system", "./internal/api/system")
	command.Dir = targetRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated packages do not compile: %v\n%s", err, output)
	}
}
