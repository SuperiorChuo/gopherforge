package agentcore

import "errors"

var (
	ErrNotFound            = errors.New("agent resource not found")
	ErrConflict            = errors.New("agent resource conflict")
	ErrValidation          = errors.New("invalid agent request")
	ErrDefinitionDisabled  = errors.New("agent definition is disabled")
	ErrNoPublishedVersion  = errors.New("agent has no published version")
	ErrIdempotencyConflict = errors.New("idempotency key belongs to another request")
	ErrLeaseLost           = errors.New("agent run lease lost")
	ErrCancelled           = errors.New("agent run cancelled")
	ErrMaxStepsExceeded    = errors.New("agent run exceeded max steps")
	ErrRunTimeout          = errors.New("agent run timeout exceeded")
	ErrToolNotFound        = errors.New("agent tool not found")
	ErrUnsafeTool          = errors.New("agent tool is not read-only")
)
