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
		{Method: "GET", Path: "/api/v1/user/me"},
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

	userMeOp := spec.Paths["/api/v1/user/me"]["get"]
	assertJSONResponseRef(t, userMeOp, "#/components/schemas/UserEnvelope")

	createRoleOp := spec.Paths["/api/v1/roles"]["post"]
	assertJSONRequestRef(t, createRoleOp, "#/components/schemas/CreateRoleRequest")
	assertJSONResponseRef(t, createRoleOp, "#/components/schemas/RoleEnvelope")

	assignRolesOp := spec.Paths["/api/v1/users/{id}/roles"]["post"]
	assertJSONRequestRef(t, assignRolesOp, "#/components/schemas/AssignRolesRequest")

	menuTreeOp := spec.Paths["/api/v1/menus/tree"]["get"]
	assertJSONResponseRef(t, menuTreeOp, "#/components/schemas/MenuTreeEnvelope")

	uploadOp := spec.Paths["/api/v1/files/upload"]["post"]
	assertJSONResponseRef(t, uploadOp, "#/components/schemas/FileEnvelope")

	runJobOp := spec.Paths["/api/v1/monitor/jobs/{id}/run"]["post"]
	assertJSONResponseRef(t, runJobOp, "#/components/schemas/EmptyEnvelope")
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
