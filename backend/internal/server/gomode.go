// Go Mode discovery and authenticated, workspace-scoped MCP read and write access.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"time"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
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
				Name: "workspace", Description: "Read and edit documents in your active workspace",
				Endpoint: goModeMCPEndpoint, ProtocolVersion: mcp.ProtocolVersion, AuthRequired: true,
			}},
			VoiceGateway: voice,
		},
	}
}

func goModeMCP(svc *handlers.Services, version string, nodeH *handlers.NodeHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := reqctx.User(r.Context())
		if u == nil {
			writeMCPError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		wsID := svc.ActiveWorkspaceID(u)
		if wsID.IsZero() {
			writeMCPError(w, http.StatusConflict, "Select a workspace first")
			return
		}
		role, msg, status := effectiveWSRole(u, wsID, svc)
		if msg != "" {
			writeMCPError(w, status, msg)
			return
		}
		registry := &workspaceRegistry{svc: svc, nodeH: nodeH, user: u, wsID: wsID, role: role}
		h := &mcp.Handler{Registry: registry, ServerInfo: mcp.Implementation{Name: "mddb", Version: version}}
		h.HandleMCP(w, r)
	}
}

// writeMCPError answers an MCP request that failed before dispatch with a
// JSON-RPC error body. MCP clients parse the response as JSON, so a plain-text
// HTTP error reaches them as an opaque "not valid JSON" failure instead of the
// actual reason.
func writeMCPError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      nil,
		"error":   map[string]any{"code": -32001, "message": message},
	})
}

type workspaceRegistry struct {
	svc   *handlers.Services
	nodeH *handlers.NodeHandler
	user  *identity.User
	wsID  ksid.ID
	role  identity.WorkspaceRole
}

func (m *workspaceRegistry) Instructions(context.Context) (string, error) {
	return "Read documents in the authenticated user's active mddb workspace. Editors can also create and modify pages; every edit is committed to the workspace git history.", nil
}

// readSpecs lists the tools every workspace member can call.
func (m *workspaceRegistry) readSpecs() []mcp.ToolSpec {
	list := mcp.NewToolSpec("nodes_list", "List nodes", "List direct children of a workspace node, or root nodes when parentId is omitted.", m.listNodes)
	read := mcp.NewToolSpec("node_read", "Read node", "Read a document or table node by its ID, including markdown content and schema.", m.readNode)
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	list.Annotations = annotations
	read.Annotations = annotations
	return []mcp.ToolSpec{list, read}
}

// writeSpecs lists the mutating tools. They require editor access, and viewers
// never see them so an agent cannot advertise a tool it cannot call.
func (m *workspaceRegistry) writeSpecs() []mcp.ToolSpec {
	update := mcp.NewToolSpec("node_update", "Update page", "Replace a page's title and markdown content. Read the page first: the whole body is overwritten.", m.updateNode)
	create := mcp.NewToolSpec("node_create", "Create page", "Create a page under a parent node, or at the workspace root when parentId is omitted.", m.createNode)
	appendSpec := mcp.NewToolSpec("node_append", "Append to page", "Append markdown to the end of a page without replacing its body.", m.appendNode)
	annotations := &mcp.ToolAnnotations{DestructiveHint: true}
	update.Annotations = annotations
	create.Annotations = annotations
	appendSpec.Annotations = annotations
	return []mcp.ToolSpec{update, create, appendSpec}
}

func (m *workspaceRegistry) allSpecs() []mcp.ToolSpec {
	return append(m.readSpecs(), m.writeSpecs()...)
}

func (m *workspaceRegistry) visibleSpecs() []mcp.ToolSpec {
	if m.role.CanEdit() {
		return m.allSpecs()
	}
	return m.readSpecs()
}

// validateToolSchemas fails when an advertised workspace tool schema is
// structurally invalid, so a malformed catalog is caught at startup instead of
// on the request path.
func (m *workspaceRegistry) validateToolSchemas() error {
	for _, spec := range m.allSpecs() {
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
	specs := m.visibleSpecs()
	out := make([]mcp.ToolDescriptor, 0, len(specs))
	for _, s := range specs {
		out = append(out, mcp.ToolDescriptor{Name: s.Name, Title: s.Title, Description: s.Description, InputSchema: s.InputSchema, OutputSchema: s.OutputSchema, Annotations: s.Annotations})
	}
	return out, nil
}

func (m *workspaceRegistry) CallTool(ctx context.Context, name string, args json.RawMessage) (mcp.RawToolResult, error) {
	for _, s := range m.readSpecs() {
		if s.Name == name {
			return s.Handler(ctx, args)
		}
	}
	for _, s := range m.writeSpecs() {
		if s.Name == name {
			if !m.role.CanEdit() {
				return mcp.RawToolResult{}, mcp.ErrInvalidParams("tool %s requires editor access to the active workspace", name)
			}
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
	node, err := m.loadNode(ctx, id)
	if err != nil {
		return mcp.ToolError[readNodeOutput](err.Error())
	}
	return mcp.TypedToolResult(readNodeOutput{Node: node})
}

// loadNode reads a node and converts it to the detail shape shared by every
// node tool.
func (m *workspaceRegistry) loadNode(ctx context.Context, id ksid.ID) (nodeDetail, error) {
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return nodeDetail{}, err
	}
	node, err := store.ReadNode(id)
	if err != nil {
		return nodeDetail{}, err
	}
	return nodeDetailFrom(node), nil
}

func nodeDetailFrom(node *content.Node) nodeDetail {
	properties := make([]nodeProperty, 0, len(node.Properties))
	for i := range node.Properties {
		properties = append(properties, nodeProperty{Name: node.Properties[i].Name, Type: string(node.Properties[i].Type)})
	}
	return nodeDetail{
		ID: node.ID.String(), ParentID: node.ParentID.String(), Title: node.Title,
		Type: string(node.Type), Content: node.Content, Properties: properties,
		Modified: node.Modified.AsTime().Format(time.RFC3339Nano),
	}
}

func (m *workspaceRegistry) updateNode(ctx context.Context, in updateNodeInput) mcp.ToolResult[writeNodeOutput] {
	id, err := ksid.Parse(in.NodeID)
	if err != nil || id.IsZero() {
		return mcp.ToolError[writeNodeOutput]("invalid nodeId")
	}
	if _, err := m.nodeH.UpdatePage(ctx, m.wsID, m.user, &dto.UpdatePageRequest{ID: id, Title: in.Title, Content: in.Content}); err != nil {
		return mcp.ToolError[writeNodeOutput](err.Error())
	}
	node, err := m.loadNode(ctx, id)
	if err != nil {
		return mcp.ToolError[writeNodeOutput](err.Error())
	}
	return mcp.TypedToolResult(writeNodeOutput{Node: node})
}

func (m *workspaceRegistry) createNode(ctx context.Context, in createNodeInput) mcp.ToolResult[writeNodeOutput] {
	var parentID ksid.ID
	if in.ParentID != "" {
		id, err := ksid.Parse(in.ParentID)
		if err != nil || id.IsZero() {
			return mcp.ToolError[writeNodeOutput]("invalid parentId")
		}
		parentID = id
	}
	resp, err := m.nodeH.CreatePage(ctx, m.wsID, m.user, &dto.CreatePageRequest{ParentID: parentID, Title: in.Title, Content: in.Content})
	if err != nil {
		return mcp.ToolError[writeNodeOutput](err.Error())
	}
	node, err := m.loadNode(ctx, resp.ID)
	if err != nil {
		return mcp.ToolError[writeNodeOutput](err.Error())
	}
	return mcp.TypedToolResult(writeNodeOutput{Node: node})
}

func (m *workspaceRegistry) appendNode(ctx context.Context, in appendNodeInput) mcp.ToolResult[writeNodeOutput] {
	id, err := ksid.Parse(in.NodeID)
	if err != nil || id.IsZero() {
		return mcp.ToolError[writeNodeOutput]("invalid nodeId")
	}
	if strings.TrimSpace(in.Content) == "" {
		return mcp.ToolError[writeNodeOutput]("content is required")
	}
	node, err := m.loadNode(ctx, id)
	if err != nil {
		return mcp.ToolError[writeNodeOutput](err.Error())
	}
	if node.Type == string(content.NodeTypeTable) {
		return mcp.ToolError[writeNodeOutput]("node is a table; node_append only writes to pages")
	}
	merged := in.Content
	if node.Content != "" {
		merged = strings.TrimRight(node.Content, "\n") + "\n\n" + in.Content
	}
	if _, err := m.nodeH.UpdatePage(ctx, m.wsID, m.user, &dto.UpdatePageRequest{ID: id, Title: node.Title, Content: merged}); err != nil {
		return mcp.ToolError[writeNodeOutput](err.Error())
	}
	updated, err := m.loadNode(ctx, id)
	if err != nil {
		return mcp.ToolError[writeNodeOutput](err.Error())
	}
	return mcp.TypedToolResult(writeNodeOutput{Node: updated})
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

// subscriptionKeepAlive is the interval between MCP subscription heartbeats that
// hold a long-lived subscriptions/listen stream open.
const subscriptionKeepAlive = 30 * time.Second

// nodeURI renders the MCP resource URI of a node in the active workspace.
func (m *workspaceRegistry) nodeURI(id ksid.ID) string {
	return fmt.Sprintf("mddb://workspaces/%s/nodes/%s", m.wsID, id)
}

// SubscribeResourceUpdates bridges the workspace SSE broker to MCP resource
// subscriptions, so an MCP client learns when a subscribed node changes instead
// of polling or overwriting a concurrent edit. The handler dedups notifications
// by resource content, so this may report any event that can touch a node.
func (m *workspaceRegistry) SubscribeResourceUpdates(ctx context.Context, filter mcp.SubscriptionFilter) (iter.Seq2[mcp.ResourceUpdate, error], error) {
	if m.svc.Broker == nil {
		return nil, mcp.ErrInvalidParams("resource subscriptions are unavailable")
	}
	events, unsubscribe := m.svc.Broker.SubscribeEvents(m.wsID)
	subscribed := make(map[string]struct{}, len(filter.ResourceSubscriptions))
	for _, uri := range filter.ResourceSubscriptions {
		subscribed[uri] = struct{}{}
	}
	return func(yield func(mcp.ResourceUpdate, error) bool) {
		defer unsubscribe()
		ticker := time.NewTicker(subscriptionKeepAlive)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-events:
				if !ok {
					return
				}
				update := mcp.ResourceUpdate{ResourcesListChanged: filter.ResourcesListChanged && nodeEventChangesTree(evt.Type)}
				if evt.NodeID != 0 {
					if _, want := subscribed[m.nodeURI(evt.NodeID)]; want {
						update.ResourceURIs = []string{m.nodeURI(evt.NodeID)}
					}
				}
				if !update.ResourcesListChanged && len(update.ResourceURIs) == 0 {
					continue
				}
				if !yield(update, nil) {
					return
				}
			case <-ticker.C:
				if !yield(mcp.ResourceUpdate{KeepAlive: true}, nil) {
					return
				}
			}
		}
	}, nil
}

// nodeEventChangesTree reports whether an event can change the workspace's
// resource tree.
func nodeEventChangesTree(eventType dto.EventType) bool {
	switch eventType {
	case dto.EventNodeCreated, dto.EventNodeDeleted, dto.EventNodeMoved:
		return true
	default:
		return false
	}
}

func (m *workspaceRegistry) resource(n *content.Node) mcp.ResourceDescriptor {
	return mcp.ResourceDescriptor{URI: m.nodeURI(n.ID), Name: n.ID.String(), Title: n.Title, MimeType: "application/json"}
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
	Modified   string         `json:"modified,omitempty"`
}

type nodeProperty struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type updateNodeInput struct {
	NodeID  string `json:"nodeId" jsonschema:"description=Page node ID to update"`
	Title   string `json:"title" jsonschema:"description=New page title"`
	Content string `json:"content" jsonschema:"description=Full replacement markdown body"`
}

type createNodeInput struct {
	ParentID string `json:"parentId,omitempty" jsonschema:"description=Parent node ID; omit for workspace root"`
	Title    string `json:"title" jsonschema:"description=Page title"`
	Content  string `json:"content,omitempty" jsonschema:"description=Initial markdown body"`
}

type appendNodeInput struct {
	NodeID  string `json:"nodeId" jsonschema:"description=Page node ID to append to"`
	Content string `json:"content" jsonschema:"description=Markdown appended after the existing body"`
}

type writeNodeOutput struct {
	Node nodeDetail `json:"node"`
}
