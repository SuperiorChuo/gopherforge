package observability

import sharedobs "github.com/go-admin-kit/services/shared/pkg/observability"

// GinTracing is the shared OpenTelemetry middleware, re-exported
// so callers only need the local observability package import.
var GinTracing = sharedobs.GinTracing
