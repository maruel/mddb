// Command apiclient generates a TypeScript API client from router.go and dto types.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func main() {
	root := findRepoRoot()
	routerPath := filepath.Join(root, "internal", "server", "router.go")
	dtoPath := filepath.Join(root, "internal", "server", "dto", "request.go")
	outPath := filepath.Clean(filepath.Join(root, "..", "frontend", "src", "api.ts"))

	// Parse DTO request types
	dtoTypes, err := parseDTOTypes(dtoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse dto error: %v\n", err)
		os.Exit(1)
	}

	// Parse routes from router.go
	routes, err := parseRoutes(routerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse routes error: %v\n", err)
		os.Exit(1)
	}

	// Match routes to request/response types
	endpoints := matchEndpoints(routes, dtoTypes)

	// Generate TypeScript
	if err := generateTypeScript(outPath, endpoints); err != nil {
		fmt.Fprintf(os.Stderr, "generate error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Generated %s with %d endpoints\n", outPath, len(endpoints))
}

// findRepoRoot finds the backend directory by looking for go.mod.
func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintln(os.Stderr, "could not find go.mod")
			os.Exit(1)
		}
		dir = parent
	}
}

// DTOType represents a parsed request struct with its field tags.
type DTOType struct {
	Name        string
	PathFields  []FieldInfo // Fields with path:"xxx" tag
	QueryFields []FieldInfo // Fields with query:"xxx" tag
	JSONFields  []FieldInfo // Fields with json:"xxx" tag
}

// FieldInfo represents a struct field with its tag info.
type FieldInfo struct {
	GoName   string // Go field name
	TagName  string // Tag value (path/query/json name)
	TSName   string // TypeScript property name (from json tag, or lowercased GoName)
	TypeName string // Go type name
	Optional bool   // Has omitempty
}

// Route represents a parsed route from router.go.
type Route struct {
	Method      string
	Path        string
	HandlerName string
	WrapperType string // "Wrap", "WrapAuth", "WrapAuthRaw", "WrapGlobalAdmin"
}

// Endpoint represents a fully resolved API endpoint.
type Endpoint struct {
	Method       string
	Path         string
	FuncName     string
	RequestType  string
	ResponseType string
	PathFields   []FieldInfo
	QueryFields  []FieldInfo
	JSONFields   []FieldInfo
	IsRaw        bool // WrapAuthRaw - no JSON body parsing
}

// parseDTOTypes parses dto/request.go and extracts struct definitions with tags.
func parseDTOTypes(path string) (map[string]*DTOType, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	types := make(map[string]*DTOType)

	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}

		dt := &DTOType{Name: ts.Name.Name}

		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			fieldName := field.Names[0].Name
			fieldType := exprToString(field.Type)

			if field.Tag == nil {
				continue
			}
			tag := field.Tag.Value
			tag = strings.Trim(tag, "`")

			// Determine TSName: use json tag name if present, else Go field name
			jsonTag := extractTag(tag, "json")
			tsName := fieldName // Default to Go field name (what tygo uses when no json tag)
			if jsonTag != "" {
				name, _ := parseJSONTag(jsonTag)
				if name != "-" && name != "" {
					tsName = name
				}
			}

			// Parse path tag
			if pathTag := extractTag(tag, "path"); pathTag != "" {
				dt.PathFields = append(dt.PathFields, FieldInfo{
					GoName:   fieldName,
					TagName:  pathTag,
					TSName:   tsName,
					TypeName: fieldType,
				})
			}

			// Parse query tag
			if queryTag := extractTag(tag, "query"); queryTag != "" {
				dt.QueryFields = append(dt.QueryFields, FieldInfo{
					GoName:   fieldName,
					TagName:  queryTag,
					TSName:   tsName,
					TypeName: fieldType,
				})
			}

			// Parse json tag
			if jsonTag != "" {
				name, optional := parseJSONTag(jsonTag)
				if name != "-" {
					dt.JSONFields = append(dt.JSONFields, FieldInfo{
						GoName:   fieldName,
						TagName:  name,
						TSName:   name, // JSON fields use json tag name
						TypeName: fieldType,
						Optional: optional,
					})
				}
			}
		}

		types[ts.Name.Name] = dt
		return true
	})

	return types, nil
}

// extractTag extracts a tag value like `path:"orgID"` -> "orgID".
func extractTag(tag, key string) string {
	re := regexp.MustCompile(key + `:"([^"]*)"`)
	matches := re.FindStringSubmatch(tag)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseJSONTag parses a json tag value, returning name and whether it has omitempty.
func parseJSONTag(tag string) (name string, optional bool) {
	parts := strings.Split(tag, ",")
	name = parts[0]
	for _, p := range parts[1:] {
		if p == "omitempty" {
			optional = true
		}
	}
	return
}

// exprToString converts an ast.Expr to a string representation.
func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *ast.MapType:
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)
	default:
		return "any"
	}
}

// parseRoutes parses router.go and extracts route definitions.
func parseRoutes(path string) ([]Route, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var routes []Route

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc" {
			return true
		}

		if len(call.Args) < 2 {
			return true
		}

		// First arg is the pattern string
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		pattern := strings.Trim(lit.Value, `"`)
		method, urlPath := parsePattern(pattern)

		// Second arg is the handler (possibly wrapped)
		wrapperType, handlerName := parseHandlerExpr(call.Args[1])

		if handlerName != "" && handlerName != "?" {
			routes = append(routes, Route{
				Method:      method,
				Path:        urlPath,
				HandlerName: handlerName,
				WrapperType: wrapperType,
			})
		}

		return true
	})

	return routes, nil
}

// parsePattern parses "GET /api/path" into method and path.
func parsePattern(p string) (method, path string) {
	parts := strings.SplitN(p, " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "GET", parts[0]
}

// parseHandlerExpr extracts wrapper type and handler name from the handler expression.
func parseHandlerExpr(expr ast.Expr) (wrapperType, handlerName string) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "none", exprName(expr)
	}

	funcName := exprName(call.Fun)
	switch funcName {
	case "Wrap":
		if len(call.Args) >= 1 {
			return "Wrap", exprName(call.Args[0])
		}
	case "WrapAuth":
		if len(call.Args) >= 5 {
			return "WrapAuth", exprName(call.Args[4])
		}
	case "WrapAuthRaw":
		if len(call.Args) >= 5 {
			return "WrapAuthRaw", exprName(call.Args[4])
		}
	case "WrapGlobalAdmin":
		if len(call.Args) >= 3 {
			return "WrapGlobalAdmin", exprName(call.Args[2])
		}
	}

	return "unknown", funcName
}

// exprName extracts a name from an expression.
func exprName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.CallExpr:
		return exprName(v.Fun)
	}
	return "?"
}

// matchEndpoints matches routes to their request/response types.
func matchEndpoints(routes []Route, dtoTypes map[string]*DTOType) []Endpoint {
	var endpoints []Endpoint

	for _, r := range routes {
		// Skip non-API routes
		if !strings.HasPrefix(r.Path, "/api/") {
			continue
		}

		// Skip OAuth routes (dynamic provider)
		if strings.Contains(r.Path, "/oauth/") {
			continue
		}

		// Derive request/response type names from handler name
		// e.g., "ListPages" -> "ListPagesRequest", "ListPagesResponse"
		handlerName := r.HandlerName
		reqTypeName := handlerName + "Request"
		respTypeName := handlerName + "Response"

		// Special cases
		switch handlerName {
		case "Health":
			reqTypeName = "HealthRequest"
			respTypeName = "HealthResponse"
		case "Me":
			reqTypeName = "MeRequest"
			respTypeName = "UserResponse"
		case "Register":
			respTypeName = "LoginResponse"
		case "AcceptInvitation":
			respTypeName = "LoginResponse"
		case "CreateOrganization":
			respTypeName = "OrganizationResponse"
		case "UpdateUserSettings":
			respTypeName = "UserResponse"
		case "UpdateMembershipSettings":
			respTypeName = "MembershipResponse"
		case "UpdateOrganization":
			respTypeName = "OrganizationResponse"
		case "GetOnboarding":
			respTypeName = "OnboardingState"
		case "UpdateOnboarding":
			respTypeName = "OnboardingState"
		case "CreateInvitation":
			respTypeName = "InvitationResponse"
		case "GetNode":
			respTypeName = "NodeResponse"
		case "CreateNode":
			respTypeName = "NodeResponse"
		case "Stats":
			reqTypeName = "AdminStatsRequest"
			respTypeName = "AdminStatsResponse"
		case "ListAllUsers":
			reqTypeName = "AdminUsersRequest"
			respTypeName = "AdminUsersResponse"
		case "ListAllOrgs":
			reqTypeName = "AdminOrgsRequest"
			respTypeName = "AdminOrgsResponse"
		case "GetOrganization":
			reqTypeName = "GetOnboardingRequest" // Uses same request shape
			respTypeName = "OrganizationResponse"
		case "UpdateSettings":
			reqTypeName = "UpdateOrgSettingsRequest"
			respTypeName = "OkResponse"
		case "GetRemote":
			reqTypeName = "GetGitRemoteRequest"
			respTypeName = "GitRemoteResponse"
		case "SetRemote":
			reqTypeName = "SetGitRemoteRequest"
			respTypeName = "GitRemoteResponse"
		case "DeleteRemote":
			reqTypeName = "DeleteGitRemoteRequest"
			respTypeName = "OkResponse"
		case "Push":
			reqTypeName = "PushGitRemoteRequest"
			respTypeName = "OkResponse"
		case "UpdateUserRole":
			reqTypeName = "UpdateRoleRequest"
			respTypeName = "OkResponse"
		case "SwitchOrg":
			reqTypeName = "SwitchOrgRequest"
			respTypeName = "SwitchOrgResponse"
		}

		dto := dtoTypes[reqTypeName]
		if dto == nil {
			fmt.Fprintf(os.Stderr, "warning: no DTO found for %s (handler: %s)\n", reqTypeName, handlerName)
			continue
		}

		// Generate function name: method + path-based name
		funcName := generateFuncName(r.Method, r.Path)

		endpoints = append(endpoints, Endpoint{
			Method:       r.Method,
			Path:         r.Path,
			FuncName:     funcName,
			RequestType:  reqTypeName,
			ResponseType: respTypeName,
			PathFields:   dto.PathFields,
			QueryFields:  dto.QueryFields,
			JSONFields:   dto.JSONFields,
			IsRaw:        r.WrapperType == "WrapAuthRaw",
		})
	}

	// Sort by function name for consistent output
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].FuncName < endpoints[j].FuncName
	})

	return endpoints
}

// title capitalizes the first letter of a string (ASCII only).
func title(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - 'a' + 'A'
	}
	return string(r)
}

// generateFuncName generates a TypeScript function name from method and path.
func generateFuncName(method, path string) string {
	// Remove /api/ prefix and {orgID}
	path = strings.TrimPrefix(path, "/api/")
	path = strings.TrimPrefix(path, "{orgID}/")

	// Replace path params with "ById" or similar
	re := regexp.MustCompile(`\{([^}]+)\}`)
	path = re.ReplaceAllStringFunc(path, func(match string) string {
		param := strings.Trim(match, "{}")
		// Keep meaningful param names for uniqueness
		if param == "id" {
			return "ById"
		}
		if param == "rid" {
			return "ByRid"
		}
		if param == "hash" {
			return "ByHash"
		}
		if param == "name" {
			return "ByName"
		}
		return ""
	})

	// Replace hyphens with nothing (next char will be capitalized)
	path = strings.ReplaceAll(path, "-", "/")
	path = strings.ReplaceAll(path, "//", "/")
	path = strings.Trim(path, "/")

	// Convert to camelCase
	parts := strings.Split(path, "/")
	var resultParts []string
	for _, part := range parts {
		if part == "" {
			continue
		}
		if len(resultParts) == 0 {
			resultParts = append(resultParts, strings.ToLower(part))
		} else {
			resultParts = append(resultParts, title(part))
		}
	}
	result := strings.Join(resultParts, "")

	// Add method prefix for non-GET
	switch method {
	case "POST":
		if !strings.HasPrefix(result, "create") && !strings.HasPrefix(result, "accept") &&
			!strings.HasPrefix(result, "search") && !strings.HasPrefix(result, "organizations") {
			result = "create" + title(result)
		}
	case "PUT":
		if !strings.HasPrefix(result, "update") {
			result = "update" + title(result)
		}
	case "PATCH":
		if !strings.HasPrefix(result, "patch") {
			result = "patch" + title(result)
		}
	case "DELETE":
		if !strings.HasPrefix(result, "delete") {
			result = "delete" + title(result)
		}
	}

	// Handle special cases
	if result == "" {
		result = "health"
	}

	return result
}

// generateTypeScript generates the TypeScript API client file.
func generateTypeScript(outPath string, endpoints []Endpoint) error {
	var b strings.Builder

	// Header
	b.WriteString("// Code generated by apiclient. DO NOT EDIT.\n\n")

	// Collect all types needed for imports
	typeSet := make(map[string]bool)
	for i := range endpoints {
		typeSet[endpoints[i].RequestType] = true
		typeSet[endpoints[i].ResponseType] = true
	}
	typeSet["ErrorResponse"] = true

	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}
	sort.Strings(types)

	b.WriteString("import type {\n")
	for _, t := range types {
		fmt.Fprintf(&b, "  %s,\n", t)
	}
	b.WriteString("} from './types';\n\n")

	// FetchFn type
	b.WriteString("/** Fetch function type - implement to add auth headers */\n")
	b.WriteString("export type FetchFn = (url: string, init?: RequestInit) => Promise<Response>;\n\n")

	// APIError class
	b.WriteString(`/** API error with parsed error response */
export class APIError extends Error {
  constructor(
    public status: number,
    public response: ErrorResponse,
  ) {
    super(response.error.message);
    this.name = 'APIError';
  }
}

async function parseResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const error = (await res.json()) as ErrorResponse;
    throw new APIError(res.status, error);
  }
  return res.json() as Promise<T>;
}

`)

	// Client factory function
	b.WriteString("/** Creates a typed API client */\n")
	b.WriteString("export function createAPIClient(fetch: FetchFn) {\n")
	b.WriteString("  return {\n")

	for i := range endpoints {
		writeEndpointMethod(&b, &endpoints[i])
	}

	b.WriteString("  };\n")
	b.WriteString("}\n\n")

	// Type export
	b.WriteString("export type APIClient = ReturnType<typeof createAPIClient>;\n")

	return os.WriteFile(outPath, []byte(b.String()), 0o600)
}

// writeEndpointMethod writes a single endpoint method to the builder.
func writeEndpointMethod(b *strings.Builder, ep *Endpoint) {
	// Skip raw endpoints (like file upload) - they need special handling
	if ep.IsRaw {
		fmt.Fprintf(b, "    // %s: raw endpoint, implement manually\n\n", ep.FuncName)
		return
	}

	// Determine if req is used
	hasBody := len(ep.JSONFields) > 0 && (ep.Method == http.MethodPost || ep.Method == http.MethodPut || ep.Method == http.MethodPatch)
	reqUsed := len(ep.PathFields) > 0 || len(ep.QueryFields) > 0 || hasBody
	reqParam := "req"
	if !reqUsed {
		reqParam = "_req"
	}

	// Build the function signature
	fmt.Fprintf(b, "    async %s(%s: %s): Promise<%s> {\n", ep.FuncName, reqParam, ep.RequestType, ep.ResponseType)

	// Build URL with path params (use TSName for TypeScript field access)
	urlTemplate := ep.Path
	for _, pf := range ep.PathFields {
		placeholder := "{" + pf.TagName + "}"
		urlTemplate = strings.Replace(urlTemplate, placeholder, "${req."+pf.TSName+"}", 1)
	}

	// Handle query params
	hasQuery := len(ep.QueryFields) > 0
	if hasQuery {
		b.WriteString("      const params = new URLSearchParams();\n")
		for _, qf := range ep.QueryFields {
			if qf.TypeName == "int" || qf.TypeName == "int64" {
				fmt.Fprintf(b, "      if (req.%s) params.set('%s', String(req.%s));\n", qf.TSName, qf.TagName, qf.TSName)
			} else {
				fmt.Fprintf(b, "      if (req.%s) params.set('%s', req.%s);\n", qf.TSName, qf.TagName, qf.TSName)
			}
		}
		fmt.Fprintf(b, "      const url = `%s` + (params.toString() ? `?${params}` : '');\n", urlTemplate)
	} else {
		fmt.Fprintf(b, "      const url = `%s`;\n", urlTemplate)
	}

	// Build fetch options
	switch {
	case hasBody:
		// Extract only JSON fields for body (use TSName for access, TagName for JSON key)
		b.WriteString("      const body = {\n")
		for _, jf := range ep.JSONFields {
			fmt.Fprintf(b, "        %s: req.%s,\n", jf.TagName, jf.TSName)
		}
		b.WriteString("      };\n")
		b.WriteString("      const res = await fetch(url, {\n")
		fmt.Fprintf(b, "        method: '%s',\n", ep.Method)
		b.WriteString("        headers: { 'Content-Type': 'application/json' },\n")
		b.WriteString("        body: JSON.stringify(body),\n")
		b.WriteString("      });\n")
	case ep.Method != http.MethodGet:
		fmt.Fprintf(b, "      const res = await fetch(url, { method: '%s' });\n", ep.Method)
	default:
		b.WriteString("      const res = await fetch(url);\n")
	}

	fmt.Fprintf(b, "      return parseResponse<%s>(res);\n", ep.ResponseType)
	b.WriteString("    },\n\n")
}
