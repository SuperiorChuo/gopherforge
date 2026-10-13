package agentcore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

type registeredTool struct {
	definition ToolDefinition
	execute    ToolExecutor
	hash       string
}

// ToolRegistry is server-owned; clients can grant registered names but cannot
// supply executable code, URLs or replacement schemas.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]registeredTool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]registeredTool)}
}

func (r *ToolRegistry) Register(definition ToolDefinition, execute ToolExecutor) error {
	if execute == nil || !toolNamePattern.MatchString(definition.Name) {
		return fmt.Errorf("%w: invalid tool registration", ErrValidation)
	}
	if definition.Schema == "" {
		definition.Schema = ToolSpecSchema
	}
	if definition.Schema != ToolSpecSchema || !definition.ReadOnly {
		return ErrUnsafeTool
	}
	if definition.TimeoutMS <= 0 || definition.TimeoutMS > 60_000 || !isJSONObject(definition.InputSchema) {
		return fmt.Errorf("%w: invalid tool contract", ErrValidation)
	}
	// Keep the server-owned contract independent from the caller's mutable
	// byte slice; published version hashes must not change behind the registry's
	// back.
	definition.InputSchema = definition.InputSchema.Clone()
	encoded, err := json.Marshal(definition)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[definition.Name]; exists {
		return fmt.Errorf("%w: duplicate tool %s", ErrConflict, definition.Name)
	}
	r.tools[definition.Name] = registeredTool{definition: definition, execute: execute, hash: hex.EncodeToString(digest[:])}
	return nil
}

func (r *ToolRegistry) Definitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		definition := tool.definition
		definition.InputSchema = definition.InputSchema.Clone()
		result = append(result, definition)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (r *ToolRegistry) Resolve(names []string) ([]ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]struct{}, len(names))
	result := make([]ToolDefinition, 0, len(names))
	for _, name := range names {
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate tool %s", ErrValidation, name)
		}
		tool, exists := r.tools[name]
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
		}
		seen[name] = struct{}{}
		definition := tool.definition
		definition.InputSchema = definition.InputSchema.Clone()
		result = append(result, definition)
	}
	return result, nil
}

func (r *ToolRegistry) SpecHash(name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, exists := r.tools[name]
	if !exists {
		return "", fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return tool.hash, nil
}

func isJSONObject(value JSONValue) bool {
	if !json.Valid(value) || len(value) == 0 || string(value) == "null" {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil && object != nil
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, arguments JSONValue) (result JSONValue, err error) {
	// 工具实现是服务端代码，但 panic 不应拖垮整个 Worker 进程；将其
	// 收敛为普通工具失败，交给 Run 的租约/账本路径处理。
	defer func() {
		if recover() != nil {
			result = nil
			err = fmt.Errorf("tool %s panicked", name)
		}
	}()
	r.mu.RLock()
	tool, exists := r.tools[name]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	if !json.Valid(arguments) {
		return nil, fmt.Errorf("%w: invalid JSON arguments", ErrValidation)
	}
	deadline := time.Duration(tool.definition.TimeoutMS) * time.Millisecond
	toolCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	result, err = tool.execute(toolCtx, arguments.Clone())
	if err != nil {
		return nil, err
	}
	if !json.Valid(result) {
		return nil, fmt.Errorf("tool %s returned invalid JSON", name)
	}
	if len(result) > maxToolResultBytes {
		return nil, fmt.Errorf("tool %s result exceeds 64 KiB", name)
	}
	return result, nil
}
