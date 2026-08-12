package observability

import sharedobs "github.com/go-admin-kit/services/shared/pkg/observability"

// GinTracing 是共享的 OpenTelemetry 中间件，重新导出
// 以便调用方只需导入本地的 observability 包。
var GinTracing = sharedobs.GinTracing
