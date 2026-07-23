package monitor

import (
	"context"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

// Services health overview: monitor pings every service's readiness endpoint
// concurrently and returns one row per service (ok/latency/error). Targets
// default to the compose/stack DNS aliases below; MONITOR_HEALTH_EXTRA adds
// out-of-network targets ("name=url,name=url", e.g. fscc control-api on the
// host bridge).

type healthTarget struct {
	Name string
	URL  string
}

// defaultHealthTargets lists the in-cluster services (container-name DNS is
// kept stable across compose and swarm via network aliases).
func defaultHealthTargets() []healthTarget {
	base := []struct{ name, host string }{
		{"monitor", "go-admin-kit-monitor:8081"},
		{"auth", "go-admin-kit-auth:8082"},
		{"identity", "go-admin-kit-identity:8083"},
		{"system", "go-admin-kit-system:8084"},
		{"audit", "go-admin-kit-audit:8085"},
		{"file", "go-admin-kit-file:8086"},
		{"bpm", "go-admin-kit-bpm:8096"},
	}
	out := make([]healthTarget, 0, len(base))
	for _, b := range base {
		out = append(out, healthTarget{Name: b.name, URL: "http://" + b.host + "/api/v1/health/ready"})
	}
	return out
}

// extraHealthTargets parses MONITOR_HEALTH_EXTRA ("name=url,name=url").
func extraHealthTargets() []healthTarget {
	raw := strings.TrimSpace(os.Getenv("MONITOR_HEALTH_EXTRA"))
	if raw == "" {
		return nil
	}
	var out []healthTarget
	for _, pair := range strings.Split(raw, ",") {
		name, url, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || name == "" || !strings.HasPrefix(url, "http") {
			continue
		}
		out = append(out, healthTarget{Name: name, URL: url})
	}
	return out
}

// ServiceHealthRow is one service's probe result.
type ServiceHealthRow struct {
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	HTTPCode  int    `json:"http_code"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

var healthProbeClient = &http.Client{Timeout: 3 * time.Second}

// GetServicesHealth handles GET /monitor/services — concurrent readiness sweep.
func (a *ServerAPI) GetServicesHealth(c *gin.Context) {
	targets := append(defaultHealthTargets(), extraHealthTargets()...)
	rows := make([]ServiceHealthRow, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t healthTarget) {
			defer wg.Done()
			rows[i] = probeHealth(c.Request.Context(), t)
		}(i, t)
	}
	wg.Wait()
	sort.SliceStable(rows, func(i, j int) bool {
		// unhealthy first so problems surface at the top
		if rows[i].OK != rows[j].OK {
			return !rows[i].OK
		}
		return rows[i].Name < rows[j].Name
	})
	healthy := 0
	for _, r := range rows {
		if r.OK {
			healthy++
		}
	}
	response.Success(c, gin.H{
		"list": rows, "total": len(rows), "healthy": healthy,
		"checked_at": time.Now().Format(time.RFC3339),
	})
}

func probeHealth(ctx context.Context, t healthTarget) ServiceHealthRow {
	row := ServiceHealthRow{Name: t.Name}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	resp, err := healthProbeClient.Do(req)
	row.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		// strip the URL prefix Go includes; keep the cause only
		msg := err.Error()
		if idx := strings.LastIndex(msg, ": "); idx >= 0 {
			msg = msg[idx+2:]
		}
		row.Error = msg
		return row
	}
	defer resp.Body.Close()
	row.HTTPCode = resp.StatusCode
	row.OK = resp.StatusCode == http.StatusOK
	return row
}
