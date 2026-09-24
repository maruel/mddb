// Go Mode discovery and authenticated, workspace-scoped MCP read access.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"strings"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

const goModeMCPEndpoint = "/api/v1/gomode/mcp"

func goModeSettings(version string, voiceEnabled bool) gomode.Settings {
	voice := gomode.VoiceGatewaySettings{Required: false}
	if voiceEnabled {
		voice.URL = "/"
		voice.AuthRequired = true
	}
	return gomode.Settings{
		Service: "mddb", ServiceVersion: version, APIVersion: 1,
		WebShell: gomode.WebShellSettings{
			BridgeVersion: 1,
			ToolGroups: []gomode.ToolGroup{{
				Name: "workspace", Description: "Read documents in your active workspace",
				Endpoint: goModeMCPEndpoint, ProtocolVersion: mcp.ProtocolVersion, AuthRequired: true,
			}},
			VoiceGateway: voice,
		},
	}
}

func goModeMCP(svc *handlers.Services, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := reqctx.User(r.Context())
		if u == nil || len(u.Settings.LastActiveWorkspaces) == 0 {
			http.Error(w, "Select a workspace first", http.StatusConflict)
			return
		}
		wsID := u.Settings.LastActiveWorkspaces[0]
		if msg, status := checkWSMembership(u, wsID, svc, identity.WSRoleViewer); msg != "" {
			http.Error(w, msg, status)
			return
		}
		h := &mcp.Handler{Registry: &workspaceRegistry{svc: svc, wsID: wsID}, ServerInfo: mcp.Implementation{Name: "mddb", Version: version}}
		h.HandleMCP(w, r)
	}
}

type workspaceRegistry struct {
	svc  *handlers.Services
	wsID ksid.ID
}

func (m *workspaceRegistry) Instructions(context.Context) (string, error) {
	return "Read documents in the authenticated user's active mddb workspace. Tools do not modify content.", nil
}

func (m *workspaceRegistry) specs() []mcp.ToolSpec {
	list := mcp.NewToolSpec("nodes_list", "List nodes", "List direct children of a workspace node, or root nodes when parentId is omitted.", m.listNodes)
	read := mcp.NewToolSpec("node_read", "Read node", "Read a document or table node by its ID, including markdown content and schema.", m.readNode)
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	list.Annotations = annotations
	read.Annotations = annotations
	return []mcp.ToolSpec{list, read}
}

// validateToolSchemas fails when an advertised workspace tool schema is
// structurally invalid, so a malformed catalog is caught at startup instead of
// on the request path.
func (m *workspaceRegistry) validateToolSchemas() error {
	for _, spec := range m.specs() {
		if err := mcp.ValidateToolSchema(spec.InputSchema); err != nil {
			return fmt.Errorf("MCP tool %s input schema: %w", spec.Name, err)
		}
		if err := mcp.ValidateToolSchema(spec.OutputSchema); err != nil {
			return fmt.Errorf("MCP tool %s output schema: %w", spec.Name, err)
		}
	}
	return nil
}

func (m *workspaceRegistry) Tools(context.Context) ([]mcp.ToolDescriptor, error) {
	specs := m.specs()
	out := make([]mcp.ToolDescriptor, 0, len(specs))
	for _, s := range specs {
		out = append(out, mcp.ToolDescriptor{Name: s.Name, Title: s.Title, Description: s.Description, InputSchema: s.InputSchema, OutputSchema: s.OutputSchema, Annotations: s.Annotations})
	}
	return out, nil
}

func (m *workspaceRegistry) CallTool(ctx context.Context, name string, args json.RawMessage) (mcp.RawToolResult, error) {
	for _, s := range m.specs() {
		if s.Name == name {
			return s.Handler(ctx, args)
		}
	}
	return mcp.RawToolResult{}, mcp.ErrInvalidParams("unknown tool: %s", name)
}

func (m *workspaceRegistry) listNodes(ctx context.Context, in listNodesInput) mcp.ToolResult[listNodesOutput] {
	var parentID ksid.ID
	if in.ParentID != "" {
		var err error
		parentID, err = ksid.Parse(in.ParentID)
		if err != nil {
			return mcp.ToolError[listNodesOutput]("invalid parentId")
		}
	}
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return mcp.ToolError[listNodesOutput](err.Error())
	}
	if !parentID.IsZero() {
		if _, err := store.ReadNode(parentID); err != nil {
			return mcp.ToolError[listNodesOutput](err.Error())
		}
	}
	nodes, err := store.ListChildren(parentID)
	if err != nil {
		return mcp.ToolError[listNodesOutput](err.Error())
	}
	out := listNodesOutput{Nodes: make([]nodeSummary, 0, len(nodes))}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, nodeSummary{ID: n.ID.String(), Title: n.Title, Type: string(n.Type), HasChildren: n.HasChildren})
	}
	return mcp.TypedToolResult(out)
}

func (m *workspaceRegistry) readNode(ctx context.Context, in readNodeInput) mcp.ToolResult[readNodeOutput] {
	id, err := ksid.Parse(in.NodeID)
	if err != nil || id.IsZero() {
		return mcp.ToolError[readNodeOutput]("invalid nodeId")
	}
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return mcp.ToolError[readNodeOutput](err.Error())
	}
	node, err := store.ReadNode(id)
	if err != nil {
		return mcp.ToolError[readNodeOutput](err.Error())
	}
	properties := make([]nodeProperty, 0, len(node.Properties))
	for i := range node.Properties {
		properties = append(properties, nodeProperty{Name: node.Properties[i].Name, Type: string(node.Properties[i].Type)})
	}
	return mcp.TypedToolResult(readNodeOutput{Node: nodeDetail{
		ID: node.ID.String(), ParentID: node.ParentID.String(), Title: node.Title,
		Type: string(node.Type), Content: node.Content, Properties: properties,
	}})
}

func (m *workspaceRegistry) ListResources(ctx context.Context, cursor string) (mcp.ResourcesListResult, error) {
	if cursor != "" {
		return mcp.ResourcesListResult{}, mcp.ErrInvalidParams("invalid cursor")
	}
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return mcp.ResourcesListResult{}, err
	}
	resources := make([]mcp.ResourceDescriptor, 0)
	parents := []ksid.ID{0}
	for len(parents) > 0 {
		if err := ctx.Err(); err != nil {
			return mcp.ResourcesListResult{}, err
		}
		parentID := parents[0]
		parents = parents[1:]
		nodes, err := store.ListChildren(parentID)
		if err != nil {
			return mcp.ResourcesListResult{}, err
		}
		for _, n := range nodes {
			resources = append(resources, m.resource(n))
			if n.HasChildren {
				parents = append(parents, n.ID)
			}
		}
	}
	return mcp.ResourcesListResult{ResultType: mcp.ResultTypeComplete, Resources: resources, TTLMS: mcp.DefaultTTLMS, CacheScope: mcp.CacheScopePrivate}, nil
}

func (m *workspaceRegistry) Resources(ctx context.Context) iter.Seq2[mcp.ResourceDescriptor, error] {
	return func(yield func(mcp.ResourceDescriptor, error) bool) {
		res, err := m.ListResources(ctx, "")
		if err != nil {
			yield(mcp.ResourceDescriptor{}, err)
			return
		}
		for i := range res.Resources {
			if !yield(res.Resources[i], nil) {
				return
			}
		}
	}
}

func (m *workspaceRegistry) ReadResource(ctx context.Context, uri string) (mcp.ResourcesReadResult, error) {
	prefix := fmt.Sprintf("mddb://workspaces/%s/nodes/", m.wsID)
	if !strings.HasPrefix(uri, prefix) {
		return mcp.ResourcesReadResult{}, mcp.ErrInvalidParams("resource is outside active workspace")
	}
	id, err := ksid.Parse(strings.TrimPrefix(uri, prefix))
	if err != nil || id.IsZero() {
		return mcp.ResourcesReadResult{}, mcp.ErrInvalidParams("invalid node resource")
	}
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	node, err := store.ReadNode(id)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	return mcp.ResourceJSON(uri, node)
}

func (m *workspaceRegistry) resource(n *content.Node) mcp.ResourceDescriptor {
	return mcp.ResourceDescriptor{URI: fmt.Sprintf("mddb://workspaces/%s/nodes/%s", m.wsID, n.ID), Name: n.ID.String(), Title: n.Title, MimeType: "application/json"}
}

type listNodesInput struct {
	ParentID string `json:"parentId,omitempty" jsonschema:"description=Parent node ID; omit for workspace root"`
}

type readNodeInput struct {
	NodeID string `json:"nodeId" jsonschema:"description=Node ID to read"`
}

type nodeSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	HasChildren bool   `json:"hasChildren"`
}

type listNodesOutput struct {
	Nodes []nodeSummary `json:"nodes"`
}

type readNodeOutput struct {
	Node nodeDetail `json:"node"`
}

type nodeDetail struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parentId,omitempty"`
	Title      string         `json:"title"`
	Type       string         `json:"type"`
	Content    string         `json:"content,omitempty"`
	Properties []nodeProperty `json:"properties,omitempty"`
}

type nodeProperty struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
