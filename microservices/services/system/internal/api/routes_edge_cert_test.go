package api

import (
	"os"
	"strings"
	"testing"
)

func TestEdgeCertRoutesKeepExportPermissionIndependent(t *testing.T) {
	source, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	text := string(source)
	required := []string{
		`router.GET("/.well-known/acme-challenge/:token", edgeCertAPI.ACMEChallenge)`,
		`protected.GET("/edge-certs/capabilities", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:list"), edgeCertAPI.Capabilities)`,
		`protected.POST("/edge-certs/:id/renew", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:issue"), edgeCertAPI.Renew)`,
		`protected.POST("/edge-certs/:id/deploy", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:issue"), edgeCertAPI.Deploy)`,
		`protected.POST("/edge-certs/:id/probe", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:issue"), edgeCertAPI.Probe)`,
		`protected.GET("/edge-certs/:id/tasks", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:list"), edgeCertAPI.ListTasks)`,
		`protected.GET("/edge-certs/:id/tasks/:taskId", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:list"), edgeCertAPI.GetTask)`,
		`protected.GET("/edge-certs/:id/certificate", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:list"), edgeCertAPI.Certificate)`,
		`protected.POST("/edge-certs/:id/export", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:export"), edgeCertAPI.Export)`,
		`protected.GET("/edge-certs/:id/download", edgeGuard, middleware.PermissionMiddleware("system:edge-cert:export"), edgeCertAPI.Download)`,
	}
	for _, snippet := range required {
		if !strings.Contains(text, snippet) {
			t.Errorf("route contract missing: %s", snippet)
		}
	}
	if strings.Contains(text, `PermissionMiddleware("system:edge-cert:issue"), edgeCertAPI.Export`) ||
		strings.Contains(text, `PermissionMiddleware("system:edge-cert:issue"), edgeCertAPI.Download`) {
		t.Fatal("private-key routes must not reuse issue permission")
	}
}
