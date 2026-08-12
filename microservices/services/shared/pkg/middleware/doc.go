// Package middleware 提供跨服务共享的 gin/HTTP 中间件与服务入口辅助。
//
// 主要内容：
//   - RequestID / Recovery / RequestLogger / ErrorHandler / SecurityHeaders
//   - ServeWithGraceful：带有序优雅关闭的 HTTP 服务入口
//   - Health 探针、gzip、pprof（按需引用）
//
// 优雅关闭示例（hooks 按底层→上层顺序；HTTP 由 ServeWithGraceful 最后注册）：
//
//	return middleware.ServeWithGraceful(
//	    middleware.DefaultServerConfig(":"+port, router),
//	    middleware.ShutdownHook{Name: "tracing", Fn: shutdownTracing},
//	    middleware.ShutdownHook{Name: "workers", Fn: func(ctx context.Context) error {
//	        cancelWorkers()
//	        return nil
//	    }},
//	)
package middleware
