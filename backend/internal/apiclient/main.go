// Command apiclient generates a TypeScript API client from router.go and handler signatures.
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
	handlersDir := filepath.Join(root, "internal", "server", "handlers")
	outPath := filepath.Clean(filepath.Join(root, "..", "frontend", "src", "api.ts"))

	// Parse DTO request types for field info
	dtoTypes, err := parseDTOTypes(dtoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse dto error: %v\n", err)
		os.Exit(1)
	}

	// Parse handler signatures to get actual request/response types
	handlerSigs, err := parseHandlerSignatures(handlersDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse handlers error: %v\n", err)
		os.Exit(1)
	}

	// Parse routes from router.go
	routes, err := parseRoutes(routerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse routes error: %v\n", err)
		os.Exit(1)
	}

	// Match routes to request/response types
	endpoints, err := matchEndpoints(routes, dtoTypes, handlerSigs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match endpoints error: %v\n", err)
		os.Exit(1)
	}

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

// HandlerSig represents a parsed handler function signature.
type HandlerSig struct {
	MethodName   string // e.g., "GetPage"
	RequestType  string // e.g., "GetPageRequest"
	ResponseType string // e.g., "GetPageResponse"
}

// DTOType represents a parsed request struct with its field tags.
type DTOType struct {
	Name        string
	PathFields  []FieldInfo
	QueryFields []FieldInfo
	JSONFields  []FieldInfo
}

// FieldInfo represents a struct field with its tag info.
type FieldInfo struct {
	GoName   string
	TagName  string
	TSName   string
	TypeName string
	Optional bool
}

// Route represents a parsed route from router.go.
type Route struct {
	Method      string
	Path        string
	HandlerName string // e.g., "GetPage" (method name only)
	IsRaw       bool   // WrapAuthRaw - no JSON body parsing
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
}

// parseHandlerSignatures parses all handler files and extracts function signatures.
func parseHandlerSignatures(dir string) (map[string]*HandlerSig, error) {
	sigs := make(map[string]*HandlerSig)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}

			// Check if this is a method on a *Handler type
			recvType := exprToString(fn.Recv.List[0].Type)
			if !strings.HasSuffix(recvType, "Handler") {
				continue
			}

			// Extract request and response types from function signature
			sig := extractSignature(fn)
			if sig != nil {
				sigs[sig.MethodName] = sig
			}
		}
	}

	return sigs, nil
}

// extractSignature extracts request/response types from a handler function.
func extractSignature(fn *ast.FuncDecl) *HandlerSig {
	if fn.Type.Params == nil || fn.Type.Results == nil {
		return nil
	}

	// Find the last parameter that's a pointer to dto.XXXRequest
	var reqType string
	for _, param := range fn.Type.Params.List {
		typeStr := exprToString(param.Type)
		if strings.HasPrefix(typeStr, "*dto.") && strings.HasSuffix(typeStr, "Request") {
			reqType = strings.TrimPrefix(typeStr, "*dto.")
		}
	}

	if reqType == "" {
		return nil
	}

	// Find the first result that's a pointer to dto.XXX (response type)
	var respType string
	for _, result := range fn.Type.Results.List {
		typeStr := exprToString(result.Type)
		if strings.HasPrefix(typeStr, "*dto.") {
			respType = strings.TrimPrefix(typeStr, "*dto.")
			break
		}
	}

	if respType == "" {
		return nil
	}

	return &HandlerSig{
		MethodName:   fn.Name.Name,
		RequestType:  reqType,
		ResponseType: respType,
	}
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
			if len(field.Names) == 0 || field.Tag == nil {
				continue
			}
			fieldName := field.Names[0].Name
			fieldType := exprToString(field.Type)
			tag := strings.Trim(field.Tag.Value, "`")

			// Determine TSName: use json tag name if present, else Go field name
			jsonTag := extractTag(tag, "json")
			tsName := fieldName
			if jsonTag != "" {
				name, _ := parseJSONTag(jsonTag)
				if name != "-" && name != "" {
					tsName = name
				}
			}

			if pathTag := extractTag(tag, "path"); pathTag != "" {
				dt.PathFields = append(dt.PathFields, FieldInfo{
					GoName: fieldName, TagName: pathTag, TSName: tsName, TypeName: fieldType,
				})
			}

			if queryTag := extractTag(tag, "query"); queryTag != "" {
				dt.QueryFields = append(dt.QueryFields, FieldInfo{
					GoName: fieldName, TagName: queryTag, TSName: tsName, TypeName: fieldType,
				})
			}

			if jsonTag != "" {
				name, optional := parseJSONTag(jsonTag)
				if name != "-" {
					dt.JSONFields = append(dt.JSONFields, FieldInfo{
						GoName: fieldName, TagName: name, TSName: name, TypeName: fieldType, Optional: optional,
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
		if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
			return true
		}

		if len(call.Args) < 2 {
			return true
		}

		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		pattern := strings.Trim(lit.Value, `"`)
		method, urlPath := parsePattern(pattern)

		wrapperType, handlerName := parseHandlerExpr(call.Args[1])
		if handlerName != "" && handlerName != "?" {
			routes = append(routes, Route{
				Method:      method,
				Path:        urlPath,
				HandlerName: handlerName,
				IsRaw:       wrapperType == "WrapAuthRaw",
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

// parseHandlerExpr extracts wrapper type and handler method name from the handler expression.
func parseHandlerExpr(expr ast.Expr) (wrapperType, handlerName string) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		// Direct handler reference (shouldn't happen in this codebase)
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

// exprName extracts the method name from an expression like `ph.GetPage`.
func exprName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name // Return just the method name
	case *ast.CallExpr:
		return exprName(v.Fun)
	}
	return "?"
}

// matchEndpoints matches routes to their request/response types.
func matchEndpoints(routes []Route, dtoTypes map[string]*DTOType, handlerSigs map[string]*HandlerSig) ([]Endpoint, error) {
	var endpoints []Endpoint
	var errs []string

	for _, r := range routes {
		// Skip non-API routes
		if !strings.HasPrefix(r.Path, "/api/") {
			continue
		}

		// Skip OAuth routes (dynamic provider)
		if strings.Contains(r.Path, "/oauth/") {
			continue
		}

		// Skip raw handlers (they don't follow the typed pattern)
		if r.IsRaw {
			continue
		}

		// Look up handler signature
		sig := handlerSigs[r.HandlerName]
		if sig == nil {
			errs = append(errs, "no handler signature found for "+r.HandlerName)
			continue
		}

		// Look up DTO for field info
		dto := dtoTypes[sig.RequestType]
		if dto == nil {
			errs = append(errs, "no DTO found for "+sig.RequestType+" (handler: "+r.HandlerName+")")
			continue
		}

		funcName := toLowerCamel(r.HandlerName)

		endpoints = append(endpoints, Endpoint{
			Method:       r.Method,
			Path:         r.Path,
			FuncName:     funcName,
			RequestType:  sig.RequestType,
			ResponseType: sig.ResponseType,
			PathFields:   dto.PathFields,
			QueryFields:  dto.QueryFields,
			JSONFields:   dto.JSONFields,
		})
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("matching errors:\n  %s", strings.Join(errs, "\n  "))
	}

	// Sort by function name for consistent output
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].FuncName < endpoints[j].FuncName
	})

	return endpoints, nil
}

// toLowerCamel converts a PascalCase string to lowerCamelCase.
func toLowerCamel(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'A' && r[0] <= 'Z' {
		r[0] = r[0] - 'A' + 'a'
	}
	return string(r)
}

// generateTypeScript generates the TypeScript API client file.
func generateTypeScript(outPath string, endpoints []Endpoint) error {
	var b strings.Builder

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

	b.WriteString("/** Fetch function type - implement to add auth headers */\n")
	b.WriteString("export type FetchFn = (url: string, init?: RequestInit) => Promise<Response>;\n\n")

	b.WriteString(`/** API error with parsed error response */
export class APIError extends Error {
  constructor(
    public status: number,
    public response: ErrorResponse
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

	b.WriteString("/** Creates a typed API client */\n")
	b.WriteString("export function createAPIClient(fetch: FetchFn) {\n")
	b.WriteString("  return {\n")

	for i := range endpoints {
		if i > 0 {
			b.WriteString("\n")
		}
		writeEndpointMethod(&b, &endpoints[i])
	}

	b.WriteString("  };\n")
	b.WriteString("}\n\n")

	b.WriteString("export type APIClient = ReturnType<typeof createAPIClient>;\n")

	return os.WriteFile(outPath, []byte(b.String()), 0o600)
}

// writeEndpointMethod writes a single endpoint method to the builder.
func writeEndpointMethod(b *strings.Builder, ep *Endpoint) {
	hasBody := len(ep.JSONFields) > 0 && (ep.Method == http.MethodPost || ep.Method == http.MethodPut || ep.Method == http.MethodPatch)
	reqUsed := len(ep.PathFields) > 0 || len(ep.QueryFields) > 0 || hasBody
	reqParam := "req"
	if !reqUsed {
		reqParam = "_req"
	}

	// Break long function signatures (printWidth)
	sigLine := fmt.Sprintf("    async %s(%s: %s): Promise<%s> {", ep.FuncName, reqParam, ep.RequestType, ep.ResponseType)
	if len(sigLine) <= 120 {
		fmt.Fprintf(b, "%s\n", sigLine)
	} else {
		fmt.Fprintf(b, "    async %s(\n      %s: %s\n    ): Promise<%s> {\n", ep.FuncName, reqParam, ep.RequestType, ep.ResponseType)
	}

	urlTemplate := ep.Path
	for _, pf := range ep.PathFields {
		placeholder := "{" + pf.TagName + "}"
		urlTemplate = strings.Replace(urlTemplate, placeholder, "${req."+pf.TSName+"}", 1)
	}

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

	switch {
	case hasBody:
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
	b.WriteString("    },\n")
}
