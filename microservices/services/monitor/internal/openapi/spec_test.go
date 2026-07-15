package openapi

import (
	"os"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSpecUsesEnglishDescriptions(t *testing.T) {
	content, err := os.ReadFile("spec.go")
	if err != nil {
		t.Fatalf("read spec.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("spec.go contains non-English source text")
	}
}

func TestNormalizeGinPathConvertsParams(t *testing.T) {
	got := NormalizeGinPath("/api/v1/monitor/jobs/:id/files/*filepath")
	want := "/api/v1/monitor/jobs/{id}/files/{filepath}"
	if got != want {
		t.Fatalf("NormalizeGinPath = %q, want %q", got, want)
	}
}

func TestBuildSpecIncludesPublicAndProtectedRoutes(t *testing.T) {
	spec := BuildSpec([]gin.RouteInfo{
		{Method: "GET", Path: "/api/v1/health/ready"},
		{Method: "GET", Path: "/api/v1/monitor/server"},
		{Method: "PUT", Path: "/api/v1/monitor/jobs/:id"},
		{Method: "GET", Path: "/uploads/*filepath"},
	}, Options{
		Title:   "Go Admin Kit API",
		Version: "test",
		Server:  "http://127.0.0.1:8081",
	})

	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("OpenAPI = %q, want 3.1.0", spec.OpenAPI)
	}
	if _, ok := spec.Paths["/uploads/{filepath}"]; ok {
		t.Fatal("BuildSpec should skip non-API static routes")
	}
	healthOp, ok := spec.Paths["/api/v1/health/ready"]["get"]
	if !ok {
		t.Fatal("BuildSpec missing public health route")
	}
	if len(healthOp.Security) != 0 {
		t.Fatalf("public health route security = %#v, want empty", healthOp.Security)
	}
	serverOp, ok := spec.Paths["/api/v1/monitor/server"]["get"]
	if !ok {
		t.Fatal("BuildSpec missing protected monitor server route")
	}
	if len(serverOp.Security) == 0 {
		t.Fatal("protected route should require BearerAuth")
	}
	jobOp, ok := spec.Paths["/api/v1/monitor/jobs/{id}"]["put"]
	if !ok {
		t.Fatal("BuildSpec missing protected job update route")
	}
	if len(jobOp.Parameters) != 1 || jobOp.Parameters[0].Name != "id" {
		t.Fatalf("path parameters = %#v, want id parameter", jobOp.Parameters)
	}
	if jobOp.RequestBody == nil {
		t.Fatal("PUT route should include a JSON request body")
	}
}

func TestBuildSpecDocumentsErrorCodeField(t *testing.T) {
	spec := BuildSpec(nil, Options{})

	apiResponse := spec.Components.Schemas["ApiResponse"]
	errorCode, ok := apiResponse.Properties["error_code"]
	if !ok {
		t.Fatal("ApiResponse schema missing error_code")
	}
	if errorCode.Type != "string" {
		t.Fatalf("error_code type = %q, want string", errorCode.Type)
	}
}

func TestBuildSpecAddsTypedCoreSchemas(t *testing.T) {
	spec := BuildSpec([]gin.RouteInfo{
		{Method: "GET", Path: "/api/v1/monitor/server"},
		{Method: "GET", Path: "/api/v1/monitor/mysql"},
		{Method: "GET", Path: "/api/v1/monitor/redis"},
		{Method: "GET", Path: "/api/v1/monitor/jobs"},
		{Method: "GET", Path: "/api/v1/monitor/jobs/health"},
		{Method: "POST", Path: "/api/v1/monitor/jobs"},
		{Method: "POST", Path: "/api/v1/monitor/jobs/:id/run"},
		{Method: "POST", Path: "/api/v1/monitor/job-logs/cleanup"},
	}, Options{})

	jobSchema := spec.Components.Schemas["SaveJobRequest"]
	if jobSchema.Properties["cron_expression"].Type != "string" {
		t.Fatalf("SaveJobRequest.cron_expression type = %q, want string", jobSchema.Properties["cron_expression"].Type)
	}
	assertRequired(t, jobSchema.Required, "name", "cron_expression", "invoke_target")

	serverOp := spec.Paths["/api/v1/monitor/server"]["get"]
	assertJSONResponseRef(t, serverOp, "#/components/schemas/ServerInfoEnvelope")

	mysqlOp := spec.Paths["/api/v1/monitor/mysql"]["get"]
	assertJSONResponseRef(t, mysqlOp, "#/components/schemas/MySQLInfoEnvelope")

	redisOp := spec.Paths["/api/v1/monitor/redis"]["get"]
	assertJSONResponseRef(t, redisOp, "#/components/schemas/RedisInfoEnvelope")

	jobListOp := spec.Paths["/api/v1/monitor/jobs"]["get"]
	assertJSONResponseRef(t, jobListOp, "#/components/schemas/JobListEnvelope")

	jobHealthOp := spec.Paths["/api/v1/monitor/jobs/health"]["get"]
	assertJSONResponseRef(t, jobHealthOp, "#/components/schemas/JobHealthEnvelope")

	createJobOp := spec.Paths["/api/v1/monitor/jobs"]["post"]
	assertJSONRequestRef(t, createJobOp, "#/components/schemas/SaveJobRequest")
	assertJSONResponseRef(t, createJobOp, "#/components/schemas/JobEnvelope")

	runJobOp := spec.Paths["/api/v1/monitor/jobs/{id}/run"]["post"]
	assertJSONResponseRef(t, runJobOp, "#/components/schemas/EmptyEnvelope")
	if runJobOp.RequestBody != nil {
		t.Fatal("run job operation should not document a request body")
	}

	cleanupOp := spec.Paths["/api/v1/monitor/job-logs/cleanup"]["post"]
	assertJSONRequestRef(t, cleanupOp, "#/components/schemas/JobLogCleanupRequest")
	assertJSONResponseRef(t, cleanupOp, "#/components/schemas/JobLogCleanupResultEnvelope")
}

func TestBuildSpecDocumentsPrometheusMetricsAsText(t *testing.T) {
	spec := BuildSpec([]gin.RouteInfo{
		{Method: "GET", Path: "/api/v1/metrics"},
	}, Options{})

	op := spec.Paths["/api/v1/metrics"]["get"]
	if len(op.Security) != 0 {
		t.Fatalf("metrics route security = %#v, want empty", op.Security)
	}
	schema, ok := op.Responses["200"].Content["text/plain"]
	if !ok {
		t.Fatal("metrics 200 response should be text/plain")
	}
	if schema.Schema.Type != "string" {
		t.Fatalf("metrics schema type = %q, want string", schema.Schema.Type)
	}
}

func assertRequired(t *testing.T, got []string, want ...string) {
	t.Helper()
	values := make(map[string]struct{}, len(got))
	for _, item := range got {
		values[item] = struct{}{}
	}
	for _, item := range want {
		if _, ok := values[item]; !ok {
			t.Fatalf("required fields = %#v, want field %q", got, item)
		}
	}
}

func assertJSONRequestRef(t *testing.T, op Operation, want string) {
	t.Helper()
	if op.RequestBody == nil {
		t.Fatalf("request body is nil, want %s", want)
	}
	got := op.RequestBody.Content["application/json"].Schema.Ref
	if got != want {
		t.Fatalf("request schema ref = %q, want %q", got, want)
	}
}

func assertJSONResponseRef(t *testing.T, op Operation, want string) {
	t.Helper()
	got := op.Responses["200"].Content["application/json"].Schema.Ref
	if got != want {
		t.Fatalf("response schema ref = %q, want %q", got, want)
	}
}
