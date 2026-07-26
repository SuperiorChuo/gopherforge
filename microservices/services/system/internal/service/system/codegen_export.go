package system

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidGenerationPlan = errors.New("生成计划无效")

func ExportZIP(plan GenerationPlan) ([]byte, error) {
	if err := validateReadyPlan(plan); err != nil {
		return nil, err
	}

	manifest, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("生成清单序列化失败: %w", err)
	}
	manifest = append(manifest, '\n')

	var patches strings.Builder
	for _, artifact := range plan.Artifacts {
		if artifact.Operation != ArtifactPatch || artifact.Diff == "" {
			continue
		}
		if patches.Len() > 0 && !strings.HasSuffix(patches.String(), "\n\n") {
			patches.WriteByte('\n')
		}
		patches.WriteString(artifact.Diff)
		if !strings.HasSuffix(artifact.Diff, "\n") {
			patches.WriteByte('\n')
		}
	}

	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	if err := writeZIPFile(writer, "codegen-manifest.json", manifest); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writeZIPFile(writer, "integration.patch", []byte(patches.String())); err != nil {
		_ = writer.Close()
		return nil, err
	}
	for _, artifact := range plan.Artifacts {
		if artifact.Operation != ArtifactCreate {
			continue
		}
		if err := writeZIPFile(writer, artifact.Path, []byte(artifact.Content)); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("关闭 ZIP 失败: %w", err)
	}
	return output.Bytes(), nil
}

func writeZIPFile(writer *zip.Writer, name string, content []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o644)
	header.SetModTime(time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC))
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("创建 ZIP 条目 %s 失败: %w", name, err)
	}
	if _, err := entry.Write(content); err != nil {
		return fmt.Errorf("写入 ZIP 条目 %s 失败: %w", name, err)
	}
	return nil
}

func validateReadyPlan(plan GenerationPlan) error {
	if plan.Digest == "" || len(plan.Artifacts) == 0 {
		return fmt.Errorf("%w: 摘要或产物为空", ErrInvalidGenerationPlan)
	}
	for _, diagnostic := range plan.Diagnostics {
		if diagnostic.Severity == DiagnosticError {
			return fmt.Errorf("%w: %s", ErrInvalidGenerationPlan, diagnostic.Message)
		}
	}
	seenPaths := make(map[string]struct{}, len(plan.Artifacts))
	for _, artifact := range plan.Artifacts {
		if err := validateRepositoryRelativePath(artifact.Path); err != nil {
			return err
		}
		if _, exists := seenPaths[artifact.Path]; exists {
			return fmt.Errorf("%w: 产物路径 %s 重复", ErrInvalidGenerationPlan, artifact.Path)
		}
		seenPaths[artifact.Path] = struct{}{}
		if artifact.Status != ArtifactReady {
			return fmt.Errorf("%w: 产物 %s 状态为 %s", ErrInvalidGenerationPlan, artifact.Path, artifact.Status)
		}
		if artifact.Operation != ArtifactCreate && artifact.Operation != ArtifactPatch {
			return fmt.Errorf("%w: 产物 %s 操作未知", ErrInvalidGenerationPlan, artifact.Path)
		}
		if artifact.Operation == ArtifactPatch && artifact.ExpectedHash == "" {
			return fmt.Errorf("%w: 补丁 %s 缺少前置哈希", ErrInvalidGenerationPlan, artifact.Path)
		}
		if artifact.ResultHash == "" || artifact.ResultHash != contentHash(artifact.Content) {
			return fmt.Errorf("%w: 产物 %s 内容哈希不匹配", ErrInvalidGenerationPlan, artifact.Path)
		}
	}

	copyPlan := plan
	copyPlan.Artifacts = append(plan.Artifacts[:0:0], plan.Artifacts...)
	copyPlan.Diagnostics = append(plan.Diagnostics[:0:0], plan.Diagnostics...)
	copyPlan.Schemas = append(plan.Schemas[:0:0], plan.Schemas...)
	wantDigest := plan.Digest
	if err := copyPlan.normalize(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGenerationPlan, err)
	}
	if copyPlan.Digest != wantDigest {
		return fmt.Errorf("%w: 计划摘要不匹配", ErrInvalidGenerationPlan)
	}
	return nil
}
