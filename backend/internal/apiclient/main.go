// Command apiclient generates a TypeScript API client from router.go and handler signatures.
package main

import (
	"flag"
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

var quiet = flag.Bool("q", false, "quiet mode")

func main() {
	flag.Parse()
	// Paths relative to backend/internal/server/ where go:generate runs.
	routerPath := "router.go"
	dtoPath := "dto/request.go"
	handlersDir := "handlers"
	outPath := "../../../frontend/src/api.gen.ts"

	dtoTypes, err := parseDTOTypes(dtoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse dto error: %v\n", err)
		os.Exit(1)
	}

	handlerSigs, err := parseHandlerSignatures(handlersDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse handlers error: %v\n", err)
		os.Exit(1)
	}

	routes, err := parseRoutes(routerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse routes error: %v\n", err)
		os.Exit(1)
	}

	endpoints, err := matchEndpoints(routes, dtoTypes, handlerSigs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match endpoints error: %v\n", err)
		os.Exit(1)
	}

	if err := generateTypeScript(outPath, endpoints); err != nil {
		fmt.Fprintf(os.Stderr, "generate error: %v\n", err)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Fprintf(os.Stderr, "Generated %s with %d endpoints\n", outPath, len(endpoints))
	}
}


// HandlerSig represents a parsed handler function signature.
type HandlerSig struct {
	MethodName   string
	RequestType  string
	ResponseType string
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
	HandlerName string
	IsRaw       bool
}

// Endpoint represents a fully resolved API endpoint.
type Endpoint struct {
	Method       string
	Path         string
	HandlerName  string
	RequestType  string
	ResponseType string
	PathFields   []FieldInfo
	QueryFields  []FieldInfo
	JSONFields   []FieldInfo
	IsOrgScoped  bool
}

// NamespaceNode represents a node in the namespace tree.
type NamespaceNode struct {
	Name     string
	Methods  []*NamespaceMethod
	Children map[string]*NamespaceNode
}

// NamespaceMethod represents a method within a namespace.
type NamespaceMethod struct {
	Name        string // e.g., "list", "get", "create"
	Endpoint    *Endpoint
	PathArgs    []PathArg   // Path params that become function args
	QueryFields []FieldInfo // Query params
	BodyFields  []FieldInfo // JSON body fields
}

// PathArg represents a path parameter that becomes a function argument.
type PathArg struct {
	Name   string // Argument name (e.g., "id", "pageId")
	TSName string // TypeScript field name from DTO
}

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

			recvType := exprToString(fn.Recv.List[0].Type)
			if !strings.HasSuffix(recvType, "Handler") {
				continue
			}

			sig := extractSignature(fn)
			if sig != nil {
				sigs[sig.MethodName] = sig
			}
		}
	}

	return sigs, nil
}

func extractSignature(fn *ast.FuncDecl) *HandlerSig {
	if fn.Type.Params == nil || fn.Type.Results == nil {
		return nil
	}

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

func extractTag(tag, key string) string {
	re := regexp.MustCompile(key + `:"([^"]*)"`)
	matches := re.FindStringSubmatch(tag)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

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

func parsePattern(p string) (method, path string) {
	parts := strings.SplitN(p, " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "GET", parts[0]
}

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

func matchEndpoints(routes []Route, dtoTypes map[string]*DTOType, handlerSigs map[string]*HandlerSig) ([]*Endpoint, error) {
	var endpoints []*Endpoint
	var errs []string

	for _, r := range routes {
		if !strings.HasPrefix(r.Path, "/api/") {
			continue
		}

		if strings.Contains(r.Path, "/oauth/") {
			continue
		}

		if r.IsRaw {
			continue
		}

		sig := handlerSigs[r.HandlerName]
		if sig == nil {
			errs = append(errs, "no handler signature found for "+r.HandlerName)
			continue
		}

		dto := dtoTypes[sig.RequestType]
		if dto == nil {
			errs = append(errs, "no DTO found for "+sig.RequestType+" (handler: "+r.HandlerName+")")
			continue
		}

		// Check if org-scoped (has {orgID} in path)
		isOrgScoped := strings.Contains(r.Path, "{orgID}")

		endpoints = append(endpoints, &Endpoint{
			Method:       r.Method,
			Path:         r.Path,
			HandlerName:  r.HandlerName,
			RequestType:  sig.RequestType,
			ResponseType: sig.ResponseType,
			PathFields:   dto.PathFields,
			QueryFields:  dto.QueryFields,
			JSONFields:   dto.JSONFields,
			IsOrgScoped:  isOrgScoped,
		})
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("matching errors:\n  %s", strings.Join(errs, "\n  "))
	}

	return endpoints, nil
}

// buildNamespaceTree builds a tree structure from endpoints.
func buildNamespaceTree(endpoints []*Endpoint, orgScoped bool) *NamespaceNode {
	root := &NamespaceNode{
		Name:     "root",
		Children: make(map[string]*NamespaceNode),
	}

	for _, ep := range endpoints {
		if ep.IsOrgScoped != orgScoped {
			continue
		}

		// Parse path into namespace parts
		nsParts, pathArgs := parseNamespacePath(ep.Path, ep.PathFields, orgScoped)

		// Find or create the namespace node
		node := root
		for _, part := range nsParts {
			if node.Children[part] == nil {
				node.Children[part] = &NamespaceNode{
					Name:     part,
					Children: make(map[string]*NamespaceNode),
				}
			}
			node = node.Children[part]
		}

		// Determine method name from handler and HTTP method
		methodName := deriveMethodName(ep.HandlerName)

		// Add method to the node
		node.Methods = append(node.Methods, &NamespaceMethod{
			Name:        methodName,
			Endpoint:    ep,
			PathArgs:    pathArgs,
			QueryFields: ep.QueryFields,
			BodyFields:  ep.JSONFields,
		})
	}

	// Flatten redundant namespaces
	flattenNamespaces(root)

	return root
}

// flattenNamespaces promotes methods from child namespaces when the child only
// has a single method with the same name as the namespace.
// E.g., auth.login.login() becomes auth.login().
func flattenNamespaces(node *NamespaceNode) {
	for name, child := range node.Children {
		// Recursively process children first
		flattenNamespaces(child)

		// Check if this child should be flattened:
		// - Has exactly one method
		// - Has no children
		// - Method name matches namespace name (or is a common single-method pattern)
		if len(child.Children) == 0 && len(child.Methods) == 1 {
			method := child.Methods[0]
			// If method name equals or contains namespace name, promote it
			if method.Name == name || method.Name == hyphenToCamel(name) ||
				strings.Contains(strings.ToLower(method.Name), strings.ToLower(name)) {
				// Promote the method to parent, keeping the method name
				node.Methods = append(node.Methods, method)
				delete(node.Children, name)
			}
		}

		// Flatten single-child namespaces where the child has methods but no grandchildren
		// e.g., settings.git.remote -> settings.git (merge remote's methods into git)
		if len(child.Children) == 1 && len(child.Methods) == 0 {
			for grandchildName, grandchild := range child.Children {
				if len(grandchild.Children) == 0 {
					// Merge grandchild's methods into child
					child.Methods = append(child.Methods, grandchild.Methods...)
					delete(child.Children, grandchildName)
				}
			}
		}
	}
}

// parseNamespacePath extracts namespace parts and path arguments from a URL path.
func parseNamespacePath(path string, pathFields []FieldInfo, orgScoped bool) ([]string, []PathArg) {
	// Remove /api/ prefix
	path = strings.TrimPrefix(path, "/api/")

	// Remove {orgID}/ for org-scoped paths
	if orgScoped {
		path = strings.TrimPrefix(path, "{orgID}/")
	}

	parts := strings.Split(path, "/")
	var nsParts []string
	var pathArgs []PathArg

	// Build a map of path tag -> field info
	pathFieldMap := make(map[string]FieldInfo)
	for _, pf := range pathFields {
		pathFieldMap[pf.TagName] = pf
	}

	for _, part := range parts {
		if part == "" {
			continue
		}

		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			// This is a path parameter
			paramName := strings.Trim(part, "{}")
			if paramName == "orgID" {
				continue // Skip orgID, handled separately
			}

			// Find the corresponding field info
			if fi, ok := pathFieldMap[paramName]; ok {
				// Determine arg name based on context
				argName := paramName
				if paramName == "id" && len(nsParts) > 0 {
					// Keep as "id" for simplicity
					argName = "id"
				}
				pathArgs = append(pathArgs, PathArg{
					Name:   argName,
					TSName: fi.TSName,
				})
			}
		} else {
			// This is a namespace segment - convert hyphens to camelCase
			nsParts = append(nsParts, hyphenToCamel(part))
		}
	}

	return nsParts, pathArgs
}

// hyphenToCamel converts hyphenated-string to camelCase.
func hyphenToCamel(s string) string {
	parts := strings.Split(s, "-")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// deriveMethodName determines the method name from handler name.
// Follows a simple convention:
//   - List* → list
//   - Get* → get
//   - Create* → create
//   - Update* → update
//   - Delete* → delete
//   - Other (Login, Register, Search, etc.) → toLowerCamel(name)
func deriveMethodName(handlerName string) string {
	switch {
	case strings.HasPrefix(handlerName, "List"):
		return "list"
	case strings.HasPrefix(handlerName, "Get"):
		return "get"
	case strings.HasPrefix(handlerName, "Create"):
		return "create"
	case strings.HasPrefix(handlerName, "Update"):
		return "update"
	case strings.HasPrefix(handlerName, "Delete"):
		return "delete"
	default:
		return toLowerCamel(handlerName)
	}
}

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
func generateTypeScript(outPath string, endpoints []*Endpoint) error {
	var b strings.Builder

	b.WriteString("// Code generated by apiclient. DO NOT EDIT.\n\n")

	// Collect response and request types for imports
	typeSet := make(map[string]bool)
	for _, ep := range endpoints {
		typeSet[ep.ResponseType] = true
		// Import request type if it has body or query fields
		if len(ep.JSONFields) > 0 || len(ep.QueryFields) > 0 {
			typeSet[ep.RequestType] = true
		}
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
	b.WriteString("} from './types.gen';\n\n")

	// FetchFn type
	b.WriteString("/** Fetch function type - implement to add auth headers */\n")
	b.WriteString("export type FetchFn = (url: string, init?: RequestInit) => Promise<Response>;\n\n")

	// APIError class and request helpers
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

async function get<T>(fetchFn: FetchFn, url: string): Promise<T> {
  const res = await fetchFn(url);
  if (!res.ok) {
    const error = (await res.json()) as ErrorResponse;
    throw new APIError(res.status, error);
  }
  return res.json() as Promise<T>;
}

async function post<T>(fetchFn: FetchFn, url: string, body?: object): Promise<T> {
  const init: RequestInit = { method: 'POST' };
  if (body) {
    init.headers = { 'Content-Type': 'application/json' };
    init.body = JSON.stringify(body);
  }
  const res = await fetchFn(url, init);
  if (!res.ok) {
    const error = (await res.json()) as ErrorResponse;
    throw new APIError(res.status, error);
  }
  return res.json() as Promise<T>;
}

`)

	// Build namespace trees
	globalTree := buildNamespaceTree(endpoints, false)
	orgTree := buildNamespaceTree(endpoints, true)

	// Generate client
	b.WriteString("/** Creates a typed API client */\n")
	b.WriteString("export function createAPIClient(fetchFn: FetchFn) {\n")
	b.WriteString("  return {\n")

	// Generate global namespaces
	writeNamespaceNode(&b, globalTree, "    ", false)

	// Generate org factory
	b.WriteString("\n    /** Returns an org-scoped API client */\n")
	b.WriteString("    org(orgID: string) {\n")
	b.WriteString("      return {\n")
	writeNamespaceNode(&b, orgTree, "        ", true)
	b.WriteString("      };\n")
	b.WriteString("    },\n")

	b.WriteString("  };\n")
	b.WriteString("}\n\n")

	b.WriteString("export type APIClient = ReturnType<typeof createAPIClient>;\n")
	b.WriteString("export type OrgAPIClient = ReturnType<APIClient['org']>;\n")

	return os.WriteFile(outPath, []byte(b.String()), 0o600)
}

// writeNamespaceNode writes a namespace node and its children recursively.
func writeNamespaceNode(b *strings.Builder, node *NamespaceNode, indent string, isOrgScoped bool) {
	// Get sorted child keys for consistent output
	childKeys := make([]string, 0, len(node.Children))
	for k := range node.Children {
		childKeys = append(childKeys, k)
	}
	sort.Strings(childKeys)

	// Write child namespaces
	for _, key := range childKeys {
		child := node.Children[key]

		// Check if this namespace has only methods (leaf) or also children
		hasChildren := len(child.Children) > 0
		hasMethods := len(child.Methods) > 0

		if hasChildren || hasMethods {
			fmt.Fprintf(b, "%s%s: {\n", indent, key)
			writeNamespaceNode(b, child, indent+"  ", isOrgScoped)
			fmt.Fprintf(b, "%s},\n", indent)
		}
	}

	// Write methods at this level
	// Sort methods for consistent output
	sort.Slice(node.Methods, func(i, j int) bool {
		return node.Methods[i].Name < node.Methods[j].Name
	})

	for _, method := range node.Methods {
		writeMethod(b, method, indent, isOrgScoped)
	}
}

// writeMethod writes a single method using get/post helpers.
func writeMethod(b *strings.Builder, m *NamespaceMethod, indent string, isOrgScoped bool) {
	ep := m.Endpoint
	isGet := ep.Method == http.MethodGet

	// Build function signature
	var params []string
	for _, arg := range m.PathArgs {
		params = append(params, arg.Name+": string")
	}

	// Add options parameter if there are query or body fields
	hasQuery := len(m.QueryFields) > 0
	hasBody := len(m.BodyFields) > 0 && !isGet

	if hasQuery || hasBody {
		params = append(params, "options: "+ep.RequestType)
	}

	paramStr := strings.Join(params, ", ")

	// Build URL template
	urlTemplate := ep.Path
	if isOrgScoped {
		urlTemplate = strings.Replace(urlTemplate, "{orgID}", "${orgID}", 1)
	}
	for _, arg := range m.PathArgs {
		placeholder := "{" + arg.Name + "}"
		urlTemplate = strings.Replace(urlTemplate, placeholder, "${"+arg.Name+"}", 1)
	}

	// Simple GET: no query params
	if isGet && !hasQuery {
		fmt.Fprintf(b, "%s%s: (%s) => get<%s>(fetchFn, `%s`),\n", indent, m.Name, paramStr, ep.ResponseType, urlTemplate)
		return
	}

	// Simple POST: no query params, no body
	if !isGet && !hasQuery && !hasBody {
		fmt.Fprintf(b, "%s%s: (%s) => post<%s>(fetchFn, `%s`),\n", indent, m.Name, paramStr, ep.ResponseType, urlTemplate)
		return
	}

	// POST with body only (no query params): pass options directly
	if !isGet && hasBody && !hasQuery {
		fmt.Fprintf(b, "%s%s: (%s) => post<%s>(fetchFn, `%s`, options),\n", indent, m.Name, paramStr, ep.ResponseType, urlTemplate)
		return
	}

	// Complex case: has query params
	fmt.Fprintf(b, "%sasync %s(%s): Promise<%s> {\n", indent, m.Name, paramStr, ep.ResponseType)
	fmt.Fprintf(b, "%s  const params = new URLSearchParams();\n", indent)
	for _, qf := range m.QueryFields {
		if qf.TypeName == "int" || qf.TypeName == "int64" {
			fmt.Fprintf(b, "%s  if (options.%s) params.set('%s', String(options.%s));\n", indent, qf.TSName, qf.TagName, qf.TSName)
		} else {
			fmt.Fprintf(b, "%s  if (options.%s) params.set('%s', options.%s);\n", indent, qf.TSName, qf.TagName, qf.TSName)
		}
	}
	fmt.Fprintf(b, "%s  const url = `%s` + (params.toString() ? `?${params}` : '');\n", indent, urlTemplate)

	switch {
	case isGet:
		fmt.Fprintf(b, "%s  return get<%s>(fetchFn, url);\n", indent, ep.ResponseType)
	case hasBody:
		fmt.Fprintf(b, "%s  return post<%s>(fetchFn, url, options);\n", indent, ep.ResponseType)
	default:
		fmt.Fprintf(b, "%s  return post<%s>(fetchFn, url);\n", indent, ep.ResponseType)
	}
	fmt.Fprintf(b, "%s},\n", indent)
}
