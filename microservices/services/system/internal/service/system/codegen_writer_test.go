package system

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestRepositoryWriterWritesReadyPlan(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "microservices/web/src/router/index.tsx", "before\n")
	writer, err := NewRepositoryWriter(root)
	if err != nil {
		t.Fatalf("NewRepositoryWriter: %v", err)
	}
	plan := exportWriterTestPlan(t, "before\n")
	result, err := writer.Write(plan)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if result.Digest != plan.Digest || len(result.Created) != 1 || len(result.Patched) != 1 {
		t.Fatalf("result = %#v", result)
	}
	assertRepoContent(t, root, "microservices/web/src/api/assets.ts", "export const assets = true\n")
	assertRepoContent(t, root, "microservices/web/src/router/index.tsx", "after\n")
}

func TestWriterRejectsStalePatchBeforeCreatingFiles(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "microservices/web/src/router/index.tsx", "before\n")
	plan := exportWriterTestPlan(t, "before\n")
	writeRepoFile(t, root, "microservices/web/src/router/index.tsx", "changed\n")
	writer, err := NewRepositoryWriter(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.Write(plan)
	if !errors.Is(err, ErrRepositoryConflict) {
		t.Fatalf("error = %v", err)
	}
	assertRepoMissing(t, root, "microservices/web/src/api/assets.ts")
	assertRepoContent(t, root, "microservices/web/src/router/index.tsx", "changed\n")
}

func TestWriterRollsBackAppliedFilesWhenRenameFails(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "microservices/web/src/router/index.tsx", "before\n")
	writer, err := NewRepositoryWriter(root)
	if err != nil {
		t.Fatal(err)
	}
	renames := 0
	writer.rename = func(oldPath, newPath string) error {
		renames++
		if renames == 2 {
			return errors.New("injected rename failure")
		}
		return os.Rename(oldPath, newPath)
	}
	_, err = writer.Write(exportWriterTestPlan(t, "before\n"))
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("error = %v", err)
	}
	assertRepoMissing(t, root, "microservices/web/src/api/assets.ts")
	assertRepoContent(t, root, "microservices/web/src/router/index.tsx", "before\n")
}

func TestWriterRestoresAppliedPatchWhenLaterCreateFails(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "microservices/a-router.tsx", "before\n")
	plan := GenerationPlan{Artifacts: []PlannedArtifact{
		{
			Path: "microservices/a-router.tsx", Operation: ArtifactPatch,
			Content: "after\n", Diff: unifiedDiff("microservices/a-router.tsx", "before\n", "after\n"),
			ExpectedHash: contentHash("before\n"), ResultHash: contentHash("after\n"), Status: ArtifactReady,
		},
		{
			Path: "microservices/z-new.go", Operation: ArtifactCreate,
			Content: "generated\n", ResultHash: contentHash("generated\n"), Status: ArtifactReady,
		},
	}}
	if err := plan.normalize(); err != nil {
		t.Fatal(err)
	}
	writer, err := NewRepositoryWriter(root)
	if err != nil {
		t.Fatal(err)
	}
	renames := 0
	writer.rename = func(oldPath, newPath string) error {
		renames++
		if renames == 2 {
			return errors.New("injected create failure")
		}
		return os.Rename(oldPath, newPath)
	}
	if _, err := writer.Write(plan); err == nil {
		t.Fatal("expected write failure")
	}
	assertRepoContent(t, root, "microservices/a-router.tsx", "before\n")
	assertRepoMissing(t, root, "microservices/z-new.go")
	assertRepoMissing(t, root, ".codegen.lock")
	assertRepoMissing(t, root, ".codegen-tmp")
}

func TestRepositoryWriterRejectsUnsafePathsAndSymlinks(t *testing.T) {
	for name, artifactPath := range map[string]string{
		"父目录穿越": "../outside.go",
		"绝对路径":  filepath.Join(string(filepath.Separator), "tmp", "outside.go"),
		"反斜杠路径": `..\outside.go`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writer, err := NewRepositoryWriter(root)
			if err != nil {
				t.Fatal(err)
			}
			plan := singleCreatePlan(t, artifactPath, "unsafe\n")
			_, err = writer.Write(plan)
			if !errors.Is(err, ErrPathOutsideRoot) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	t.Run("符号链接父目录", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "microservices")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		writer, err := NewRepositoryWriter(root)
		if err != nil {
			t.Fatal(err)
		}
		_, err = writer.Write(singleCreatePlan(t, "microservices/escape.go", "unsafe\n"))
		if !errors.Is(err, ErrPathOutsideRoot) {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(outside, "escape.go")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("outside target was touched: %v", err)
		}
	})
}

func TestWriterRejectsExistingCreateAndActiveLock(t *testing.T) {
	t.Run("同名文件", func(t *testing.T) {
		root := t.TempDir()
		writeRepoFile(t, root, "microservices/web/src/api/assets.ts", "owned\n")
		writer, err := NewRepositoryWriter(root)
		if err != nil {
			t.Fatal(err)
		}
		_, err = writer.Write(singleCreatePlan(t, "microservices/web/src/api/assets.ts", "generated\n"))
		if !errors.Is(err, ErrRepositoryConflict) {
			t.Fatalf("error = %v", err)
		}
		assertRepoContent(t, root, "microservices/web/src/api/assets.ts", "owned\n")
	})

	t.Run("写入锁", func(t *testing.T) {
		root := t.TempDir()
		writeRepoFile(t, root, ".codegen.lock", "busy\n")
		writer, err := NewRepositoryWriter(root)
		if err != nil {
			t.Fatal(err)
		}
		_, err = writer.Write(singleCreatePlan(t, "microservices/new.go", "generated\n"))
		if !errors.Is(err, ErrRepositoryLocked) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestRepositoryWriterImplementsRepositorySource(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "microservices/services/monitor/migrations/000028_existing.sql", "existing\n")
	writer, err := NewRepositoryWriter(root)
	if err != nil {
		t.Fatal(err)
	}
	var source RepositorySource = writer
	content, err := source.ReadFile("microservices/services/monitor/migrations/000028_existing.sql")
	if err != nil || string(content) != "existing\n" {
		t.Fatalf("ReadFile = %q, %v", content, err)
	}
	files, err := source.ListFiles("microservices/services/monitor/migrations/")
	if err != nil || len(files) != 1 || files[0] != "microservices/services/monitor/migrations/000028_existing.sql" {
		t.Fatalf("ListFiles = %#v, %v", files, err)
	}
}

func exportWriterTestPlan(t *testing.T, original string) GenerationPlan {
	t.Helper()
	plan := GenerationPlan{Artifacts: []PlannedArtifact{
		{
			Path: "microservices/web/src/api/assets.ts", Operation: ArtifactCreate,
			Content: "export const assets = true\n", ResultHash: contentHash("export const assets = true\n"), Status: ArtifactReady,
		},
		{
			Path: "microservices/web/src/router/index.tsx", Operation: ArtifactPatch,
			Content: "after\n", Diff: unifiedDiff("microservices/web/src/router/index.tsx", original, "after\n"),
			ExpectedHash: contentHash(original), ResultHash: contentHash("after\n"), Status: ArtifactReady,
		},
	}}
	if err := plan.normalize(); err != nil {
		t.Fatalf("normalize plan: %v", err)
	}
	return plan
}

func singleCreatePlan(t *testing.T, path, content string) GenerationPlan {
	t.Helper()
	plan := GenerationPlan{Artifacts: []PlannedArtifact{{
		Path: path, Operation: ArtifactCreate, Content: content,
		ResultHash: contentHash(content), Status: ArtifactReady,
	}}}
	if err := plan.normalize(); err != nil {
		t.Fatal(err)
	}
	return plan
}

func writeRepoFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertRepoContent(t *testing.T, root, relative, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	if string(content) != want {
		t.Fatalf("%s = %q, want %q", relative, content, want)
	}
}

func assertRepoMissing(t *testing.T, root, relative string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s exists or cannot be checked: %v", relative, err)
	}
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
