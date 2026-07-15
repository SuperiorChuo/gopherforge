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
	got := NormalizeGinPath("/api/v1/users/:id/files/*filepath")
	want := "/api/v1/users/{id}/files/{filepath}"
	if got != want {
		t.Fatalf("NormalizeGinPath = %q, want %q", got, want)
	}
}

func TestBuildSpecIncludesPublicAndProtectedRoutes(t *testing.T) {
	spec := BuildSpec([]gin.RouteInfo{
		{Method: "GET", Path: "/api/v1/health/ready"},
		{Method: "POST", Path: "/api/v1/login"},
		{Method: "GET", Path: "/api/v1/oauth/github/login"},
		{Method: "POST", Path: "/api/v1/oauth/bind"},
		{Method: "GET", Path: "/api/v1/users/:id"},
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
	if _, ok := spec.Paths["/api/v1/health/ready"]["get"]; !ok {
		t.Fatal("BuildSpec missing public health route")
	}
	usersOp, ok := spec.Paths["/api/v1/users/{id}"]["get"]
	if !ok {
		t.Fatal("BuildSpec missing protected user detail route")
	}
	if len(usersOp.Security) == 0 {
		t.Fatal("protected route should require BearerAuth")
	}
	if len(usersOp.Parameters) != 1 || usersOp.Parameters[0].Name != "id" {
		t.Fatalf("path parameters = %#v, want id parameter", usersOp.Parameters)
	}

	loginOp := spec.Paths["/api/v1/login"]["post"]
	if len(loginOp.Security) != 0 {
		t.Fatalf("public login route security = %#v, want empty", loginOp.Security)
	}
	if loginOp.RequestBody == nil {
		t.Fatal("POST route should include a generic JSON request body")
	}
	oauthLoginOp := spec.Paths["/api/v1/oauth/github/login"]["get"]
	if len(oauthLoginOp.Security) != 0 {
		t.Fatalf("oauth login security = %#v, want empty", oauthLoginOp.Security)
	}
	oauthBindOp := spec.Paths["/api/v1/oauth/bind"]["post"]
	if len(oauthBindOp.Security) == 0 {
		t.Fatal("oauth bind route should require BearerAuth")
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
		{Method: "POST", Path: "/api/v1/login"},
		{Method: "GET", Path: "/api/v1/oauth/github/login"},
		{Method: "GET", Path: "/api/v1/oauth/github/callback"},
		{Method: "GET", Path: "/api/v1/user/me"},
		{Method: "POST", Path: "/api/v1/oauth/bind"},
		{Method: "POST", Path: "/api/v1/oauth/unbind"},
		{Method: "GET", Path: "/api/v1/user/menus"},
		{Method: "GET", Path: "/api/v1/users"},
		{Method: "POST", Path: "/api/v1/users"},
		{Method: "PUT", Path: "/api/v1/users/:id"},
		{Method: "POST", Path: "/api/v1/users/:id/roles"},
		{Method: "GET", Path: "/api/v1/roles"},
		{Method: "POST", Path: "/api/v1/roles"},
		{Method: "POST", Path: "/api/v1/roles/:id/permissions"},
		{Method: "GET", Path: "/api/v1/menus/tree"},
		{Method: "POST", Path: "/api/v1/menus"},
		{Method: "POST", Path: "/api/v1/files/upload"},
		{Method: "GET", Path: "/api/v1/files/hash/check"},
		{Method: "GET", Path: "/api/v1/files/stats"},
		{Method: "GET", Path: "/api/v1/files/:id/download"},
		{Method: "GET", Path: "/api/v1/files/:id/preview"},
		{Method: "POST", Path: "/api/v1/monitor/jobs/:id/run"},
	}, Options{})

	loginSchema := spec.Components.Schemas["LoginRequest"]
	if loginSchema.Properties["username"].Type != "string" {
		t.Fatalf("LoginRequest.username type = %q, want string", loginSchema.Properties["username"].Type)
	}
	assertRequired(t, loginSchema.Required, "username", "password", "captcha_id", "captcha_code")

	loginOp := spec.Paths["/api/v1/login"]["post"]
	assertJSONRequestRef(t, loginOp, "#/components/schemas/LoginRequest")
	assertJSONResponseRef(t, loginOp, "#/components/schemas/LoginResponseEnvelope")

	oauthLoginOp := spec.Paths["/api/v1/oauth/github/login"]["get"]
	assertStatusResponse(t, oauthLoginOp, "302")
	assertStatusResponse(t, oauthLoginOp, "503")
	oauthCallbackOp := spec.Paths["/api/v1/oauth/github/callback"]["get"]
	assertStatusResponse(t, oauthCallbackOp, "503")

	userMeOp := spec.Paths["/api/v1/user/me"]["get"]
	assertJSONResponseRef(t, userMeOp, "#/components/schemas/UserEnvelope")

	oauthBindOp := spec.Paths["/api/v1/oauth/bind"]["post"]
	assertJSONRequestRef(t, oauthBindOp, "#/components/schemas/OAuthBindRequest")
	assertJSONResponseRef(t, oauthBindOp, "#/components/schemas/EmptyEnvelope")
	assertStatusResponse(t, oauthBindOp, "409")
	assertStatusResponse(t, oauthBindOp, "503")

	oauthUnbindOp := spec.Paths["/api/v1/oauth/unbind"]["post"]
	assertJSONRequestRef(t, oauthUnbindOp, "#/components/schemas/OAuthUnbindRequest")
	assertJSONResponseRef(t, oauthUnbindOp, "#/components/schemas/EmptyEnvelope")
	assertStatusResponse(t, oauthUnbindOp, "404")
	assertStatusResponse(t, oauthUnbindOp, "503")

	createRoleOp := spec.Paths["/api/v1/roles"]["post"]
	assertJSONRequestRef(t, createRoleOp, "#/components/schemas/CreateRoleRequest")
	assertJSONResponseRef(t, createRoleOp, "#/components/schemas/RoleEnvelope")

	assignRolesOp := spec.Paths["/api/v1/users/{id}/roles"]["post"]
	assertJSONRequestRef(t, assignRolesOp, "#/components/schemas/AssignRolesRequest")

	menuTreeOp := spec.Paths["/api/v1/menus/tree"]["get"]
	assertJSONResponseRef(t, menuTreeOp, "#/components/schemas/MenuTreeEnvelope")

	uploadOp := spec.Paths["/api/v1/files/upload"]["post"]
	assertJSONResponseRef(t, uploadOp, "#/components/schemas/FileEnvelope")

	hashCheckOp := spec.Paths["/api/v1/files/hash/check"]["get"]
	assertJSONResponseRef(t, hashCheckOp, "#/components/schemas/FileHashCheckEnvelope")
	assertRequiredQueryParam(t, hashCheckOp, "hash", "string")

	statsOp := spec.Paths["/api/v1/files/stats"]["get"]
	assertJSONResponseRef(t, statsOp, "#/components/schemas/FileStatsEnvelope")
	fileStats := spec.Components.Schemas["FileStats"]
	for _, field := range []string{"total", "total_size", "by_type"} {
		if _, ok := fileStats.Properties[field]; !ok {
			t.Fatalf("FileStats missing %s", field)
		}
	}
	for _, field := range []string{"total_files", "image_count", "video_count", "document_count", "other_count"} {
		if _, ok := fileStats.Properties[field]; ok {
			t.Fatalf("FileStats should not include stale field %s", field)
		}
	}

	downloadOp := spec.Paths["/api/v1/files/{id}/download"]["get"]
	assertBinaryResponse(t, downloadOp)
	previewOp := spec.Paths["/api/v1/files/{id}/preview"]["get"]
	assertBinaryResponse(t, previewOp)

	runJobOp := spec.Paths["/api/v1/monitor/jobs/{id}/run"]["post"]
	assertJSONResponseRef(t, runJobOp, "#/components/schemas/EmptyEnvelope")
}

func TestOpenAPIFileSchemaIncludesThumbnailFields(t *testing.T) {
	spec := BuildSpec(nil, Options{})
	fileSchema := spec.Components.Schemas["FileItem"]

	for _, field := range []string{"thumbnail_path", "thumbnail_url"} {
		prop, ok := fileSchema.Properties[field]
		if !ok {
			t.Fatalf("FileItem missing %s", field)
		}
		if prop.Type != "string" {
			t.Fatalf("%s type = %q, want string", field, prop.Type)
		}
	}
	for _, field := range []string{"thumbnail_width", "thumbnail_height"} {
		prop, ok := fileSchema.Properties[field]
		if !ok {
			t.Fatalf("FileItem missing %s", field)
		}
		if prop.Type != "integer" {
			t.Fatalf("%s type = %q, want integer", field, prop.Type)
		}
	}
}

func TestBuildSpecDocumentsNotificationWebSocketAsUpgrade(t *testing.T) {
	spec := BuildSpec([]gin.RouteInfo{
		{Method: "GET", Path: "/api/v1/ws/notifications"},
	}, Options{})

	op := spec.Paths["/api/v1/ws/notifications"]["get"]
	if !op.XWebSocket {
		t.Fatal("notification websocket operation should be marked with x-websocket")
	}
	if _, ok := op.Responses["200"]; ok {
		t.Fatal("notification websocket must not document a normal JSON 200 response")
	}
	assertStatusResponse(t, op, "101")
	if op.Responses["101"].Description != "Switching Protocols to notification WebSocket stream" {
		t.Fatalf("101 description = %q", op.Responses["101"].Description)
	}
	if len(op.Parameters) != 1 || op.Parameters[0].Name != "ticket" || !op.Parameters[0].Required {
		t.Fatalf("websocket parameters = %#v, want required ticket query parameter", op.Parameters)
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

func assertRequiredQueryParam(t *testing.T, op Operation, name, schemaType string) {
	t.Helper()
	for _, param := range op.Parameters {
		if param.In == "query" && param.Name == name {
			if !param.Required {
				t.Fatalf("query parameter %q should be required", name)
			}
			if param.Schema.Type != schemaType {
				t.Fatalf("query parameter %q type = %q, want %q", name, param.Schema.Type, schemaType)
			}
			return
		}
	}
	t.Fatalf("operation missing %q query parameter", name)
}

func assertBinaryResponse(t *testing.T, op Operation) {
	t.Helper()
	if _, ok := op.Responses["200"].Content["application/json"]; ok {
		t.Fatal("binary endpoint must not document an application/json 200 response")
	}
	schema := op.Responses["200"].Content["application/octet-stream"].Schema
	if schema.Type != "string" || schema.Format != "binary" {
		t.Fatalf("binary schema = %#v, want string/binary", schema)
	}
}

func assertStatusResponse(t *testing.T, op Operation, status string) {
	t.Helper()
	if _, ok := op.Responses[status]; !ok {
		t.Fatalf("operation missing %s response", status)
	}
}
