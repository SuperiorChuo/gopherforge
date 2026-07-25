package openapi

import (
	"encoding/json"
	"os"
	"reflect"
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
	assertRequired(t, apiResponse.Required, "code", "message")
	errorCode, ok := apiResponse.Properties["error_code"]
	if !ok {
		t.Fatal("ApiResponse schema missing error_code")
	}
	if errorCode.Type != "string" {
		t.Fatalf("error_code type = %q, want string", errorCode.Type)
	}
}

func TestBuildSpecDocumentsMonitorAuthorizationAndJobValidation(t *testing.T) {
	spec := BuildSpec([]gin.RouteInfo{
		{Method: "GET", Path: "/api/v1/monitor/server"},
		{Method: "GET", Path: "/api/v1/monitor/services"},
		{Method: "GET", Path: "/api/v1/monitor/mysql"},
		{Method: "GET", Path: "/api/v1/monitor/redis"},
		{Method: "GET", Path: "/api/v1/monitor/jobs"},
		{Method: "GET", Path: "/api/v1/monitor/jobs/health"},
		{Method: "GET", Path: "/api/v1/monitor/jobs/heartbeats"},
		{Method: "POST", Path: "/api/v1/monitor/jobs"},
		{Method: "PUT", Path: "/api/v1/monitor/jobs/:id"},
		{Method: "DELETE", Path: "/api/v1/monitor/jobs/:id"},
		{Method: "POST", Path: "/api/v1/monitor/jobs/:id/start"},
		{Method: "POST", Path: "/api/v1/monitor/jobs/:id/stop"},
		{Method: "POST", Path: "/api/v1/monitor/jobs/:id/run"},
		{Method: "POST", Path: "/api/v1/monitor/job-logs/cleanup"},
	}, Options{})

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "get", path: "/api/v1/monitor/server"},
		{method: "get", path: "/api/v1/monitor/services"},
		{method: "get", path: "/api/v1/monitor/mysql"},
		{method: "get", path: "/api/v1/monitor/redis"},
		{method: "get", path: "/api/v1/monitor/jobs"},
		{method: "get", path: "/api/v1/monitor/jobs/health"},
		{method: "get", path: "/api/v1/monitor/jobs/heartbeats"},
		{method: "post", path: "/api/v1/monitor/jobs"},
		{method: "put", path: "/api/v1/monitor/jobs/{id}"},
		{method: "delete", path: "/api/v1/monitor/jobs/{id}"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/start"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/stop"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/run"},
		{method: "post", path: "/api/v1/monitor/job-logs/cleanup"},
	} {
		assertJSONResponseRefAtStatus(t, spec.Paths[route.path][route.method], "403", "#/components/schemas/ApiResponse")
	}

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "put", path: "/api/v1/monitor/jobs/{id}"},
		{method: "delete", path: "/api/v1/monitor/jobs/{id}"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/start"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/stop"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/run"},
	} {
		assertJSONResponseRefAtStatus(t, spec.Paths[route.path][route.method], "404", "#/components/schemas/ApiResponse")
	}

	cleanupOp := spec.Paths["/api/v1/monitor/job-logs/cleanup"]["post"]
	assertJSONRequestRef(t, cleanupOp, "#/components/schemas/JobLogCleanupRequest")
	if cleanupOp.RequestBody.Required {
		t.Fatal("cleanup job logs request body should be optional")
	}
	assertSchemaMinimum(t, spec.Components.Schemas["JobLogCleanupRequest"].Properties["retention_days"], 1)
	minimum := 1
	assertQueryParameter(t, cleanupOp, "retention_days", Schema{Type: "integer", Format: "int64", Minimum: &minimum})

	jobListOp := spec.Paths["/api/v1/monitor/jobs"]["get"]
	assertSchemaIntegerEnum(t, queryParameter(t, jobListOp, "status").Schema, 0, 1)
	jobHealthOp := spec.Paths["/api/v1/monitor/jobs/health"]["get"]
	assertSchemaMinimum(t, queryParameter(t, jobHealthOp, "window_hours").Schema, 1)
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

func TestBuildSpecAddsHeartbeatAndServicesContracts(t *testing.T) {
	spec := BuildSpec([]gin.RouteInfo{
		{Method: "GET", Path: "/api/v1/monitor/jobs/heartbeats"},
		{Method: "GET", Path: "/api/v1/monitor/services"},
		{Method: "GET", Path: "/api/v1/monitor/mysql"},
		{Method: "GET", Path: "/api/v1/monitor/jobs"},
		{Method: "GET", Path: "/api/v1/monitor/jobs/health"},
		{Method: "POST", Path: "/api/v1/monitor/jobs"},
		{Method: "PUT", Path: "/api/v1/monitor/jobs/:id"},
		{Method: "DELETE", Path: "/api/v1/monitor/jobs/:id"},
		{Method: "POST", Path: "/api/v1/monitor/jobs/:id/start"},
		{Method: "POST", Path: "/api/v1/monitor/jobs/:id/stop"},
		{Method: "POST", Path: "/api/v1/monitor/jobs/:id/run"},
		{Method: "POST", Path: "/api/v1/monitor/job-logs/cleanup"},
	}, Options{})

	heartbeat := spec.Components.Schemas["JobHeartbeat"]
	assertRequired(t, heartbeat.Required,
		"id", "job_key", "service", "description", "interval_sec", "last_run_at",
		"last_status", "last_error", "last_duration_ms", "runs", "fails", "updated_at", "stale",
	)
	assertPropertyType(t, heartbeat, "id", "integer")
	assertPropertyType(t, heartbeat, "job_key", "string")
	assertPropertyType(t, heartbeat, "interval_sec", "integer")
	assertPropertyFormat(t, heartbeat, "last_run_at", "date-time")
	assertPropertyType(t, heartbeat, "stale", "boolean")

	heartbeatsResponse := spec.Components.Schemas["JobHeartbeatsResponse"]
	assertRequired(t, heartbeatsResponse.Required, "list", "total")
	assertPropertyType(t, heartbeatsResponse, "list", "array")
	if heartbeatsResponse.Properties["list"].Items == nil || heartbeatsResponse.Properties["list"].Items.Ref != "#/components/schemas/JobHeartbeat" {
		t.Fatalf("JobHeartbeatsResponse.list item ref = %#v, want JobHeartbeat", heartbeatsResponse.Properties["list"].Items)
	}
	assertPropertyType(t, heartbeatsResponse, "total", "integer")

	servicesRow := spec.Components.Schemas["ServiceHealthRow"]
	assertRequired(t, servicesRow.Required, "name", "ok", "http_code", "latency_ms")
	assertNotRequired(t, servicesRow.Required, "error")
	assertPropertyType(t, servicesRow, "name", "string")
	assertPropertyType(t, servicesRow, "ok", "boolean")
	assertPropertyType(t, servicesRow, "http_code", "integer")
	assertPropertyType(t, servicesRow, "latency_ms", "integer")
	assertPropertyType(t, servicesRow, "error", "string")

	servicesResponse := spec.Components.Schemas["ServicesHealthResponse"]
	assertRequired(t, servicesResponse.Required, "list", "total", "healthy", "checked_at")
	assertPropertyType(t, servicesResponse, "list", "array")
	if servicesResponse.Properties["list"].Items == nil || servicesResponse.Properties["list"].Items.Ref != "#/components/schemas/ServiceHealthRow" {
		t.Fatalf("ServicesHealthResponse.list item ref = %#v, want ServiceHealthRow", servicesResponse.Properties["list"].Items)
	}
	assertPropertyType(t, servicesResponse, "total", "integer")
	assertPropertyType(t, servicesResponse, "healthy", "integer")
	assertPropertyFormat(t, servicesResponse, "checked_at", "date-time")

	heartbeatsOp := spec.Paths["/api/v1/monitor/jobs/heartbeats"]["get"]
	assertJSONResponseRef(t, heartbeatsOp, "#/components/schemas/JobHeartbeatsEnvelope")
	assertJSONResponseRefAtStatus(t, heartbeatsOp, "503", "#/components/schemas/ApiResponse")

	servicesOp := spec.Paths["/api/v1/monitor/services"]["get"]
	assertJSONResponseRef(t, servicesOp, "#/components/schemas/ServicesHealthEnvelope")

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "get", path: "/api/v1/monitor/mysql"},
		{method: "get", path: "/api/v1/monitor/jobs"},
		{method: "get", path: "/api/v1/monitor/jobs/health"},
		{method: "post", path: "/api/v1/monitor/jobs"},
		{method: "put", path: "/api/v1/monitor/jobs/{id}"},
		{method: "delete", path: "/api/v1/monitor/jobs/{id}"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/start"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/stop"},
		{method: "post", path: "/api/v1/monitor/jobs/{id}/run"},
		{method: "post", path: "/api/v1/monitor/job-logs/cleanup"},
	} {
		op := spec.Paths[route.path][route.method]
		assertJSONResponseRefAtStatus(t, op, "503", "#/components/schemas/ApiResponse")
	}
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

func assertNotRequired(t *testing.T, got []string, unwanted ...string) {
	t.Helper()
	values := make(map[string]struct{}, len(got))
	for _, item := range got {
		values[item] = struct{}{}
	}
	for _, item := range unwanted {
		if _, ok := values[item]; ok {
			t.Fatalf("required fields = %#v, did not want field %q", got, item)
		}
	}
}

func assertPropertyType(t *testing.T, schema Schema, name, want string) {
	t.Helper()
	property, ok := schema.Properties[name]
	if !ok {
		t.Fatalf("schema missing property %q", name)
	}
	if property.Type != want {
		t.Fatalf("property %s type = %q, want %q", name, property.Type, want)
	}
}

func assertPropertyFormat(t *testing.T, schema Schema, name, want string) {
	t.Helper()
	property, ok := schema.Properties[name]
	if !ok {
		t.Fatalf("schema missing property %q", name)
	}
	if property.Format != want {
		t.Fatalf("property %s format = %q, want %q", name, property.Format, want)
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
	assertJSONResponseRefAtStatus(t, op, "200", want)
}

func assertJSONResponseRefAtStatus(t *testing.T, op Operation, status, want string) {
	t.Helper()
	response, ok := op.Responses[status]
	if !ok {
		t.Fatalf("response %s is missing", status)
	}
	got := response.Content["application/json"].Schema.Ref
	if got != want {
		t.Fatalf("response %s schema ref = %q, want %q", status, got, want)
	}
}

func queryParameter(t *testing.T, op Operation, name string) Parameter {
	t.Helper()
	for _, parameter := range op.Parameters {
		if parameter.In == "query" && parameter.Name == name {
			return parameter
		}
	}
	t.Fatalf("query parameter %q is missing", name)
	return Parameter{}
}

func assertQueryParameter(t *testing.T, op Operation, name string, want Schema) {
	t.Helper()
	got := queryParameter(t, op, name).Schema
	if got.Type != want.Type || got.Format != want.Format {
		t.Fatalf("query parameter %s schema = %#v, want type %q format %q", name, got, want.Type, want.Format)
	}
	if (got.Minimum == nil) != (want.Minimum == nil) || (got.Minimum != nil && *got.Minimum != *want.Minimum) {
		t.Fatalf("query parameter %s minimum = %#v, want %#v", name, got.Minimum, want.Minimum)
	}
}

func assertSchemaIntegerEnum(t *testing.T, schema Schema, want ...int) {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var document struct {
		Enum []int `json:"enum"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal schema enum: %v", err)
	}
	if !reflect.DeepEqual(document.Enum, want) {
		t.Fatalf("schema enum = %#v, want %#v", document.Enum, want)
	}
}

func assertSchemaMinimum(t *testing.T, schema Schema, want int) {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var document struct {
		Minimum *int `json:"minimum"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal schema minimum: %v", err)
	}
	if document.Minimum == nil || *document.Minimum != want {
		t.Fatalf("schema minimum = %#v, want %d", document.Minimum, want)
	}
}
