// SDK route coverage tests compare the generated client specification with registered JSON endpoints.
package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

func TestSDKRoutesMatchRouter(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "router.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	registered := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Handle" {
			return true
		}
		wrapper, ok := call.Args[1].(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := wrapper.Fun.(*ast.Ident)
		if !ok || !strings.HasPrefix(name.Name, "Wrap") || name.Name == "WrapAuthRaw" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		pattern, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Errorf("invalid route pattern %s: %v", lit.Value, err)
			return true
		}
		if !strings.HasPrefix(pattern, "GET ") && !strings.HasPrefix(pattern, "POST ") {
			pattern = "GET " + pattern
		}
		if registered[pattern] {
			t.Errorf("duplicate router endpoint %s", pattern)
		}
		registered[pattern] = true
		return true
	})

	specified := make(map[string]bool)
	for _, route := range dto.SDKAPI().Routes {
		pattern := route.Method + " " + route.Path
		if specified[pattern] {
			t.Errorf("duplicate SDK endpoint %s", pattern)
		}
		specified[pattern] = true
		if !registered[pattern] {
			t.Errorf("SDK endpoint missing from router: %s", pattern)
		}
	}
	for pattern := range registered {
		if !specified[pattern] {
			t.Errorf("router endpoint missing from SDK: %s", pattern)
		}
	}
}
