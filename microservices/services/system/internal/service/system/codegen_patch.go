package system

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type integrationPatchSpec struct {
	path  string
	apply func(string) (string, error)
}

func buildIntegrationArtifacts(req GenerateRequest, source RepositorySource) ([]PlannedArtifact, []Diagnostic) {
	specs := integrationPatchSpecs(req)
	artifacts := make([]PlannedArtifact, 0, len(specs))
	diagnostics := make([]Diagnostic, 0)
	for _, spec := range specs {
		originalBytes, err := source.ReadFile(spec.path)
		if err != nil {
			message := "读取接入文件失败"
			code := "integration_file_unreadable"
			if errors.Is(err, fs.ErrNotExist) {
				message = "接入文件不存在"
				code = "integration_file_missing"
			}
			artifacts = append(artifacts, PlannedArtifact{
				Path: spec.path, Operation: ArtifactPatch, Status: ArtifactInvalid,
			})
			diagnostics = append(diagnostics, Diagnostic{
				Severity: DiagnosticError, Code: code, Message: message, Path: spec.path,
			})
			continue
		}

		original := normalizeGeneratedText(string(originalBytes))
		artifact := PlannedArtifact{
			Path:         spec.path,
			Operation:    ArtifactPatch,
			ExpectedHash: contentHashBytes(originalBytes),
			ResultHash:   contentHash(original),
			Status:       ArtifactInvalid,
		}
		patched, err := spec.apply(original)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: DiagnosticError,
				Code:     "integration_anchor_invalid",
				Message:  err.Error(),
				Path:     spec.path,
			})
			artifacts = append(artifacts, artifact)
			continue
		}
		patched = normalizeGeneratedText(patched)
		artifact.Content = patched
		artifact.Diff = unifiedDiff(spec.path, original, patched)
		artifact.ResultHash = contentHash(patched)
		artifact.Status = ArtifactReady
		artifacts = append(artifacts, artifact)
	}
	return artifacts, diagnostics
}

func integrationPatchSpecs(req GenerateRequest) []integrationPatchSpec {
	moduleType := exportedName(req.Module)
	moduleVar := camelName(req.Module)
	title := strconv.Quote(req.Title)
	codegenDownloadRoute := "\t\t\tprotected.POST(\"/codegen/download\", middleware.PermissionMiddleware(\"system:codegen:generate\"), codegenAPI.Download)"
	codegenMenuLine := "\t{ID: 112, Name: \"codegen\", Title: \"代码生成\", Icon: \"code\", Path: \"/system/codegen\", Component: \"system/codegen/index\", ParentID: 10, Sort: 15, Status: 1, Hidden: 0, Permission: \"system:codegen:list\"},"

	specs := []integrationPatchSpec{
		{
			path: "microservices/services/system/internal/api/routes.go",
			apply: func(source string) (string, error) {
				var err error
				source, err = patchAfterUniqueAnchor(source,
					"\tvar codegenAPI *system.CodegenAPI",
					fmt.Sprintf("\n\tvar %sAPI *system.%sAPI", moduleVar, moduleType))
				if err != nil {
					return "", err
				}
				source, err = patchAfterUniqueAnchor(source,
					"\t\tcodegenAPI = system.NewCodegenAPIWithOptions(systemsvc.NewCodegenServiceWithDB(deps.DB), codegenAPIOptions())",
					fmt.Sprintf("\n\t\t%sAPI = system.New%sAPI(deps.DB)", moduleVar, moduleType))
				if err != nil {
					return "", err
				}
				return patchAfterUniqueAnchor(source, codegenDownloadRoute,
					fmt.Sprintf("\n\n\t\t\tif %sAPI != nil {\n\t\t\t\t%sAPI.RegisterRoutes(protected)\n\t\t\t}", moduleVar, moduleVar))
			},
		},
		{
			path: "microservices/web/src/router/index.tsx",
			apply: func(source string) (string, error) {
				return patchAfterUniqueAnchor(source,
					"  { path: 'system/codegen', element: lazyLoad(() => import('@/pages/system/codegen')) },",
					fmt.Sprintf("\n  { path: 'system/%s', element: lazyLoad(() => import('@/pages/system/%s')) },", req.Module, req.Module))
			},
		},
		{
			path: "microservices/web/src/layouts/menu-defs.tsx",
			apply: func(source string) (string, error) {
				var err error
				source, err = patchAfterUniqueAnchor(source,
					"      { label: '代码生成', key: '/system/codegen', icon: <CodeOutlined /> },",
					fmt.Sprintf("\n      { label: %s, key: '/system/%s', icon: <DatabaseOutlined /> },", title, req.Module))
				if err != nil {
					return "", err
				}
				return patchAfterUniqueAnchor(source,
					"  '/system/codegen': '代码生成',",
					fmt.Sprintf("\n  '/system/%s': %s,", req.Module, title))
			},
		},
		{
			path: "microservices/services/system/internal/service/system/menu_seed.go",
			apply: func(source string) (string, error) {
				nextID, err := nextMenuSeedID(source)
				if err != nil {
					return "", err
				}
				line := fmt.Sprintf("\n\t{ID: %d, Name: %q, Title: %s, Icon: \"data-base\", Path: \"/system/%s\", Component: \"system/%s/index\", ParentID: 10, Sort: 100, Status: 1, Hidden: 0, Permission: \"system:%s:list\"},",
					nextID, req.Module, title, req.Module, req.Module, req.Module)
				return patchAfterUniqueAnchor(source, codegenMenuLine, line)
			},
		},
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].path < specs[j].path })
	return specs
}

func patchAfterUniqueAnchor(source, anchor, insertion string) (string, error) {
	if count := strings.Count(source, anchor); count != 1 {
		return "", fmt.Errorf("接入锚点数量不是 1（实际 %d）: %q", count, anchor)
	}
	return strings.Replace(source, anchor, anchor+insertion, 1), nil
}

var menuSeedIDPattern = regexp.MustCompile(`\bID:\s*([0-9]+)`)

func nextMenuSeedID(source string) (int64, error) {
	matches := menuSeedIDPattern.FindAllStringSubmatch(source, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("菜单种子中没有可用的 ID")
	}
	var maximum int64
	for _, match := range matches {
		value, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("解析菜单 ID %q: %w", match[1], err)
		}
		if value > maximum {
			maximum = value
		}
	}
	return maximum + 1, nil
}

func unifiedDiff(path, before, after string) string {
	if before == after {
		return ""
	}
	oldLines := diffLines(before)
	newLines := diffLines(after)
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}
	contextStart := prefix - min(prefix, 3)
	contextSuffix := min(suffix, 3)
	oldEnd := len(oldLines) - suffix
	newEnd := len(newLines) - suffix
	oldCount := oldEnd - contextStart + contextSuffix
	newCount := newEnd - contextStart + contextSuffix

	var result strings.Builder
	fmt.Fprintf(&result, "--- a/%s\n+++ b/%s\n", path, path)
	fmt.Fprintf(&result, "@@ -%d,%d +%d,%d @@\n", contextStart+1, oldCount, contextStart+1, newCount)
	for _, line := range oldLines[contextStart:prefix] {
		fmt.Fprintf(&result, " %s\n", line)
	}
	for _, line := range oldLines[prefix:oldEnd] {
		fmt.Fprintf(&result, "-%s\n", line)
	}
	for _, line := range newLines[prefix:newEnd] {
		fmt.Fprintf(&result, "+%s\n", line)
	}
	for _, line := range oldLines[oldEnd : oldEnd+contextSuffix] {
		fmt.Fprintf(&result, " %s\n", line)
	}
	return result.String()
}

func diffLines(content string) []string {
	content = normalizeGeneratedText(content)
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}
