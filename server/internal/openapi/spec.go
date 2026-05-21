package openapi

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

type Options struct {
	Title   string
	Version string
	Server  string
}

type Spec struct {
	OpenAPI    string                          `json:"openapi"`
	Info       Info                            `json:"info"`
	Servers    []Server                        `json:"servers,omitempty"`
	Paths      map[string]map[string]Operation `json:"paths"`
	Components Components                      `json:"components"`
}

type Info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type Server struct {
	URL string `json:"url"`
}

type Components struct {
	Schemas         map[string]Schema         `json:"schemas"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes"`
}

type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
}

type Operation struct {
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Security    []map[string][]string `json:"security,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses"`
}

type Parameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
	Schema   Schema `json:"schema"`
}

type RequestBody struct {
	Required bool                 `json:"required"`
	Content  map[string]MediaType `json:"content"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema Schema `json:"schema"`
}

type Schema struct {
	Ref                  string            `json:"$ref,omitempty"`
	Type                 string            `json:"type,omitempty"`
	Format               string            `json:"format,omitempty"`
	Description          string            `json:"description,omitempty"`
	Required             []string          `json:"required,omitempty"`
	Enum                 []string          `json:"enum,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
	AdditionalProperties any               `json:"additionalProperties,omitempty"`
}

func BuildSpec(routes []gin.RouteInfo, opts Options) Spec {
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "Go Admin Kit API"
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = "dev"
	}

	spec := Spec{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:   title,
			Version: version,
		},
		Paths: make(map[string]map[string]Operation),
		Components: Components{
			Schemas: map[string]Schema{
				"ApiResponse": {
					Type: "object",
					Properties: map[string]Schema{
						"code":    {Type: "integer"},
						"message": {Type: "string"},
						"error_code": {
							Type:        "string",
							Description: "Stable machine-readable error code, present on error responses",
						},
						"data": {
							Description:          "Business response data",
							AdditionalProperties: true,
						},
					},
				},
			},
			SecuritySchemes: map[string]SecurityScheme{
				"BearerAuth": {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "JWT",
				},
			},
		},
	}
	for name, schema := range coreSchemas() {
		spec.Components.Schemas[name] = schema
	}
	if strings.TrimSpace(opts.Server) != "" {
		spec.Servers = []Server{{URL: strings.TrimRight(opts.Server, "/")}}
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})

	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/api/v1/") && route.Path != "/api/v1" {
			continue
		}

		path := NormalizeGinPath(route.Path)
		method := strings.ToLower(route.Method)
		if spec.Paths[path] == nil {
			spec.Paths[path] = make(map[string]Operation)
		}
		spec.Paths[path][method] = buildOperation(route.Method, path)
	}

	return spec
}

func NormalizeGinPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			name := strings.TrimLeft(part, ":*")
			parts[i] = "{" + name + "}"
		}
	}
	return strings.Join(parts, "/")
}

func buildOperation(method, path string) Operation {
	contract, hasContract := contractFor(method, path)
	op := Operation{
		Tags:        []string{tagForPath(path)},
		Summary:     summaryFor(method, path),
		OperationID: operationID(method, path),
		Parameters:  pathParameters(path),
		Responses: map[string]Response{
			"200": jsonResponse("Request succeeded", Schema{Ref: "#/components/schemas/ApiResponse"}),
			"400": jsonResponse("Invalid request parameters", Schema{Ref: "#/components/schemas/ApiResponse"}),
			"401": jsonResponse("Unauthenticated or session expired", Schema{Ref: "#/components/schemas/ApiResponse"}),
			"500": jsonResponse("Internal server error", Schema{Ref: "#/components/schemas/ApiResponse"}),
		},
	}
	if !isPublicRoute(method, path) {
		op.Security = []map[string][]string{{"BearerAuth": {}}}
	}
	if hasContract && len(contract.QueryParams) > 0 {
		op.Parameters = append(op.Parameters, contract.QueryParams...)
	}
	if hasContract && contract.RequestSchema != "" {
		op.RequestBody = jsonRequestBody(refSchema(contract.RequestSchema))
	} else if hasJSONRequestBody(method, path) && (!hasContract || !contract.NoRequestBody) {
		op.RequestBody = &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				"application/json": {Schema: Schema{Type: "object", AdditionalProperties: true}},
			},
		}
	}
	if strings.Contains(path, "/files/upload") {
		op.RequestBody = &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				"multipart/form-data": {
					Schema: Schema{
						Type: "object",
						Properties: map[string]Schema{
							"file":  {Type: "string", Format: "binary"},
							"files": {Type: "array", Items: &Schema{Type: "string", Format: "binary"}},
						},
					},
				},
			},
		}
	}
	if hasContract && contract.ResponseSchema != "" {
		op.Responses["200"] = jsonResponse("Request succeeded", refSchema(contract.ResponseSchema))
	}
	if path == "/api/v1/metrics" {
		op.Responses["200"] = Response{
			Description: "Prometheus metrics text",
			Content: map[string]MediaType{
				"text/plain": {Schema: Schema{Type: "string"}},
			},
		}
	}
	return op
}

func jsonRequestBody(schema Schema) *RequestBody {
	return &RequestBody{
		Required: true,
		Content: map[string]MediaType{
			"application/json": {Schema: schema},
		},
	}
}

func jsonResponse(description string, schema Schema) Response {
	return Response{
		Description: description,
		Content: map[string]MediaType{
			"application/json": {Schema: schema},
		},
	}
}

func hasJSONRequestBody(method, path string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH":
		return !strings.Contains(path, "/files/upload")
	default:
		return false
	}
}

func isPublicRoute(method, path string) bool {
	method = strings.ToUpper(method)
	publicExact := map[string]struct{}{
		"POST /api/v1/login":          {},
		"POST /api/v1/auth/login":     {},
		"POST /api/v1/register":       {},
		"POST /api/v1/refresh":        {},
		"GET /api/v1/captcha":         {},
		"POST /api/v1/captcha/verify": {},
	}
	if _, ok := publicExact[method+" "+path]; ok {
		return true
	}
	publicPrefixes := []string{
		"/api/v1/health",
		"/api/v1/metrics",
		"/api/v1/ip/",
		"/api/v1/oauth/",
	}
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func tagForPath(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/v1/")
	segment := strings.Split(trimmed, "/")[0]
	if segment == "" {
		return "api"
	}
	switch segment {
	case "auth", "captcha", "login", "logout", "oauth", "refresh", "register", "user":
		return "auth"
	case "health", "ip", "metrics":
		return "common"
	case "monitor":
		return "monitor"
	default:
		return "system"
	}
}

func summaryFor(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

var nonIdentifierChars = regexp.MustCompile(`[^A-Za-z0-9]+`)

func operationID(method, path string) string {
	parts := nonIdentifierChars.Split(strings.Trim(path, "/"), -1)
	values := make([]string, 0, len(parts)+1)
	values = append(values, strings.ToLower(method))
	for _, part := range parts {
		if part == "" {
			continue
		}
		values = append(values, titleIdentifierPart(part))
	}
	return strings.Join(values, "")
}

func titleIdentifierPart(value string) string {
	value = strings.ToLower(value)
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func pathParameters(path string) []Parameter {
	matches := regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(path, -1)
	params := make([]Parameter, 0, len(matches))
	for _, match := range matches {
		params = append(params, Parameter{
			Name:     match[1],
			In:       "path",
			Required: true,
			Schema:   pathParamSchema(match[1]),
		})
	}
	return params
}

func pathParamSchema(name string) Schema {
	switch name {
	case "id", "user_id":
		return Schema{Type: "integer", Format: "int64"}
	default:
		return Schema{Type: "string"}
	}
}
