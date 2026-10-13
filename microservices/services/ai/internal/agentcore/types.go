// Package agentcore implements the runtime-neutral Agent Kernel domain.
package agentcore

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	AgentSpecSchema      = "agent-spec/v1"
	ToolSpecSchema       = "tool-spec/v1"
	CheckpointSchema     = "agent-checkpoint/v1"
	RunEventSchema       = "run-event/v1"
	DefaultMaxSteps      = 8
	DefaultTimeoutSecond = 120

	maxModelToolCalls     = 8
	maxToolArgumentsBytes = 16 * 1024
	maxModelContentBytes  = 128 * 1024
	maxCheckpointBytes    = 512 * 1024
	maxToolResultBytes    = 64 * 1024
)

const (
	DefinitionActive   = "active"
	DefinitionDisabled = "disabled"

	RunQueued     = "queued"
	RunRunning    = "running"
	RunCancelling = "cancelling"
	RunSucceeded  = "succeeded"
	RunFailed     = "failed"
	RunCancelled  = "cancelled"

	StepRunning   = "running"
	StepSucceeded = "succeeded"
	StepFailed    = "failed"
	StepCancelled = "cancelled"

	StepKindModel = "model"
	StepKindTool  = "tool"
)

// JSONValue keeps JSON fields portable across PostgreSQL and SQLite tests.
type JSONValue json.RawMessage

func NewJSONValue(value any) (JSONValue, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return JSONValue(encoded), nil
}

func MustJSONValue(value any) JSONValue {
	encoded, err := NewJSONValue(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func (j JSONValue) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("{}"), nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid JSON value")
	}
	return j, nil
}

func (j *JSONValue) UnmarshalJSON(data []byte) error {
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON value")
	}
	*j = append((*j)[:0], data...)
	return nil
}

func (j JSONValue) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "{}", nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid JSON value")
	}
	return string(j), nil
}

func (j *JSONValue) Scan(value any) error {
	switch typed := value.(type) {
	case nil:
		*j = nil
		return nil
	case []byte:
		*j = append((*j)[:0], typed...)
	case string:
		*j = append((*j)[:0], typed...)
	default:
		return fmt.Errorf("scan JSON value from %T", value)
	}
	if len(*j) > 0 && !json.Valid(*j) {
		return fmt.Errorf("database contains invalid JSON")
	}
	return nil
}

func (j JSONValue) Clone() JSONValue { return append(JSONValue(nil), j...) }

// AgentDefinition is the mutable identity of an Agent.
type AgentDefinition struct {
	ID              uint64         `gorm:"primaryKey" json:"id"`
	TenantID        uint           `gorm:"column:tenant_id;not null;index" json:"tenant_id"`
	AgentKey        string         `gorm:"column:agent_key;size:64;not null" json:"agent_key"`
	Name            string         `gorm:"column:name;size:100;not null" json:"name"`
	Description     string         `gorm:"column:description;size:500;not null" json:"description"`
	Status          string         `gorm:"column:status;size:16;not null" json:"status"`
	LatestVersionID *uint64        `gorm:"column:latest_version_id" json:"latest_version_id,omitempty"`
	CreatedBy       uint64         `gorm:"column:created_by;not null" json:"created_by"`
	CreatedAt       time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (AgentDefinition) TableName() string { return "agent_definitions" }

// AgentSpec is the immutable, runtime-neutral publication contract.
type AgentSpec struct {
	Schema         string   `json:"schema"`
	Name           string   `json:"name"`
	Instructions   string   `json:"instructions"`
	ModelID        uint64   `json:"model_id"`
	Tools          []string `json:"tools"`
	MaxSteps       int      `json:"max_steps"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type AgentVersion struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint      `gorm:"column:tenant_id;not null;index" json:"tenant_id"`
	AgentDefinitionID uint64    `gorm:"column:agent_definition_id;not null;index" json:"agent_definition_id"`
	Version           int       `gorm:"column:version;not null" json:"version"`
	SchemaVersion     string    `gorm:"column:schema_version;size:32;not null" json:"schema_version"`
	ModelID           uint64    `gorm:"column:model_id;not null" json:"model_id"`
	Instructions      string    `gorm:"column:instructions;type:text;not null" json:"instructions"`
	Tools             JSONValue `gorm:"column:tools;type:jsonb;not null" json:"tools"`
	MaxSteps          int       `gorm:"column:max_steps;not null" json:"max_steps"`
	TimeoutSeconds    int       `gorm:"column:timeout_seconds;not null" json:"timeout_seconds"`
	Spec              JSONValue `gorm:"column:spec;type:jsonb;not null" json:"spec"`
	SpecSHA256        string    `gorm:"column:spec_sha256;size:64;not null" json:"spec_sha256"`
	CreatedBy         uint64    `gorm:"column:created_by;not null" json:"created_by"`
	CreatedAt         time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AgentVersion) TableName() string { return "agent_versions" }

func (v AgentVersion) ToolNames() ([]string, error) {
	var names []string
	if err := json.Unmarshal(v.Tools, &names); err != nil {
		return nil, err
	}
	if names == nil {
		return nil, fmt.Errorf("version tools must be an array")
	}
	return names, nil
}

func (v AgentVersion) ValidateIntegrity() error {
	var spec AgentSpec
	if err := json.Unmarshal(v.Spec, &spec); err != nil {
		return fmt.Errorf("invalid version spec: %w", err)
	}
	if spec.Schema != AgentSpecSchema || v.SchemaVersion != AgentSpecSchema || spec.Tools == nil {
		return fmt.Errorf("invalid version spec schema")
	}
	if spec.ModelID != v.ModelID || spec.Instructions != v.Instructions || spec.MaxSteps != v.MaxSteps || spec.TimeoutSeconds != v.TimeoutSeconds {
		return fmt.Errorf("version spec columns do not match")
	}
	names, err := v.ToolNames()
	if err != nil || len(names) != len(spec.Tools) {
		return fmt.Errorf("version spec tools do not match")
	}
	for index := range names {
		if names[index] != spec.Tools[index] {
			return fmt.Errorf("version spec tools do not match")
		}
	}
	hash, _, err := hashJSON(spec)
	if err != nil {
		return fmt.Errorf("hash version spec: %w", err)
	}
	if hash != v.SpecSHA256 {
		return fmt.Errorf("version spec hash does not match")
	}
	return nil
}

type AgentVersionTool struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	TenantID       uint      `gorm:"column:tenant_id;not null;index" json:"tenant_id"`
	AgentVersionID uint64    `gorm:"column:agent_version_id;not null;index" json:"agent_version_id"`
	ToolName       string    `gorm:"column:tool_name;size:64;not null" json:"tool_name"`
	ToolSpecSHA256 string    `gorm:"column:tool_spec_sha256;size:64;not null" json:"tool_spec_sha256"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AgentVersionTool) TableName() string { return "agent_version_tools" }

// Checkpoint is committed together with every completed step.
type Checkpoint struct {
	Schema           string     `json:"schema"`
	NextOrdinal      int        `json:"next_ordinal"`
	Messages         []Message  `json:"messages"`
	PendingToolCalls []ToolCall `json:"pending_tool_calls,omitempty"`
	PendingToolIndex int        `json:"pending_tool_index,omitempty"`
	PendingModelStep int        `json:"pending_model_step,omitempty"`
}

func NewCheckpoint(input string) Checkpoint {
	return Checkpoint{Schema: CheckpointSchema, NextOrdinal: 1, Messages: []Message{{Role: "user", Content: input}}}
}

type AgentRun struct {
	ID                uint64     `gorm:"primaryKey" json:"id"`
	TenantID          uint       `gorm:"column:tenant_id;not null;index" json:"tenant_id"`
	AgentDefinitionID uint64     `gorm:"column:agent_definition_id;not null;index" json:"agent_definition_id"`
	AgentVersionID    uint64     `gorm:"column:agent_version_id;not null;index" json:"agent_version_id"`
	RequestedBy       uint64     `gorm:"column:requested_by;not null" json:"requested_by"`
	IdempotencyKey    string     `gorm:"column:idempotency_key;size:128;not null" json:"idempotency_key"`
	RequestSHA256     string     `gorm:"column:request_sha256;size:64;not null" json:"request_sha256"`
	Input             string     `gorm:"column:input;type:text;not null" json:"input"`
	Status            string     `gorm:"column:status;size:16;not null" json:"status"`
	Attempt           int        `gorm:"column:attempt;not null" json:"attempt"`
	MaxSteps          int        `gorm:"column:max_steps;not null" json:"max_steps"`
	TimeoutSeconds    int        `gorm:"column:timeout_seconds;not null" json:"timeout_seconds"`
	Checkpoint        JSONValue  `gorm:"column:checkpoint;type:jsonb;not null" json:"checkpoint"`
	Output            string     `gorm:"column:output;type:text;not null" json:"output"`
	ErrorCode         string     `gorm:"column:error_code;size:64;not null" json:"error_code"`
	ErrorMessage      string     `gorm:"column:error_message;size:500;not null" json:"error_message"`
	LeaseOwner        string     `gorm:"column:lease_owner;size:128;not null" json:"-"`
	LeaseExpiresAt    *time.Time `gorm:"column:lease_expires_at" json:"lease_expires_at,omitempty"`
	HeartbeatAt       *time.Time `gorm:"column:heartbeat_at" json:"heartbeat_at,omitempty"`
	CancelRequestedAt *time.Time `gorm:"column:cancel_requested_at" json:"cancel_requested_at,omitempty"`
	StartedAt         *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt        *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	CreatedAt         time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (AgentRun) TableName() string { return "agent_runs" }

func (r AgentRun) IsTerminal() bool {
	return r.Status == RunSucceeded || r.Status == RunFailed || r.Status == RunCancelled
}

func (r AgentRun) DecodeCheckpoint() (Checkpoint, error) {
	var checkpoint Checkpoint
	if len(r.Checkpoint) == 0 || len(r.Checkpoint) > maxCheckpointBytes {
		return checkpoint, fmt.Errorf("checkpoint exceeds limit")
	}
	if err := json.Unmarshal(r.Checkpoint, &checkpoint); err != nil {
		return checkpoint, err
	}
	if checkpoint.Schema != CheckpointSchema || checkpoint.NextOrdinal < 1 {
		return checkpoint, fmt.Errorf("unsupported checkpoint")
	}
	if err := validateCheckpoint(checkpoint); err != nil {
		return checkpoint, err
	}
	return checkpoint, nil
}

func validateCheckpoint(checkpoint Checkpoint) error {
	if len(checkpoint.Messages) == 0 {
		return fmt.Errorf("checkpoint has no messages")
	}
	if checkpoint.PendingToolIndex < 0 || checkpoint.PendingToolIndex > len(checkpoint.PendingToolCalls) {
		return fmt.Errorf("checkpoint has invalid pending tool index")
	}
	if checkpoint.PendingModelStep < 0 || checkpoint.PendingModelStep >= checkpoint.NextOrdinal {
		return fmt.Errorf("checkpoint has invalid pending model step")
	}
	if len(checkpoint.PendingToolCalls) == 0 {
		if checkpoint.PendingToolIndex != 0 || checkpoint.PendingModelStep != 0 {
			return fmt.Errorf("checkpoint has stale pending tool state")
		}
	} else {
		// An exhausted pending list must have been cleared in the same
		// transaction as its final tool result; accepting it would skip work.
		if checkpoint.PendingToolIndex >= len(checkpoint.PendingToolCalls) || checkpoint.PendingModelStep == 0 {
			return fmt.Errorf("checkpoint has incomplete pending tool state")
		}
		if len(checkpoint.PendingToolCalls) > maxModelToolCalls {
			return fmt.Errorf("checkpoint has too many pending tool calls")
		}
		if err := validateToolCalls(checkpoint.PendingToolCalls); err != nil {
			return err
		}
	}
	for _, message := range checkpoint.Messages {
		switch message.Role {
		case "system", "user":
			if message.ToolCallID != "" || len(message.ToolCalls) != 0 || len(message.Content) > maxModelContentBytes {
				return fmt.Errorf("checkpoint contains an invalid message")
			}
		case "assistant":
			if message.ToolCallID != "" || len(message.Content) > maxModelContentBytes || len(message.ToolCalls) > maxModelToolCalls {
				return fmt.Errorf("checkpoint contains an invalid assistant message")
			}
			if err := validateToolCalls(message.ToolCalls); err != nil {
				return err
			}
		case "tool":
			if message.ToolCallID == "" || len(message.ToolCallID) > 128 || len(message.Content) > maxToolResultBytes || len(message.ToolCalls) != 0 {
				return fmt.Errorf("checkpoint contains an invalid tool message")
			}
		default:
			return fmt.Errorf("checkpoint contains unsupported message role")
		}
	}
	return nil
}

func validateToolCalls(calls []ToolCall) error {
	seenIDs := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		if call.ID == "" || len(call.ID) > 128 || !toolNamePattern.MatchString(call.Name) || len(call.Arguments) == 0 || len(call.Arguments) > maxToolArgumentsBytes || !json.Valid(call.Arguments) {
			return fmt.Errorf("checkpoint contains an invalid tool call")
		}
		if _, duplicate := seenIDs[call.ID]; duplicate {
			return fmt.Errorf("checkpoint contains duplicate tool call ids")
		}
		seenIDs[call.ID] = struct{}{}
	}
	return nil
}

type AgentStep struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	TenantID     uint       `gorm:"column:tenant_id;not null;index" json:"tenant_id"`
	RunID        uint64     `gorm:"column:run_id;not null;index" json:"run_id"`
	Ordinal      int        `gorm:"column:ordinal;not null" json:"ordinal"`
	Kind         string     `gorm:"column:kind;size:16;not null" json:"kind"`
	Status       string     `gorm:"column:status;size:16;not null" json:"status"`
	Name         string     `gorm:"column:name;size:100;not null" json:"name"`
	Request      JSONValue  `gorm:"column:request;type:jsonb;not null" json:"request"`
	Response     JSONValue  `gorm:"column:response;type:jsonb;not null" json:"response"`
	InputTokens  int        `gorm:"column:input_tokens;not null" json:"input_tokens"`
	OutputTokens int        `gorm:"column:output_tokens;not null" json:"output_tokens"`
	StopReason   string     `gorm:"column:stop_reason;size:64;not null" json:"stop_reason"`
	ErrorCode    string     `gorm:"column:error_code;size:64;not null" json:"error_code"`
	ErrorMessage string     `gorm:"column:error_message;size:500;not null" json:"error_message"`
	StartedAt    time.Time  `gorm:"column:started_at" json:"started_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (AgentStep) TableName() string { return "agent_steps" }

type AgentToolCall struct {
	ID              uint64     `gorm:"primaryKey" json:"id"`
	TenantID        uint       `gorm:"column:tenant_id;not null;index" json:"tenant_id"`
	RunID           uint64     `gorm:"column:run_id;not null;index" json:"run_id"`
	StepID          uint64     `gorm:"column:step_id;not null" json:"step_id"`
	ProviderCallID  string     `gorm:"column:provider_call_id;size:128;not null" json:"provider_call_id"`
	IdempotencyKey  string     `gorm:"column:idempotency_key;size:128;not null" json:"idempotency_key"`
	ToolName        string     `gorm:"column:tool_name;size:64;not null" json:"tool_name"`
	Arguments       JSONValue  `gorm:"column:arguments;type:jsonb;not null" json:"arguments"`
	ArgumentsSHA256 string     `gorm:"column:arguments_sha256;size:64;not null" json:"arguments_sha256"`
	Status          string     `gorm:"column:status;size:16;not null" json:"status"`
	Result          JSONValue  `gorm:"column:result;type:jsonb;not null" json:"result"`
	ErrorCode       string     `gorm:"column:error_code;size:64;not null" json:"error_code"`
	ErrorMessage    string     `gorm:"column:error_message;size:500;not null" json:"error_message"`
	Attempt         int        `gorm:"column:attempt;not null" json:"attempt"`
	StartedAt       time.Time  `gorm:"column:started_at" json:"started_at"`
	FinishedAt      *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (AgentToolCall) TableName() string { return "agent_tool_calls" }

type RunEvent struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	TenantID      uint      `gorm:"column:tenant_id;not null;index" json:"tenant_id"`
	RunID         uint64    `gorm:"column:run_id;not null;index" json:"run_id"`
	StepID        *uint64   `gorm:"column:step_id" json:"step_id,omitempty"`
	SchemaVersion string    `gorm:"column:schema_version;size:32;not null" json:"schema"`
	EventType     string    `gorm:"column:event_type;size:64;not null" json:"type"`
	Data          JSONValue `gorm:"column:data;type:jsonb;not null" json:"data"`
	OccurredAt    time.Time `gorm:"column:occurred_at" json:"occurred_at"`
}

func (RunEvent) TableName() string { return "agent_run_events" }

type ToolCall struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Arguments JSONValue `json:"arguments"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ModelRequest struct {
	ModelID  uint64           `json:"model_id"`
	ActorID  uint64           `json:"actor_id"`
	RunID    uint64           `json:"run_id"`
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools"`
}

type ModelResponse struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls"`
	Usage      Usage      `json:"usage"`
	StopReason string     `json:"stop_reason"`
}

// ModelPort hides provider-specific APIs from the Kernel.
type ModelPort interface {
	Validate(ctx context.Context, modelID uint64, requiresTools bool) error
	Complete(ctx context.Context, request ModelRequest) (ModelResponse, error)
}

type ToolDefinition struct {
	Schema      string    `json:"schema"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ReadOnly    bool      `json:"read_only"`
	TimeoutMS   int       `json:"timeout_ms"`
	InputSchema JSONValue `json:"input_schema"`
}

type ToolExecutor func(ctx context.Context, arguments JSONValue) (JSONValue, error)

type RunDetails struct {
	Run        AgentRun        `json:"run"`
	Definition AgentDefinition `json:"definition"`
	Version    AgentVersion    `json:"version"`
	Steps      []AgentStep     `json:"steps"`
	ToolCalls  []AgentToolCall `json:"tool_calls"`
	Events     []RunEvent      `json:"events"`
	// 事件按ID升序每批最多200条；after_id=EventsNextCursor读取下一批。
	EventsHasMore    bool   `json:"events_has_more"`
	EventsNextCursor uint64 `json:"events_next_cursor"`
}
