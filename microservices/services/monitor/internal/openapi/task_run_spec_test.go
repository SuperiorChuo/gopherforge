package openapi

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildSpecAddsTaskRunContracts(t *testing.T) {
	spec := BuildSpec([]gin.RouteInfo{
		{Method: "GET", Path: "/api/v1/monitor/task-runs"},
		{Method: "GET", Path: "/api/v1/monitor/task-runs/summary"},
		{Method: "GET", Path: "/api/v1/monitor/task-runs/:id"},
	}, Options{})

	listOp := spec.Paths["/api/v1/monitor/task-runs"]["get"]
	assertJSONResponseRef(t, listOp, "#/components/schemas/TaskRunListEnvelope")
	if got := queryParameter(t, listOp, "status").Schema.Type; got != "string" {
		t.Fatalf("task run status query type = %q, want string", got)
	}
	assertJSONResponseRefAtStatus(t, listOp, "403", "#/components/schemas/ApiResponse")
	assertJSONResponseRefAtStatus(t, listOp, "503", "#/components/schemas/ApiResponse")

	summaryOp := spec.Paths["/api/v1/monitor/task-runs/summary"]["get"]
	assertJSONResponseRef(t, summaryOp, "#/components/schemas/TaskRunSummaryEnvelope")
	assertSchemaMinimum(t, queryParameter(t, summaryOp, "window_hours").Schema, 1)
	assertJSONResponseRefAtStatus(t, summaryOp, "503", "#/components/schemas/ApiResponse")

	detailOp := spec.Paths["/api/v1/monitor/task-runs/{id}"]["get"]
	assertJSONResponseRef(t, detailOp, "#/components/schemas/TaskRunEnvelope")
	assertJSONResponseRefAtStatus(t, detailOp, "404", "#/components/schemas/ApiResponse")
	assertJSONResponseRefAtStatus(t, detailOp, "503", "#/components/schemas/ApiResponse")

	run := spec.Components.Schemas["OpsTaskRun"]
	assertRequired(t, run.Required, "id", "run_id", "task_key", "service", "source", "trigger_type", "status", "attempt", "started_at", "duration_ms", "created_at")
	assertPropertyFormat(t, run, "started_at", "date-time")
}
