package system

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

type ArtifactOperation string

const (
	ArtifactCreate ArtifactOperation = "create"
	ArtifactPatch  ArtifactOperation = "patch"
)

type ArtifactStatus string

const (
	ArtifactReady    ArtifactStatus = "ready"
	ArtifactConflict ArtifactStatus = "conflict"
	ArtifactInvalid  ArtifactStatus = "invalid"
)

type DiagnosticSeverity string

const (
	DiagnosticError   DiagnosticSeverity = "error"
	DiagnosticWarning DiagnosticSeverity = "warning"
)

type Diagnostic struct {
	Severity DiagnosticSeverity `json:"severity"`
	Code     string             `json:"code"`
	Message  string             `json:"message"`
	Path     string             `json:"path,omitempty"`
}

type PlannedArtifact struct {
	Path         string            `json:"path"`
	Operation    ArtifactOperation `json:"operation"`
	Content      string            `json:"content,omitempty"`
	Diff         string            `json:"diff,omitempty"`
	ExpectedHash string            `json:"expected_hash,omitempty"`
	ResultHash   string            `json:"result_hash"`
	Status       ArtifactStatus    `json:"status"`
}

type GenerationPlan struct {
	Digest      string            `json:"digest"`
	Request     GenerateRequest   `json:"request"`
	Schemas     []TableSchema     `json:"schemas"`
	Artifacts   []PlannedArtifact `json:"artifacts"`
	Diagnostics []Diagnostic      `json:"diagnostics"`
}

type RepositorySource interface {
	ReadFile(path string) ([]byte, error)
	ListFiles(prefix string) ([]string, error)
}

func (s CodegenService) BuildPlan(req GenerateRequest, source RepositorySource) (GenerationPlan, error) {
	if source == nil {
		return GenerationPlan{}, fmt.Errorf("repository source is required")
	}
	validated, err := s.ValidateRequest(req)
	if err != nil {
		return GenerationPlan{}, err
	}
	generated, err := s.generateValidated(validated)
	if err != nil {
		return GenerationPlan{}, err
	}

	plan := GenerationPlan{
		Request:     validated.Request,
		Schemas:     validated.Schemas,
		Artifacts:   make([]PlannedArtifact, 0, len(generated)+4),
		Diagnostics: make([]Diagnostic, 0),
	}
	for _, file := range generated {
		path := repositoryArtifactPath(file.Path)
		if strings.Contains(path, "/migrations/000000_codegen_") {
			path, err = allocateMigrationPath(path, source)
			if err != nil {
				return GenerationPlan{}, err
			}
		}
		content := normalizeGeneratedText(file.Content)
		artifact := PlannedArtifact{
			Path:       path,
			Operation:  ArtifactCreate,
			Content:    content,
			ResultHash: contentHash(content),
			Status:     ArtifactReady,
		}
		if _, readErr := source.ReadFile(path); readErr == nil {
			artifact.Status = ArtifactConflict
			plan.Diagnostics = append(plan.Diagnostics, Diagnostic{
				Severity: DiagnosticError,
				Code:     "create_target_exists",
				Message:  "目标文件已存在，生成器不会覆盖",
				Path:     path,
			})
		} else if !errors.Is(readErr, fs.ErrNotExist) {
			return GenerationPlan{}, fmt.Errorf("read create target %s: %w", path, readErr)
		}
		plan.Artifacts = append(plan.Artifacts, artifact)
	}

	patches, diagnostics := buildIntegrationArtifacts(validated.Request, source)
	plan.Artifacts = append(plan.Artifacts, patches...)
	plan.Diagnostics = append(plan.Diagnostics, diagnostics...)
	if err := plan.normalize(); err != nil {
		return GenerationPlan{}, err
	}
	return plan, nil
}

var migrationNamePattern = mustRe(`^([0-9]{6})_`)

func allocateMigrationPath(placeholder string, source RepositorySource) (string, error) {
	const prefix = "microservices/services/monitor/migrations/"
	files, err := source.ListFiles(prefix)
	if err != nil {
		return "", fmt.Errorf("list monitor migrations: %w", err)
	}
	maximum := 0
	for _, file := range files {
		match := migrationNamePattern.FindStringSubmatch(path.Base(file))
		if len(match) != 2 {
			continue
		}
		value, err := strconv.Atoi(match[1])
		if err != nil {
			return "", fmt.Errorf("parse migration number %q: %w", match[1], err)
		}
		if value > maximum {
			maximum = value
		}
	}
	name := strings.TrimPrefix(path.Base(placeholder), "000000_")
	return prefix + fmt.Sprintf("%06d_%s", maximum+1, name), nil
}

func (p *GenerationPlan) normalize() error {
	sort.Slice(p.Artifacts, func(i, j int) bool { return p.Artifacts[i].Path < p.Artifacts[j].Path })
	sort.Slice(p.Diagnostics, func(i, j int) bool {
		left := p.Diagnostics[i].Path + "\x00" + p.Diagnostics[i].Code + "\x00" + p.Diagnostics[i].Message
		right := p.Diagnostics[j].Path + "\x00" + p.Diagnostics[j].Code + "\x00" + p.Diagnostics[j].Message
		return left < right
	})
	p.Digest = ""
	encoded, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal normalized generation plan: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return fmt.Errorf("decode normalized generation plan: %w", err)
	}
	delete(payload, "digest")
	canonical, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal canonical generation plan: %w", err)
	}
	p.Digest = contentHashBytes(canonical)
	return nil
}

func repositoryArtifactPath(path string) string {
	path = strings.TrimPrefix(strings.ReplaceAll(path, `\`, "/"), "./")
	if strings.HasPrefix(path, "microservices/") {
		return path
	}
	if strings.HasPrefix(path, "web/") {
		return "microservices/" + path
	}
	return "microservices/services/system/" + path
}

func normalizeGeneratedText(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.ReplaceAll(content, "\r", "\n")
}

func contentHash(content string) string {
	return contentHashBytes([]byte(content))
}

func contentHashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
