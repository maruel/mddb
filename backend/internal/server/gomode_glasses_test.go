// Tests node_show_on_glasses: page text for the glasses kept from the model, and its errors.

package server

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/gomodedev/gomode"
	"github.com/gomodedev/gomode/mcp"
)

func TestGlassesText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, markdown, want string
		cut                  bool
	}{
		{"Heading", "# Title\n## Sub\n#hashtag", "Title\nSub\n#hashtag", false},
		{"Lists", "- one\n* two\n+ three\n1. four", "- one\n- two\n- three\n1. four", false},
		{"Tasks", "- [ ] milk\n- [X] eggs", "[ ] milk\n[x] eggs", false},
		{"Inline", "**bold** __strong__ ~~old~~ `code` snake_case *em*", "bold strong old code snake_case *em*", false},
		{"Links", "See [docs](https://example.com) ![](a.png) ![logo](b.png)", "See docs  logo", false},
		{"Quote", "> > quoted", "quoted", false},
		{"Rules", "a\n\n---\n\n* * *\n\nb", "a\n\nb", false},
		{"Table", "| a | b |\n|---|:-:|\n| 1 | 2 |", "| a | b |\n| 1 | 2 |", false},
		{"Code", "before\n```go\n  x := **1**\n```\nafter", "before\n  x := **1**\nafter", false},
		{"Blank", "\n\na\n\n\n\nb\n\n", "a\n\nb", false},
		{"Cut", "first line\nsecond li", "first line…", true},
		{"CutOneLine", "a very long li", "a very long li…", true},
		{"Empty", "---\n\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := glassesText(tc.markdown, tc.cut); got != tc.want {
				t.Fatalf("glassesText(%q) = %q, want %q", tc.markdown, got, tc.want)
			}
		})
	}
}

// glassesOutcome is the wire result of a node_show_on_glasses call.
type glassesOutcome struct {
	IsError           bool               `json:"isError"`
	Content           []mcp.ContentBlock `json:"content"`
	StructuredContent mcp.TextOutput     `json:"structuredContent"`
}

func TestGoModeMCPShowNodeOnGlasses(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	token, _ := setupGoModeWorkspace(t, env, "gomode-glasses@example.com")
	halo := map[string]any{"extensions": map[string]any{gomode.DisplayExtension: map[string]any{
		"width": 256, "height": 256, "shape": "circle", "colorSpace": "rgb", "bitsPerPixel": 4, "imageTypes": []string{"image/png"},
	}}}
	show := func(t *testing.T, nodeID string, capabilities map[string]any) glassesOutcome {
		params := map[string]any{"name": "node_show_on_glasses", "arguments": map[string]any{"nodeId": nodeID}}
		if capabilities != nil {
			params["_meta"] = map[string]any{"io.modelcontextprotocol/clientCapabilities": capabilities}
		}
		var out glassesOutcome
		if err := json.Unmarshal(callMCPNamedAt(t, env, token, goModeMCPEndpoint, "tools/call", params, "node_show_on_glasses"), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	// userOnly returns the text of the blocks whose audience is the user alone.
	userOnly := func(out glassesOutcome) []string {
		var texts []string
		for _, b := range out.Content {
			if slices.Equal(b.Annotations.Audience, []mcp.Role{mcp.RoleUser}) {
				texts = append(texts, b.Text)
			}
		}
		return texts
	}
	page := callMCPTool(t, env, token, "page_create", map[string]any{"title": "Groceries", "content": "## Today\n\n- [ ] **milk**\n- [x] eggs"}).StructuredContent.Node
	t.Run("valid", func(t *testing.T) {
		t.Run("Page", func(t *testing.T) {
			out := show(t, page.ID, halo)
			if out.IsError || out.StructuredContent.Result != `The glasses show the page "Groceries".` {
				t.Fatalf("result = %+v", out)
			}
			if got := userOnly(out); !slices.Equal(got, []string{"Groceries\n\nToday\n\n[ ] milk\n[x] eggs"}) {
				t.Fatalf("glasses blocks = %q", got)
			}
		})
		t.Run("Long", func(t *testing.T) {
			line := strings.Repeat("word ", 19) + "end\n"
			long := callMCPTool(t, env, token, "page_create", map[string]any{"title": "Long", "content": strings.Repeat(line, 2*glassesContentBytes/len(line))}).StructuredContent.Node
			out := show(t, long.ID, halo)
			if out.IsError || out.StructuredContent.Result != `The glasses show the start of the page "Long".` {
				t.Fatalf("result = %+v", out)
			}
			got := userOnly(out)
			if len(got) != 1 || len(got[0]) > glassesContentBytes || !strings.HasSuffix(got[0], "end…") {
				t.Fatalf("glasses blocks = %q", got)
			}
		})
	})
	t.Run("error", func(t *testing.T) {
		table := callMCPTool(t, env, token, "table_create", map[string]any{"title": "Tasks", "properties": []any{}}).StructuredContent.Node
		for name, tc := range map[string]struct {
			nodeID       string
			capabilities map[string]any
		}{
			"NoGlasses": {page.ID, map[string]any{}},
			"Malformed": {page.ID, map[string]any{"extensions": map[string]any{gomode.DisplayExtension: map[string]any{"width": 0}}}},
			"NoText":    {table.ID, halo},
			"InvalidID": {"invalid", halo},
		} {
			t.Run(name, func(t *testing.T) {
				out := show(t, tc.nodeID, tc.capabilities)
				if !out.IsError || len(userOnly(out)) != 0 {
					t.Fatalf("result = %+v, want a tool error without glasses blocks", out)
				}
			})
		}
	})
}
