package agentcore

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var agentKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

const maxDatabaseID = uint64(^uint64(0) >> 1)

func validDatabaseID(id uint64) bool { return id > 0 && id <= maxDatabaseID }

type Service struct {
	repository *Repository
	models     ModelPort
	tools      *ToolRegistry
}

func NewService(repository *Repository, models ModelPort, tools *ToolRegistry) *Service {
	return &Service{repository: repository, models: models, tools: tools}
}

type CreateDefinitionInput struct {
	AgentKey    string `json:"agent_key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) CreateDefinition(ctx context.Context, actorID uint64, input CreateDefinitionInput) (*AgentDefinition, error) {
	input.AgentKey = strings.TrimSpace(input.AgentKey)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !validDatabaseID(actorID) || !agentKeyPattern.MatchString(input.AgentKey) || utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 100 || utf8.RuneCountInString(input.Description) > 500 {
		return nil, ErrValidation
	}
	definition := &AgentDefinition{
		AgentKey: input.AgentKey, Name: input.Name, Description: input.Description,
		Status: DefinitionActive, CreatedBy: actorID,
	}
	if err := s.repository.CreateDefinition(ctx, definition); err != nil {
		return nil, err
	}
	return definition, nil
}

type UpdateDefinitionInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (s *Service) UpdateDefinition(ctx context.Context, id uint64, input UpdateDefinitionInput) (*AgentDefinition, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !validDatabaseID(id) || input.Name == "" || utf8.RuneCountInString(input.Name) > 100 || utf8.RuneCountInString(input.Description) > 500 ||
		(input.Status != DefinitionActive && input.Status != DefinitionDisabled) {
		return nil, ErrValidation
	}
	return s.repository.UpdateDefinition(ctx, id, input.Name, input.Description, input.Status)
}

type PublishVersionInput struct {
	Instructions   string   `json:"instructions"`
	ModelID        uint64   `json:"model_id"`
	Tools          []string `json:"tools"`
	MaxSteps       int      `json:"max_steps"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

func (s *Service) PublishVersion(ctx context.Context, definitionID, actorID uint64, input PublishVersionInput) (*AgentVersion, error) {
	if !validDatabaseID(definitionID) || !validDatabaseID(actorID) || !validDatabaseID(input.ModelID) {
		return nil, ErrValidation
	}
	definition, err := s.repository.GetDefinition(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	input.Instructions = strings.TrimSpace(input.Instructions)
	if input.MaxSteps == 0 {
		input.MaxSteps = DefaultMaxSteps
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = DefaultTimeoutSecond
	}
	if input.Instructions == "" || utf8.RuneCountInString(input.Instructions) > 20_000 || input.MaxSteps < 1 || input.MaxSteps > 32 ||
		input.TimeoutSeconds < 10 || input.TimeoutSeconds > 600 {
		return nil, ErrValidation
	}

	toolNames := make([]string, len(input.Tools))
	copy(toolNames, input.Tools)
	sort.Strings(toolNames)
	definitions, err := s.tools.Resolve(toolNames)
	if err != nil {
		return nil, err
	}
	if err := s.models.Validate(ctx, input.ModelID, len(definitions) > 0); err != nil {
		return nil, err
	}
	toolHashes := make(map[string]string, len(toolNames))
	for _, name := range toolNames {
		hash, hashErr := s.tools.SpecHash(name)
		if hashErr != nil {
			return nil, hashErr
		}
		toolHashes[name] = hash
	}
	spec := AgentSpec{
		Schema: AgentSpecSchema, Name: definition.Name, Instructions: input.Instructions,
		ModelID: input.ModelID, Tools: toolNames, MaxSteps: input.MaxSteps, TimeoutSeconds: input.TimeoutSeconds,
	}
	specHash, specJSON, err := hashJSON(spec)
	if err != nil {
		return nil, err
	}
	return s.repository.PublishVersion(ctx, definitionID, actorID, spec, specJSON, specHash, toolHashes)
}

type CreateRunInput struct {
	Input          string `json:"input"`
	IdempotencyKey string `json:"-"`
}

type CreateRunResult struct {
	Run    *AgentRun `json:"run"`
	Reused bool      `json:"reused"`
}

func (s *Service) CreateRun(ctx context.Context, definitionID, actorID uint64, input CreateRunInput) (*CreateRunResult, error) {
	input.Input = strings.TrimSpace(input.Input)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !validDatabaseID(definitionID) || !validDatabaseID(actorID) || input.Input == "" || utf8.RuneCountInString(input.Input) > 20_000 || utf8.RuneCountInString(input.IdempotencyKey) > 128 {
		return nil, ErrValidation
	}
	// 已提交的相同请求优先返回原Run；启停与最新版本仅约束真正的新请求。
	if input.IdempotencyKey != "" {
		existing, err := s.repository.findRunByIdempotencyKey(ctx, actorID, input.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			if !sameRunRequest(*existing, AgentRun{AgentDefinitionID: definitionID, RequestedBy: actorID, Input: input.Input}) {
				return nil, ErrIdempotencyConflict
			}
			return &CreateRunResult{Run: existing, Reused: true}, nil
		}
	}
	definition, err := s.repository.GetDefinition(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	if definition.Status != DefinitionActive {
		return nil, ErrDefinitionDisabled
	}
	if definition.LatestVersionID == nil || *definition.LatestVersionID == 0 {
		return nil, ErrNoPublishedVersion
	}
	version, err := s.repository.GetVersion(ctx, *definition.LatestVersionID)
	if err != nil {
		return nil, err
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = uuid.NewString()
	}
	requestHash, _, err := hashJSON(map[string]any{
		"agent_definition_id": definition.ID,
		"agent_version_id":    version.ID,
		"requested_by":        actorID,
		"input":               input.Input,
	})
	if err != nil {
		return nil, err
	}
	checkpoint := NewCheckpoint(input.Input)
	checkpoint.Messages = append([]Message{{Role: "system", Content: version.Instructions}}, checkpoint.Messages...)
	checkpointJSON, err := NewJSONValue(checkpoint)
	if err != nil {
		return nil, err
	}
	run := AgentRun{
		AgentDefinitionID: definition.ID, AgentVersionID: version.ID, RequestedBy: actorID,
		IdempotencyKey: input.IdempotencyKey, RequestSHA256: requestHash, Input: input.Input,
		Status: RunQueued, MaxSteps: version.MaxSteps, TimeoutSeconds: version.TimeoutSeconds,
		Checkpoint: checkpointJSON,
	}
	created, reused, err := s.repository.CreateRun(ctx, run)
	if err != nil {
		return nil, err
	}
	return &CreateRunResult{Run: created, Reused: reused}, nil
}

func (s *Service) ListDefinitions(ctx context.Context, page, pageSize int) ([]AgentDefinition, int64, error) {
	return s.repository.ListDefinitions(ctx, page, pageSize)
}

func (s *Service) GetDefinition(ctx context.Context, id uint64) (*AgentDefinition, error) {
	return s.repository.GetDefinition(ctx, id)
}

func (s *Service) ListVersions(ctx context.Context, definitionID uint64) ([]AgentVersion, error) {
	if _, err := s.repository.GetDefinition(ctx, definitionID); err != nil {
		return nil, err
	}
	return s.repository.ListVersions(ctx, definitionID)
}

func (s *Service) ListRuns(ctx context.Context, definitionID uint64, status string, page, pageSize int) ([]AgentRun, int64, error) {
	if status != "" && !validRunStatus(status) {
		return nil, 0, ErrValidation
	}
	return s.repository.ListRuns(ctx, definitionID, status, page, pageSize)
}

func (s *Service) GetRunDetails(ctx context.Context, id uint64) (*RunDetails, error) {
	return s.repository.GetRunDetails(ctx, id)
}

// 保留默认首批接口，HTTP详情通过after_id显式翻页。
func (s *Service) GetRunDetailsAfter(ctx context.Context, id, afterID uint64) (*RunDetails, error) {
	return s.repository.GetRunDetailsAfter(ctx, id, afterID)
}

func (s *Service) CancelRun(ctx context.Context, id uint64) (*AgentRun, error) {
	return s.repository.RequestCancel(ctx, id)
}

func (s *Service) ListEventsAfter(ctx context.Context, runID, afterID uint64, limit int) ([]RunEvent, bool, error) {
	return s.repository.ListEventsAfter(ctx, runID, afterID, limit)
}

func (s *Service) ToolDefinitions() []ToolDefinition { return s.tools.Definitions() }

func validRunStatus(status string) bool {
	switch status {
	case RunQueued, RunRunning, RunCancelling, RunSucceeded, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

func toolCallIdempotencyKey(runID uint64, modelOrdinal, callIndex int, call ToolCall) (string, string, error) {
	key, hash, _, _, err := toolCallIdentity(runID, modelOrdinal, callIndex, call)
	return key, hash, err
}

func toolCallIdentity(runID uint64, modelOrdinal, callIndex int, call ToolCall) (string, string, string, string, error) {
	canonicalArguments, err := canonicalJSON(call.Arguments)
	if err != nil {
		return "", "", "", "", fmt.Errorf("canonicalize tool arguments: %w", err)
	}
	canonicalHash := hashBytes(canonicalArguments)
	legacyHash := hashBytes(call.Arguments)
	primaryMaterial := fmt.Sprintf("%d:%d:%d:%s:%s", runID, modelOrdinal, callIndex, call.Name, canonicalHash)
	legacyMaterial := fmt.Sprintf("%d:%d:%d:%s:%s", runID, modelOrdinal, callIndex, call.Name, legacyHash)
	primaryKey := "tool_" + hashBytes([]byte(primaryMaterial))
	legacyKey := "tool_" + hashBytes([]byte(legacyMaterial))
	if primaryKey == legacyKey {
		legacyKey, legacyHash = "", ""
	}
	return primaryKey, canonicalHash, legacyKey, legacyHash, nil
}
