package middleware

import "strings"

// IsHealthProbePath reports whether path is a liveness/readiness endpoint.
// Container healthchecks poll these every 10s per service; probe traffic
// skips rate limiting (two Redis commands per hit) and, when successful,
// request logging. Failed probes still produce a log line.
func IsHealthProbePath(path string) bool {
	return strings.HasSuffix(path, "/health/live") || strings.HasSuffix(path, "/health/ready")
}
