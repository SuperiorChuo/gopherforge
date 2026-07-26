package system

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestExportZIPContainsManifestPatchAndCreateArtifacts(t *testing.T) {
	plan := exportWriterTestPlan(t, "before\n")
	payload, err := ExportZIP(plan)
	if err != nil {
		t.Fatalf("ExportZIP: %v", err)
	}
	files := unzipCodegenFiles(t, payload)
	for _, path := range []string{
		"codegen-manifest.json",
		"integration.patch",
		"microservices/web/src/api/assets.ts",
	} {
		if _, ok := files[path]; !ok {
			t.Fatalf("ZIP missing %q; have %v", path, sortedMapKeys(files))
		}
	}
	if !strings.Contains(files["integration.patch"], "+++ b/microservices/web/src/router/index.tsx") {
		t.Fatalf("integration.patch = %q", files["integration.patch"])
	}
	var manifest GenerationPlan
	if err := json.Unmarshal([]byte(files["codegen-manifest.json"]), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Digest != plan.Digest {
		t.Fatalf("manifest digest = %q, want %q", manifest.Digest, plan.Digest)
	}
}

func TestExportZIPPreservesEmptyDiagnosticsDigest(t *testing.T) {
	plan := exportWriterTestPlan(t, "before\n")
	plan.Diagnostics = make([]Diagnostic, 0)
	if err := plan.normalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := ExportZIP(plan); err != nil {
		t.Fatalf("ExportZIP: %v", err)
	}
}

func TestExportZIPRejectsNonReadyOrTamperedPlan(t *testing.T) {
	for name, mutate := range map[string]func(*GenerationPlan){
		"存在冲突": func(plan *GenerationPlan) { plan.Artifacts[0].Status = ArtifactConflict },
		"内容被改": func(plan *GenerationPlan) { plan.Artifacts[0].Content += "tampered" },
	} {
		t.Run(name, func(t *testing.T) {
			plan := exportWriterTestPlan(t, "before\n")
			mutate(&plan)
			if _, err := ExportZIP(plan); err == nil {
				t.Fatal("expected invalid plan error")
			}
		})
	}
}

func TestExportZIPRejectsDuplicateArtifactPath(t *testing.T) {
	plan := singleCreatePlan(t, "microservices/duplicate.go", "first\n")
	plan.Artifacts = append(plan.Artifacts, PlannedArtifact{
		Path: "microservices/duplicate.go", Operation: ArtifactCreate,
		Content: "second\n", ResultHash: contentHash("second\n"), Status: ArtifactReady,
	})
	if err := plan.normalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := ExportZIP(plan); err == nil {
		t.Fatal("expected duplicate artifact path rejection")
	}
}

func unzipCodegenFiles(t *testing.T, payload []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("open ZIP: %v", err)
	}
	files := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		files[file.Name] = string(content)
	}
	return files
}
