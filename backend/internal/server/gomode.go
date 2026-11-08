// Go Mode conversation focus and workspace-authorized MCP access with session-owned streams.

package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

const goModeMCPEndpoint = "/api/v1/gomode/mcp"

const voiceItemsURI = "gomode://items"

func goModeSettings(version string, voiceEnabled bool) gomode.Settings {
	voice := gomode.VoiceGatewaySettings{Required: false}
	if voiceEnabled {
		voice.URL = "/"
		voice.AuthRequired = true
	}
	return gomode.Settings{
		Service: "mddb", ServiceVersion: version, APIVersion: 1,
		WebShell: gomode.WebShellSettings{
			BridgeVersion: 3,
			ToolGroups: []gomode.ToolGroup{{
				Name: "workspace", Description: "Read and edit documents in your active workspace",
				Endpoint: goModeMCPEndpoint, ProtocolVersion: mcp.ProtocolVersion, AuthRequired: true,
			}},
			VoiceGateway: voice,
		},
	}
}

func goModeMCP(svc *handlers.Services, version string, nodeH *handlers.NodeHandler, focus *voiceFocusState) http.HandlerFunc {
	sessions := mcp.NewSessionStore(goModeSessionOwner)
	return func(w http.ResponseWriter, r *http.Request) {
		u := reqctx.User(r.Context())
		if u == nil {
			writeMCPError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		var wsID ksid.ID
		var role identity.WorkspaceRole
		if raw := r.URL.Query().Get("workspaceId"); raw != "" {
			var err error
			wsID, err = ksid.Parse(raw)
			if err != nil || wsID.IsZero() {
				writeMCPError(w, http.StatusBadRequest, "Invalid workspaceId")
				return
			}
			var msg string
			var status int
			role, msg, status = svc.EffectiveWorkspaceRole(u, wsID)
			if msg != "" {
				writeMCPError(w, status, msg)
				return
			}
		}
		ctx := context.WithValue(r.Context(), goModeWorkspaceKey{}, wsID)
		registry := &workspaceRegistry{svc: svc, nodeH: nodeH, user: u, wsID: wsID, role: role, focus: focus}
		h := &mcp.Handler{Registry: registry, Sessions: sessions, ServerInfo: mcp.Implementation{Name: "mddb", Version: version}}
		h.HandleMCP(w, r.WithContext(ctx))
	}
}

type goModeWorkspaceKey struct{}

func goModeSessionOwner(ctx context.Context) (string, error) {
	u := reqctx.User(ctx)
	id := reqctx.SessionID(ctx)
	if u == nil || id.IsZero() {
		return "", errors.New("authenticated MCP session is missing its owner")
	}
	ws := ctx.Value(goModeWorkspaceKey{}).(ksid.ID)
	return fmt.Sprintf("user:%s/session:%s/workspace:%s", u.ID, id, ws), nil
}

// writeMCPError answers an MCP request that failed before dispatch with a
// JSON-RPC error body. MCP clients parse the response as JSON, so a plain-text
// HTTP error reaches them as an opaque "not valid JSON" failure instead of the
// actual reason.
func writeMCPError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(mcp.JSONRPCResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcp.JSONRPCError{Code: -32001, Message: message}})
}

type workspaceRegistry struct {
	svc   *handlers.Services
	nodeH *handlers.NodeHandler
	user  *identity.User
	wsID  ksid.ID
	role  identity.WorkspaceRole
	focus *voiceFocusState
}

func (m *workspaceRegistry) Instructions(ctx context.Context, meta *mcp.RequestMeta) (string, error) {
	ctx, err := withVoiceSession(ctx, meta)
	if err != nil {
		return "", err
	}
	bound, _ := m.bindWorkspace(ctx)
	if bound == nil {
		return "Use workspaces_list, then workspace_select to choose a workspace for this conversation. Never infer a workspace from another tab.\n\n" + mddbVoiceSystemInstruction, nil
	}
	f := m.focus.lookup(voiceFocusKey{user: m.user.ID, session: conversationSession(ctx)}, bound.wsID)
	if f.node != 0 {
		store, err := bound.svc.FileStore.GetWorkspaceStore(ctx, bound.wsID)
		if err == nil {
			if _, err = store.ReadNodeSummary(ctx, f.node); err == nil {
				return mddbVoiceSystemInstruction + "\nSelected node: " + bound.nodeURI(f.node) + ". Read this resource before answering about it.", nil
			}
		}
	}
	return mddbVoiceSystemInstruction, nil
}

// readSpecs lists the tools every workspace member can call.
func (m *workspaceRegistry) readSpecs() []mcp.ToolSpec {
	list := mcp.NewToolSpec("nodes_list", "List nodes", "List direct children of a workspace node, or root nodes when parentId is omitted.", m.listNodes)
	read := mcp.NewToolSpec("node_read", "Read node", "Read a document or table node by its ID, including markdown content and schema.", m.readNode)
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	list.Annotations = annotations
	read.Annotations = annotations
	workspaces := mcp.NewToolSpec("workspaces_list", "List workspaces", "List accessible workspaces. Select one explicitly when using an unscoped native connection.", m.listWorkspaces)
	selectSpec := mcp.NewToolSpec("workspace_select", "Select workspace", "Choose a workspace for this conversation, independently of other tabs and conversations.", m.selectWorkspace)
	workspaces.Annotations = annotations
	return []mcp.ToolSpec{workspaces, selectSpec, list, read}
}

// writeSpecs lists the mutating tools. They require editor access, and viewers
// never see them so an agent cannot advertise a tool it cannot call.
func (m *workspaceRegistry) writeSpecs() []mcp.ToolSpec {
	update := mcp.NewToolSpec("node_update", "Update page", "Replace a page's title and markdown content. Read the page first: the whole body is overwritten.", m.updateNode)
	page := mcp.NewToolSpec("page_create", "Create text page", "Create a text page with a Markdown body under a parent node, or at the workspace root when parentId is omitted.", m.createPage)
	table := mcp.NewToolSpec("table_create", "Create table", "Create a table with a column schema under a parent node, or at the workspace root when parentId is omitted.", m.createTable)
	appendSpec := mcp.NewToolSpec("node_append", "Append to page", "Append markdown to the end of a page without replacing its body.", m.appendNode)
	annotations := &mcp.ToolAnnotations{DestructiveHint: true}
	update.Annotations = annotations
	page.Annotations = annotations
	table.Annotations = annotations
	appendSpec.Annotations = annotations
	return []mcp.ToolSpec{update, page, table, appendSpec}
}

func (m *workspaceRegistry) allSpecs() []mcp.ToolSpec {
	return append(m.readSpecs(), m.writeSpecs()...)
}

func (m *workspaceRegistry) visibleSpecs() []mcp.ToolSpec {
	canEdit := m.role.CanEdit()
	if m.wsID == 0 && m.user != nil {
		for _, ws := range m.accessibleWorkspaces() {
			if ws.Role == string(identity.WSRoleEditor) || ws.Role == string(identity.WSRoleAdmin) {
				canEdit = true
			}
		}
	}
	if canEdit {
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

func (m *workspaceRegistry) CallTool(ctx context.Context, params *mcp.ToolsCallParams) (mcp.RawToolResult, error) {
	var err error
	ctx, err = withVoiceSession(ctx, &params.Meta)
	if err != nil {
		return mcp.RawToolResult{}, err
	}
	if params.Name != "workspaces_list" && params.Name != "workspace_select" {
		for _, spec := range m.writeSpecs() {
			if spec.Name == params.Name && m.wsID != 0 && !m.role.CanEdit() {
				return mcp.RawToolResult{}, mcp.ErrInvalidParams("tool %s requires editor access", params.Name)
			}
		}
		m, err = m.bindWorkspace(ctx)
		if err != nil {
			return mcp.RawToolResult{}, err
		}
	}
	for _, s := range m.readSpecs() {
		if s.Name == params.Name {
			return s.Handler(ctx, params)
		}
	}
	for _, s := range m.writeSpecs() {
		if s.Name == params.Name {
			if !m.role.CanEdit() {
				return mcp.RawToolResult{}, mcp.ErrInvalidParams("tool %s requires editor access to the active workspace", params.Name)
			}
			return s.Handler(ctx, params)
		}
	}
	return mcp.RawToolResult{}, mcp.ErrInvalidParams("unknown tool: %s", params.Name)
}

func (m *workspaceRegistry) listNodes(ctx context.Context, req *mcp.ToolRequest[mcpschema.ListNodesInput]) mcp.ToolResult[mcpschema.ListNodesOutput] {
	in := req.Arguments
	var parentID ksid.ID
	if in.ParentID != "" {
		var err error
		parentID, err = ksid.Parse(in.ParentID)
		if err != nil {
			return mcp.ToolError[mcpschema.ListNodesOutput]("invalid parentId")
		}
	}
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return mcp.ToolError[mcpschema.ListNodesOutput](err.Error())
	}
	if !parentID.IsZero() {
		if _, err := store.ReadNodeSummary(ctx, parentID); err != nil {
			return mcp.ToolError[mcpschema.ListNodesOutput](err.Error())
		}
	}
	c, err := m.decodeCursor(in.Cursor, "children", parentID, 0, mcpNodePageSize)
	if err != nil {
		return mcp.ToolError[mcpschema.ListNodesOutput](err.Error())
	}
	page, err := store.ListNodeSummaries(ctx, &parentID, c.Last, mcpNodePageSize)
	nodes := page.Nodes
	if err != nil {
		return mcp.ToolError[mcpschema.ListNodesOutput](err.Error())
	}
	out := mcpschema.ListNodesOutput{Nodes: make([]mcpschema.NodeSummary, 0, len(nodes))}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, mcpschema.NodeSummary{ID: n.ID.String(), Title: n.Title, Type: string(n.Type), HasChildren: n.HasChildren})
	}
	if page.Next != 0 {
		c.Last = page.Next
		out.NextCursor = encodeMCPCursor(&c)
	}
	m.publishNode(ctx, parentID)
	return mcp.TypedToolResult(out)
}

func (m *workspaceRegistry) readNode(ctx context.Context, req *mcp.ToolRequest[mcpschema.ReadNodeInput]) mcp.ToolResult[mcpschema.ReadNodeOutput] {
	in := req.Arguments
	id, err := ksid.Parse(in.NodeID)
	if err != nil || id.IsZero() {
		return mcp.ToolError[mcpschema.ReadNodeOutput]("invalid nodeId")
	}
	snap, next, err := m.readSnapshot(ctx, id, in.ContentCursor)
	node := m.snapshotDetail(&snap, next)
	if err != nil {
		return mcp.ToolError[mcpschema.ReadNodeOutput](err.Error())
	}
	m.publishNode(ctx, id)
	return mcp.TypedToolResult(mcpschema.ReadNodeOutput{Node: node})
}

// loadNode reads a node and converts it to the detail shape shared by every
// node tool.
func (m *workspaceRegistry) loadNode(ctx context.Context, id ksid.ID) (mcpschema.NodeDetail, error) {
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return mcpschema.NodeDetail{}, err
	}
	snap, err := store.ReadPageSnapshot(ctx, id, 0, mcpContentPageSize)
	if err != nil {
		return mcpschema.NodeDetail{}, err
	}
	return m.snapshotDetail(&snap, m.snapshotNext(&snap)), nil
}

func nodeDetailFrom(node *content.Node) mcpschema.NodeDetail {
	properties := make([]mcpschema.NodeProperty, 0, len(node.Properties))
	for i := range node.Properties {
		properties = append(properties, mcpschema.NodeProperty{Name: node.Properties[i].Name, Type: string(node.Properties[i].Type)})
	}
	return mcpschema.NodeDetail{
		ID: node.ID.String(), ParentID: node.ParentID.String(), Title: node.Title,
		Type: string(node.Type), Content: node.Content, Properties: properties,
		Modified: node.Modified.AsTime().Format(time.RFC3339Nano),
	}
}

func (m *workspaceRegistry) updateNode(ctx context.Context, req *mcp.ToolRequest[mcpschema.UpdateNodeInput]) mcp.ToolResult[mcpschema.WriteNodeOutput] {
	in := req.Arguments
	id, err := ksid.Parse(in.NodeID)
	if err != nil || id.IsZero() {
		return mcp.ToolError[mcpschema.WriteNodeOutput]("invalid nodeId")
	}
	request := &dto.UpdatePageRequest{WsID: m.wsID, ID: id, Title: in.Title, Content: in.Content, ExpectedRevision: in.ExpectedRevision}
	if err := request.Validate(); err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	node, err := m.nodeH.ReplacePage(ctx, m.wsID, m.user, request)
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	return m.writtenResult(ctx, node)
}

func (m *workspaceRegistry) createPage(ctx context.Context, req *mcp.ToolRequest[mcpschema.CreatePageInput]) mcp.ToolResult[mcpschema.WriteNodeOutput] {
	in := req.Arguments
	var parentID ksid.ID
	if in.ParentID != "" {
		id, err := ksid.Parse(in.ParentID)
		if err != nil || id.IsZero() {
			return mcp.ToolError[mcpschema.WriteNodeOutput]("invalid parentId")
		}
		parentID = id
	}
	resp, err := m.nodeH.CreatePage(ctx, m.wsID, m.user, &dto.CreatePageRequest{ParentID: parentID, Title: in.Title, Content: in.Content})
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	node, err := m.loadNode(ctx, resp.ID)
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	m.publishNode(ctx, resp.ID)
	return mcp.TypedToolResult(mcpschema.WriteNodeOutput{Node: node})
}

func (m *workspaceRegistry) createTable(ctx context.Context, req *mcp.ToolRequest[mcpschema.CreateTableInput]) mcp.ToolResult[mcpschema.WriteNodeOutput] {
	in := req.Arguments
	var parentID ksid.ID
	if in.ParentID != "" {
		id, err := ksid.Parse(in.ParentID)
		if err != nil || id.IsZero() {
			return mcp.ToolError[mcpschema.WriteNodeOutput]("invalid parentId")
		}
		parentID = id
	}
	resp, err := m.nodeH.CreateTable(ctx, m.wsID, m.user, &dto.CreateTableRequest{ParentID: parentID, Title: in.Title, Properties: in.Properties})
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	node, err := m.loadNode(ctx, resp.ID)
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	m.publishNode(ctx, resp.ID)
	return mcp.TypedToolResult(mcpschema.WriteNodeOutput{Node: node})
}

func (m *workspaceRegistry) appendNode(ctx context.Context, req *mcp.ToolRequest[mcpschema.AppendNodeInput]) mcp.ToolResult[mcpschema.WriteNodeOutput] {
	in := req.Arguments
	id, err := ksid.Parse(in.NodeID)
	if err != nil || id.IsZero() {
		return mcp.ToolError[mcpschema.WriteNodeOutput]("invalid nodeId")
	}
	if strings.TrimSpace(in.Content) == "" {
		return mcp.ToolError[mcpschema.WriteNodeOutput]("content is required")
	}
	request := &dto.AppendPageRequest{WsID: m.wsID, ID: id, Content: in.Content, ExpectedRevision: in.ExpectedRevision}
	if err := request.Validate(); err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	node, err := m.nodeH.AppendToPage(ctx, m.wsID, m.user, request)
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	return m.writtenResult(ctx, node)
}

func (m *workspaceRegistry) ListResources(ctx context.Context, raw string) (mcp.ResourcesListResult, error) {
	if err := ctx.Err(); err != nil {
		return mcp.ResourcesListResult{}, err
	}
	c, err := m.decodeCursor(raw, "resources", 0, 0, mcpNodePageSize)
	if err != nil {
		return mcp.ResourcesListResult{}, mcp.ErrInvalidParams("%s", err)
	}
	resources := []mcp.ResourceDescriptor{}
	if raw == "" {
		resources = append(resources, m.itemsResource())
	}
	out := mcp.ResourcesListResult{ResultType: mcp.ResultTypeComplete, Resources: resources, TTLMS: mcp.DefaultTTLMS, CacheScope: mcp.CacheScopePrivate}
	// The unscoped native catalog advertises generic context only. Session-aware
	// nodes_list owns discovery after workspace_select.
	if m.wsID == 0 {
		if raw != "" {
			return out, mcp.ErrInvalidParams("unscoped catalog has no continuation")
		}
		return out, nil
	}
	bound, err := m.bindWorkspace(ctx)
	if err != nil {
		return out, err
	}
	m = bound
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return out, err
	}
	page, err := store.ListNodeSummaries(ctx, nil, c.Last, mcpNodePageSize)
	if err != nil {
		return out, err
	}
	for _, n := range page.Nodes {
		out.Resources = append(out.Resources, m.resource(n))
	}
	if page.Next != 0 {
		c.Last = page.Next
		out.NextCursor = encodeMCPCursor(&c)
	}
	return out, nil
}

func (m *workspaceRegistry) itemsResource() mcp.ResourceDescriptor {
	return mcp.ResourceDescriptor{URI: voiceItemsURI, Name: "items", Title: "Workspace nodes and current selection", MimeType: "application/json"}
}

func (m *workspaceRegistry) Resources(ctx context.Context) iter.Seq2[mcp.ResourceDescriptor, error] {
	return func(yield func(mcp.ResourceDescriptor, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(mcp.ResourceDescriptor{}, err)
			return
		}
		// Advertise setup context before touching an arbitrarily large node catalog.
		if !yield(m.itemsResource(), nil) || m.wsID == 0 {
			return
		}
		raw := ""
		for {
			page, err := m.ListResources(ctx, raw)
			if err != nil {
				yield(mcp.ResourceDescriptor{}, err)
				return
			}
			for i := range page.Resources {
				r := page.Resources[i]
				if r.URI != voiceItemsURI && !yield(r, nil) {
					return
				}
			}
			if page.NextCursor == "" {
				return
			}
			raw = page.NextCursor
		}
	}
}

func (m *workspaceRegistry) ReadResource(ctx context.Context, params *mcp.ResourcesReadParams) (mcp.ResourcesReadResult, error) {
	var err error
	ctx, err = withVoiceSession(ctx, &params.Meta)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	uri := params.URI
	if uri == voiceItemsURI {
		bound, err := m.bindWorkspace(ctx)
		if err != nil {
			return mcp.ResourceJSON(uri, mcpschema.ServiceItemsOutput{Items: []mcpschema.ServiceItem{}})
		}
		items, err := bound.serviceItems(ctx)
		if err != nil {
			return mcp.ResourcesReadResult{}, err
		}
		return mcp.ResourceJSON(uri, items)
	}
	ws, id, raw, err := parseNodeResource(uri)
	if err != nil {
		return mcp.ResourcesReadResult{}, mcp.ErrInvalidParams("%s", err)
	}
	if m.wsID != 0 && ws != m.wsID {
		return mcp.ResourcesReadResult{}, mcp.ErrInvalidParams("resource is outside active workspace")
	}
	bound := *m
	bound.wsID = ws
	m, err = bound.bindWorkspace(ctx)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	snap, next, err := m.readSnapshot(ctx, id, raw)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	node, err := m.snapshotResource(&snap, next)
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
func (m *workspaceRegistry) SubscribeResourceUpdates(ctx context.Context, params *mcp.SubscriptionsListenParams) (iter.Seq2[mcp.ResourceUpdate, error], error) {
	ctx, err := withVoiceSession(ctx, &params.Meta)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.svc.Broker == nil {
		return nil, mcp.ErrInvalidParams("resource subscriptions are unavailable")
	}
	filter := params.Notifications
	if len(filter.ResourceSubscriptions) > 128 {
		return nil, mcp.ErrInvalidParams("too many resource subscriptions")
	}
	base := m
	bound, e := base.bindWorkspace(ctx)
	if e == nil {
		m = bound
	}
	subscribed := make(map[string]struct{}, len(filter.ResourceSubscriptions))
	for _, uri := range filter.ResourceSubscriptions {
		if len(uri) > 1024 {
			return nil, mcp.ErrInvalidParams("resource URI exceeds limit")
		}
		if uri != voiceItemsURI {
			ws, _, raw, e := parseNodeResource(uri)
			if e != nil || raw != "" || ws != m.wsID {
				return nil, mcp.ErrInvalidParams("subscription is outside active workspace")
			}
		}
		subscribed[uri] = struct{}{}
	}
	// Registration precedes the handler's initial reads. Cancellation owns cleanup
	// even if the returned iterator is never started.
	events, unsubscribe := m.svc.Broker.SubscribeEvents(m.wsID)
	focusEvents, unsubscribeFocus := m.focus.subscribe(ctx, voiceFocusKey{user: m.user.ID, session: conversationSession(ctx)})
	var mu sync.Mutex
	closed := false
	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		closed = true
		unsubscribe()
		unsubscribeFocus()
	}
	withdraw := func() mcp.ResourceUpdate {
		update := mcp.ResourceUpdate{ResourcesListChanged: filter.ResourcesListChanged}
		for uri := range subscribed {
			update.ResourceURIs = append(update.ResourceURIs, uri)
		}
		return update
	}
	stop := context.AfterFunc(ctx, cleanup)
	if err := ctx.Err(); err != nil {
		stop()
		cleanup()
		return nil, err
	}
	return func(yield func(mcp.ResourceUpdate, error) bool) {
		defer func() { stop(); cleanup() }()
		ticker := time.NewTicker(subscriptionKeepAlive)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-focusEvents:
				bound, e := base.bindWorkspace(ctx)
				if e != nil && base.wsID != 0 {
					yield(withdraw(), nil)
					return
				}
				if e != nil {
					bound = base
				}
				update := mcp.ResourceUpdate{}
				if bound.wsID != m.wsID {
					mu.Lock()
					if closed {
						mu.Unlock()
						return
					}
					unsubscribe()
					m = bound
					events, unsubscribe = m.svc.Broker.SubscribeEvents(m.wsID)
					mu.Unlock()
					// Withdraw old workspace resources, including nodes outside the visible page.
					for uri := range subscribed {
						if uri != voiceItemsURI {
							update.ResourceURIs = append(update.ResourceURIs, uri)
							delete(subscribed, uri)
						}
					}
					update.ResourcesListChanged = filter.ResourcesListChanged
				}
				if _, want := subscribed[voiceItemsURI]; want {
					update.ResourceURIs = append(update.ResourceURIs, voiceItemsURI)
				}
				if (update.ResourcesListChanged || len(update.ResourceURIs) > 0) && !yield(update, nil) {
					return
				}
			case evt, ok := <-events:
				if !ok {
					return
				}
				if m.wsID != 0 {
					if _, msg, _ := m.svc.EffectiveWorkspaceRole(m.user, m.wsID); msg != "" {
						yield(withdraw(), nil)
						return
					}
				}
				update := mcp.ResourceUpdate{ResourcesListChanged: filter.ResourcesListChanged && nodeEventChangesTree(evt.Type)}
				if _, want := subscribed[voiceItemsURI]; want {
					update.ResourceURIs = append(update.ResourceURIs, voiceItemsURI)
				}
				if evt.Type == dto.EventNodeDeleted {
					store, e := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
					if e != nil {
						yield(mcp.ResourceUpdate{}, e)
						return
					}
					for uri := range subscribed {
						if uri == voiceItemsURI {
							continue
						}
						_, id, _, e := parseNodeResource(uri)
						if e != nil {
							continue
						}
						if _, e = store.ReadNodeSummary(ctx, id); errors.Is(e, content.ErrNodeNotFound) {
							update.ResourceURIs = append(update.ResourceURIs, uri)
							delete(subscribed, uri)
						} else if e != nil {
							yield(mcp.ResourceUpdate{}, e)
							return
						}
					}
				} else if _, want := subscribed[m.nodeURI(evt.NodeID)]; want {
					update.ResourceURIs = append(update.ResourceURIs, m.nodeURI(evt.NodeID))
				}
				if (update.ResourcesListChanged || len(update.ResourceURIs) > 0) && !yield(update, nil) {
					return
				}
			case <-ticker.C:
				if m.wsID != 0 {
					if _, msg, _ := m.svc.EffectiveWorkspaceRole(m.user, m.wsID); msg != "" {
						yield(withdraw(), nil)
						return
					}
				}
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

func (m *workspaceRegistry) accessibleWorkspaces() []mcpschema.WorkspaceSummary {
	out := []mcpschema.WorkspaceSummary{}
	for ws := range m.svc.Workspace.Iter(0) {
		role, msg, _ := m.svc.EffectiveWorkspaceRole(m.user, ws.ID)
		if msg == "" {
			out = append(out, mcpschema.WorkspaceSummary{ID: ws.ID.String(), Title: ws.Name, Role: string(role)})
		}
	}
	return out
}

func (m *workspaceRegistry) bindWorkspace(ctx context.Context) (*workspaceRegistry, error) {
	id := m.wsID
	if id == 0 {
		if session := conversationSession(ctx); session != "" {
			id = m.focus.workspace(voiceFocusKey{user: m.user.ID, session: session})
		}
		if id == 0 {
			workspaces := m.accessibleWorkspaces()
			if len(workspaces) != 1 {
				return nil, mcp.ErrInvalidParams("select a workspace with workspace_select first")
			}
			id, _ = ksid.Parse(workspaces[0].ID)
		}
	}
	role, msg, _ := m.svc.EffectiveWorkspaceRole(m.user, id)
	if msg != "" {
		return nil, mcp.ErrInvalidParams("%s", msg)
	}
	bound := *m
	bound.wsID = id
	bound.role = role
	return &bound, nil
}

func (m *workspaceRegistry) listWorkspaces(context.Context, *mcp.ToolRequest[mcpschema.WorkspaceListInput]) mcp.ToolResult[mcpschema.WorkspaceListOutput] {
	return mcp.TypedToolResult(mcpschema.WorkspaceListOutput{Workspaces: m.accessibleWorkspaces()})
}

// Native discovery has no tab identity. Multiple accessible workspaces require
// an explicit conversation choice rather than the user's shared workspace LRU.
func (m *workspaceRegistry) selectWorkspace(ctx context.Context, req *mcp.ToolRequest[mcpschema.WorkspaceSelectInput]) mcp.ToolResult[mcpschema.WorkspaceSelectOutput] {
	if conversationSession(ctx) == "" {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput]("workspace_select requires openai/session")
	}
	id, err := ksid.Parse(req.Arguments.WorkspaceID)
	if err != nil || id == 0 {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput]("invalid workspaceId")
	}
	if m.wsID != 0 && id != m.wsID {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput]("workspace is outside this connection's scope")
	}
	role, msg, _ := m.svc.EffectiveWorkspaceRole(m.user, id)
	if msg != "" {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput](msg)
	}
	ws, err := m.svc.Workspace.Get(id)
	if err != nil {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput](err.Error())
	}
	if _, applied, err := m.focus.set(ctx, voiceFocusKey{user: m.user.ID, session: conversationSession(ctx)}, id, 0, nil, voiceFocusTool); err != nil {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput](err.Error())
	} else if !applied {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput]("browser selection owns this conversation's focus")
	}
	return mcp.TypedToolResult(mcpschema.WorkspaceSelectOutput{Workspace: mcpschema.WorkspaceSummary{ID: id.String(), Title: ws.Name, Role: string(role)}})
}

func (m *workspaceRegistry) publishNode(ctx context.Context, id ksid.ID) {
	if session := conversationSession(ctx); session != "" {
		_, _, _ = m.focus.set(ctx, voiceFocusKey{user: m.user.ID, session: session}, m.wsID, id, nil, voiceFocusTool)
	}
}

func (m *workspaceRegistry) serviceItems(ctx context.Context) (mcpschema.ServiceItemsOutput, error) {
	const limit = 20
	out := mcpschema.ServiceItemsOutput{Items: []mcpschema.ServiceItem{}}
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return out, err
	}
	root := ksid.ID(0)
	page, err := store.ListNodeSummaries(ctx, &root, 0, limit)
	nodes := page.Nodes
	if err != nil {
		return out, err
	}
	activeRoot := false
	f := m.focus.lookup(voiceFocusKey{user: m.user.ID, session: conversationSession(ctx)}, m.wsID)
	if f.node != 0 {
		if node, e := store.ReadNodeSummary(ctx, f.node); e == nil {
			activeRoot = node.ParentID == 0
			out.Items = append(out.Items, mcpschema.ServiceItem{ID: node.ID.String(), Reference: m.nodeURI(node.ID), Title: node.Title, State: string(node.Type), Active: true})
		}
	}
	for _, node := range nodes {
		if node.ID == f.node {
			continue
		}
		if len(out.Items) == limit {
			break
		}
		out.Items = append(out.Items, mcpschema.ServiceItem{ID: node.ID.String(), Reference: m.nodeURI(node.ID), Title: node.Title, State: string(node.Type)})
	}
	rootIncluded := 0
	for _, item := range out.Items {
		if !item.Active {
			rootIncluded++
		}
	}
	if activeRoot {
		rootIncluded++
	}
	out.OmittedCount = page.Total - rootIncluded
	if out.OmittedCount > 0 {
		out.MoreItemsHint = fmt.Sprintf("%d more root nodes; use nodes_list to discover them.", out.OmittedCount)
	}
	return out, nil
}

const mddbVoiceSystemInstruction = "You are the voice assistant for the authenticated user's " +
	"active mddb workspace.\n\n" +
	"Use nodes_list to discover pages and tables. " +
	"Use node_read before answering questions about a page's contents, and answer only from its result. Follow nextContentCursor or nextContentURI until complete before replacing a long page. " +
	"Editors can use page_create for text pages with Markdown content and table_create for tables with a column schema. " +
	"Editors can update and append pages. Read a page before updating it because node_update replaces " +
	"both its title and its complete Markdown body. Every edit is committed to the workspace git history. " +
	"Refer to pages and tables by title."

func (m *workspaceRegistry) decodeCursor(raw, kind string, parent, node ksid.ID, limit int) (mcpschema.Cursor, error) {
	c := mcpschema.Cursor{Version: 1, Workspace: m.wsID, Kind: kind, Parent: parent, Node: node, Limit: limit}
	if raw == "" {
		return c, nil
	}
	if len(raw) > maxMCPCursorBytes {
		return c, errors.New("cursor exceeds limit")
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return c, errors.New("invalid cursor")
	}
	var got mcpschema.Cursor
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&got); err != nil {
		return c, errors.New("invalid cursor")
	}
	if err = d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return c, errors.New("invalid cursor")
	}
	if got.Version != 1 || got.Workspace != m.wsID || got.Kind != kind || got.Parent != parent || got.Node != node || got.Limit != limit || got.Offset < 0 {
		return c, errors.New("cursor is outside query scope")
	}
	if kind == "content" {
		if got.Last != 0 || got.Offset == 0 || len(got.Revision) != 64 {
			return c, errors.New("invalid content cursor")
		}
	} else if got.Last == 0 || got.Offset != 0 || got.Revision != "" {
		return c, errors.New("invalid list cursor")
	}
	return got, nil
}

func (m *workspaceRegistry) readSnapshot(ctx context.Context, id ksid.ID, raw string) (content.PageSnapshot, string, error) {
	c, err := m.decodeCursor(raw, "content", 0, id, mcpContentPageSize)
	if err != nil {
		return content.PageSnapshot{}, "", err
	}
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return content.PageSnapshot{}, "", err
	}
	snap, err := store.ReadPageSnapshot(ctx, id, c.Offset, mcpContentPageSize)
	if err != nil {
		return snap, "", err
	}
	if raw != "" && snap.Revision != c.Revision {
		return snap, "", errors.New("document changed; restart reading from the first chunk")
	}
	next := m.snapshotNext(&snap)
	return snap, next, nil
}

func (m *workspaceRegistry) snapshotDetail(s *content.PageSnapshot, next string) mcpschema.NodeDetail {
	n := nodeDetailFrom(&s.Node)
	n.Revision = s.Revision
	n.ContentBytes = s.ContentBytes
	n.ContentTruncated = next != ""
	n.NextContentCursor = next
	return n
}

func (m *workspaceRegistry) snapshotResource(s *content.PageSnapshot, next string) (mcpschema.ResourceNode, error) {
	n := &s.Node
	// This explicit schema conversion preserves nested view/config fields and
	// storage.Time's numeric-seconds JSON contract without exposing runtime DTOs.
	b, err := json.Marshal(n)
	if err != nil {
		return mcpschema.ResourceNode{}, err
	}
	var out mcpschema.ResourceNode
	if err := json.Unmarshal(b, &out); err != nil {
		return out, err
	}
	// Resource reads omit expansion state; discovery summaries own child flags.
	out.HasChildren = false
	out.Children = nil
	out.Revision = s.Revision
	out.ContentBytes = s.ContentBytes
	out.ContentTruncated = next != ""
	if next != "" {
		out.NextContentURI = m.nodeURI(n.ID) + "?cursor=" + url.QueryEscape(next)
	}
	return out, nil
}

func (m *workspaceRegistry) snapshotNext(s *content.PageSnapshot) string {
	if s.NextOffset >= s.ContentBytes {
		return ""
	}
	return encodeMCPCursor(&mcpschema.Cursor{Version: 1, Workspace: m.wsID, Kind: "content", Node: s.Node.ID, Limit: mcpContentPageSize, Offset: s.NextOffset, Revision: s.Revision})
}

func (m *workspaceRegistry) writtenResult(ctx context.Context, node *content.Node) mcp.ToolResult[mcpschema.WriteNodeOutput] {
	snap, err := content.PageWindow(node, mcpContentPageSize)
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	m.publishNode(ctx, node.ID)
	return mcp.TypedToolResult(mcpschema.WriteNodeOutput{Node: m.snapshotDetail(&snap, m.snapshotNext(&snap))})
}
