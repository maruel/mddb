// Command apiroutes extracts API routes from router.go and generates docs/API.md.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

var quiet = flag.Bool("q", false, "quiet mode")

type route struct {
	Method  string
	Path    string
	Role    string
	Handler string
}

func main() {
	flag.Parse()
	// Paths relative to backend/internal/server/ where go:generate runs.
	routerPath := "router.go"

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, routerPath, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	var routes []route

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
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		pattern := strings.Trim(lit.Value, `"`)
		method, path := parsePattern(pattern)
		role, handler := parseHandler(call.Args[1])

		routes = append(routes, route{
			Method:  method,
			Path:    path,
			Role:    role,
			Handler: handler,
		})
		return true
	})

	// Sort by path, then method
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})

	// Group routes by prefix
	groups := groupRoutes(routes)

	outPath := "../../docs/API.md"
	out, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create file: %v\n", err)
		os.Exit(1)
	}
	if err := writeMarkdown(out, groups); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}
	if err := out.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close file: %v\n", err)
		os.Exit(1)
	}
	if !*quiet {
		fmt.Fprintf(os.Stderr, "Generated backend/docs/API.md with %d routes\n", len(routes))
	}
}

func parsePattern(p string) (method, path string) {
	parts := strings.SplitN(p, " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "*", parts[0]
}

func parseHandler(expr ast.Expr) (role, handler string) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "public", exprName(expr)
	}

	funcName := exprName(call.Fun)
	switch funcName {
	case "Wrap":
		if len(call.Args) >= 1 {
			return "public", exprName(call.Args[0])
		}
	case "WrapAuth":
		// Args: userService, orgMemService, sessionService, jwtSecret, role, handler, rlConfig
		if len(call.Args) >= 6 {
			return exprName(call.Args[4]), exprName(call.Args[5])
		}
	case "WrapWSAuth", "WrapAuthRaw":
		// Args: userService, orgMemService, wsMemService, wsService, sessionService, jwtSecret, role, handler, rlConfig
		if len(call.Args) >= 8 {
			return exprName(call.Args[6]), exprName(call.Args[7])
		}
	case "WrapGlobalAdmin":
		// Args: userService, sessionService, jwtSecret, handler
		if len(call.Args) >= 4 {
			return "globalAdmin", exprName(call.Args[3])
		}
	}
	return "?", funcName
}

func exprName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprName(v.X) + "." + v.Sel.Name
	case *ast.CallExpr:
		return exprName(v.Fun)
	}
	return "?"
}

type routeGroup struct {
	Name   string
	Routes []route
}

func groupRoutes(routes []route) []routeGroup {
	order := []string{"Health", "Admin", "Auth", "Settings", "Users", "Invitations", "Nodes", "Pages", "Tables", "Search", "Assets"}
	groups := make(map[string][]route)

	for _, r := range routes {
		name := categorize(r.Path)
		groups[name] = append(groups[name], r)
	}

	var result []routeGroup
	for _, name := range order {
		if rs, ok := groups[name]; ok {
			result = append(result, routeGroup{Name: name, Routes: rs})
			delete(groups, name)
		}
	}
	// Any remaining
	for name, rs := range groups {
		result = append(result, routeGroup{Name: name, Routes: rs})
	}
	return result
}

func categorize(path string) string {
	switch {
	case path == "/api/health":
		return "Health"
	case path == "/":
		return "Frontend"
	case strings.HasPrefix(path, "/api/admin"):
		return "Admin"
	case strings.HasPrefix(path, "/api/auth"):
		return "Auth"
	case strings.Contains(path, "/settings/") || strings.Contains(path, "/onboarding"):
		return "Settings"
	case strings.Contains(path, "/users"):
		return "Users"
	case strings.Contains(path, "/invitations"):
		return "Invitations"
	case strings.Contains(path, "/nodes"):
		return "Nodes"
	case strings.Contains(path, "/pages") && strings.Contains(path, "/assets"):
		return "Assets"
	case strings.Contains(path, "/pages"):
		return "Pages"
	case strings.Contains(path, "/tables") || strings.Contains(path, "/records"):
		return "Tables"
	case strings.Contains(path, "/search"):
		return "Search"
	case strings.HasPrefix(path, "/assets/"):
		return "Assets"
	default:
		return "Other"
	}
}

func writeMarkdown(out *os.File, groups []routeGroup) error {
	lines := []string{
		"# mddb API Reference",
		"",
		"<!-- Code generated by go generate; DO NOT EDIT. -->",
		"",
		"RESTful JSON API for mddb. Most endpoints require `{orgID}` for data isolation.",
		"",
		"## Authentication",
		"",
		"Include JWT token in Authorization header: `Authorization: Bearer <token>`",
		"",
		"**Roles:** `viewer` (read), `editor` (read/write), `admin` (full), `globalAdmin` (server-wide)",
		"",
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}

	for _, g := range groups {
		if g.Name == "Frontend" {
			continue
		}
		if _, err := fmt.Fprintf(out, "## %s\n\n", g.Name); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "| Method | Path | Auth |"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "|--------|------|------|"); err != nil {
			return err
		}
		for _, r := range g.Routes {
			if _, err := fmt.Fprintf(out, "| %s | `%s` | %s |\n", r.Method, r.Path, r.Role); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	return nil
}
