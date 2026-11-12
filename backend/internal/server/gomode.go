// Owns mddb public MCP context, authorized workspace resources, and conversation-scoped streams.

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"slices"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

const goModeMCPEndpoint = "/api/v1/gomode/mcp"

const voiceItemsURI = "mddb://catalog"

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

func goModeMCP(svc *handlers.Services, cfg *handlers.Config, nodeH *handlers.NodeHandler, focus *voiceFocusState) http.HandlerFunc {
	sessions := mcp.NewSessionStore(goModeSessionOwner)
	return func(w http.ResponseWriter, r *http.Request) {
		if !validGoModeOrigin(r, cfg.BaseURL) {
			writeMCPError(w, http.StatusForbidden, "Invalid MCP origin")
			return
		}
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
		// Correlation is generated after authentication, never trusted from an
		// actor-supplied hint when reconciling a mutation result with its event.
		ctx = reqctx.WithRequestID(ctx, ksid.NewID().String())
		// Native dispatch rejects missing or mismatched Mcp-Method headers before
		// opening a subscription. Ordinary POST calls need no lifetime binding.
		if r.Method == http.MethodGet || r.Header.Get("Mcp-Method") == string(mcp.MethodSubscriptionsListen) {
			session := reqctx.Session(ctx)
			if session == nil {
				writeMCPError(w, http.StatusUnauthorized, "Authentication required")
				return
			}
			bound, cancel, err := svc.Session.Context(ctx, session)
			if err != nil {
				writeMCPError(w, http.StatusUnauthorized, "Session is no longer active")
				return
			}
			defer cancel()
			ctx = bound
		}
		registry := &workspaceRegistry{svc: svc, nodeH: nodeH, user: u, wsID: wsID, role: role, focus: focus, released: r.Method != http.MethodPost || r.Header.Get("Mcp-Method") == ""}
		h := &mcp.Handler{Registry: registry, Sessions: sessions, ServerInfo: mcp.Implementation{Name: "mddb", Version: cfg.Version}}
		h.HandleMCP(w, r.WithContext(ctx))
	}
}

func validGoModeOrigin(r *http.Request, baseURL string) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 {
		return false
	}
	origin, err := url.Parse(values[0])
	if err != nil || origin.User != nil || origin.Host == "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	base, err := url.Parse(baseURL)
	return err == nil && (origin.Scheme == "http" || origin.Scheme == "https") && origin.Scheme == base.Scheme && strings.EqualFold(origin.Host, base.Host)
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
	// Released GET carries no per-POST conversation metadata.
	released bool
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
	f := m.focus.lookup(voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: conversationSession(ctx)}, bound.wsID)
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
	record := mcp.NewToolSpec("record_read", "Read record", "Read one table record's actual data and watch it independently of the node schema.", m.readRecord)
	query := mcp.NewToolSpec("records_query", "Query records", "Read a bounded data window, optionally using a saved view's filters and ordering. Follow nextURI for more rows; absent records may be outside this window.", m.queryRecords)
	record.Annotations = annotations
	query.Annotations = annotations
	return []mcp.ToolSpec{workspaces, selectSpec, list, read, record, query}
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
			result, err := s.Handler(ctx, params)
			return m.contextualResult(ctx, result), err
		}
	}
	for _, s := range m.writeSpecs() {
		if s.Name == params.Name {
			if !m.role.CanEdit() {
				return mcp.RawToolResult{}, mcp.ErrInvalidParams("tool %s requires editor access to the active workspace", params.Name)
			}
			result, err := s.Handler(ctx, params)
			return m.contextualResult(ctx, result), err
		}
	}
	return mcp.RawToolResult{}, mcp.ErrInvalidParams("unknown tool: %s", params.Name)
}

func (m *workspaceRegistry) contextualResult(ctx context.Context, result mcp.RawToolResult) mcp.RawToolResult {
	if conversationSession(ctx) != "" {
		projection, err := m.conversationContext(ctx)
		if err != nil {
			return result
		}
		if result.Meta == nil {
			result.Meta = mcp.MetaObject{}
		}
		result.Meta[gomode.ConversationContextMetaKey] = projection
	}
	return result
}

func (m *workspaceRegistry) listNodes(ctx context.Context, req *mcp.ToolRequest[mcpschema.ListNodesInput]) mcp.ToolResult[mcpschema.ListNodesOutput] {
	var parent ksid.ID
	var err error
	if req.Arguments.ParentID != "" {
		parent, err = ksid.Parse(req.Arguments.ParentID)
		if err != nil {
			return mcp.ToolError[mcpschema.ListNodesOutput]("invalid parentId")
		}
	}
	out, err := m.childrenSnapshot(ctx, &childrenResource{workspace: m.wsID, parent: parent, cursor: req.Arguments.Cursor})
	if err != nil {
		return mcp.ToolError[mcpschema.ListNodesOutput](err.Error())
	}
	m.publishNode(ctx, parent)
	return mcp.TypedToolResult(out)
}

func (m *workspaceRegistry) readNode(ctx context.Context, req *mcp.ToolRequest[mcpschema.ReadNodeInput]) mcp.ToolResult[mcpschema.ReadNodeOutput] {
	in := req.Arguments
	id, err := ksid.Parse(in.NodeID)
	if err != nil || id.IsZero() {
		return mcp.ToolError[mcpschema.ReadNodeOutput]("invalid nodeId")
	}
	read, err := m.beginResourceRead(ctx, m.nodeURI(id), id, 0, watchNode)
	if err != nil {
		return mcp.ToolError[mcpschema.ReadNodeOutput](err.Error())
	}
	snap, next, err := m.readSnapshot(ctx, id, in.ContentCursor)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.finishResourceRead(ctx, read, "", true)
		}
		return mcp.ToolError[mcpschema.ReadNodeOutput](err.Error())
	}
	node := m.snapshotDetail(&snap, next)
	m.finishResourceRead(ctx, read, snap.Revision, false)
	m.publishNode(ctx, id)
	return mcp.TypedToolResult(mcpschema.ReadNodeOutput{Node: node})
}

// loadNode reads a node and converts it to the detail shape shared by every
// node tool.
func (m *workspaceRegistry) loadNode(ctx context.Context, id ksid.ID) (mcpschema.NodeDetail, error) {
	read, err := m.beginResourceRead(ctx, m.nodeURI(id), id, 0, watchNode)
	if err != nil {
		return mcpschema.NodeDetail{}, err
	}
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return mcpschema.NodeDetail{}, err
	}
	snap, err := store.ReadPageSnapshot(ctx, id, 0, mcpContentPageSize)
	if err != nil {
		return mcpschema.NodeDetail{}, err
	}
	m.finishResourceRead(ctx, read, snap.Revision, false)
	return m.snapshotDetail(&snap, m.snapshotNext(&snap)), nil
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
	read, err := m.beginResourceRead(ctx, m.nodeURI(id), id, 0, watchNode)
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	node, err := m.nodeH.ReplacePage(ctx, m.wsID, m.user, request)
	if err != nil {
		return pageToolError(err)
	}
	return m.writtenResult(ctx, node, read)
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
	read, err := m.beginResourceRead(ctx, m.nodeURI(id), id, 0, watchNode)
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	node, err := m.nodeH.AppendToPage(ctx, m.wsID, m.user, request)
	if err != nil {
		return pageToolError(err)
	}
	return m.writtenResult(ctx, node, read)
}

func pageToolError(err error) mcp.ToolResult[mcpschema.WriteNodeOutput] {
	out := mcpschema.MutationError{Error: err.Error(), Code: dto.ErrorCodeInternal}
	if api, ok := errors.AsType[*dto.APIError](err); ok {
		out.Code = api.Code()
		details := api.Details()
		out.CurrentRevision, _ = details["current_revision"].(string)
		out.Recovery, _ = details["recovery"].(string)
	}
	return mcp.ToolErrorWithMeta[mcpschema.WriteNodeOutput](err.Error(), mcp.MetaObject{
		"com.fghbuild.mddb/mutationError": out,
	})
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
		resources = append(resources, m.itemsResource(), m.contextResource())
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
	return mcp.ResourceDescriptor{Meta: mcp.MetaObject{gomode.ModelContextMetaKey: true}, URI: voiceItemsURI, Name: "items", Title: "Workspace root nodes", MimeType: "application/json"}
}

func (m *workspaceRegistry) contextResource() mcp.ResourceDescriptor {
	return mcp.ResourceDescriptor{Meta: mcp.MetaObject{gomode.ModelContextMetaKey: true}, URI: voiceContextURI, Name: "context", Title: "Conversation focus and watched resources", MimeType: "application/json"}
}

func (m *workspaceRegistry) Resources(ctx context.Context) iter.Seq2[mcp.ResourceDescriptor, error] {
	return func(yield func(mcp.ResourceDescriptor, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(mcp.ResourceDescriptor{}, err)
			return
		}
		// Advertise setup context before touching an arbitrarily large node catalog.
		if !yield(m.itemsResource(), nil) || !yield(m.contextResource(), nil) || m.wsID == 0 {
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
				if r.URI != voiceItemsURI && r.URI != voiceContextURI && !yield(r, nil) {
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
	if strings.Contains(uri, "/children") {
		return m.readChildrenResource(ctx, uri)
	}
	if strings.Contains(uri, "/records") {
		return m.readRecordResource(ctx, uri)
	}
	if uri == voiceContextURI {
		bound, err := m.bindWorkspace(ctx)
		if err != nil {
			m.focus.mu.Lock()
			m.focus.contextVersion++
			state := mcpschema.ConversationContext{Epoch: m.focus.contextEpoch, Version: m.focus.contextVersion}
			m.focus.mu.Unlock()
			projection, projectionErr := projectConversationState(state)
			if projectionErr != nil {
				return mcp.ResourcesReadResult{}, projectionErr
			}
			return contextResource(uri, projection)
		}
		projection, err := bound.conversationContext(ctx)
		if err != nil {
			return mcp.ResourcesReadResult{}, err
		}
		return contextResource(uri, projection)
	}
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
	read, err := m.beginResourceRead(ctx, m.nodeURI(id), id, 0, watchNode)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	snap, next, err := m.readSnapshot(ctx, id, raw)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.finishResourceRead(ctx, read, "", true)
		}
		return mcp.ResourcesReadResult{}, err
	}
	node := m.snapshotDetail(&snap, next)
	m.finishResourceRead(ctx, read, snap.Revision, false)
	result, err := mcp.ResourceJSON(uri, node)
	if read.context != nil {
		if projection, contextErr := m.conversationContext(ctx); contextErr == nil {
			result.Meta = mcp.MetaObject{gomode.ConversationContextMetaKey: projection}
		}
	}
	return result, err
}

// subscriptionKeepAlive is the interval between MCP subscription heartbeats that
// hold a long-lived subscriptions/listen stream open.
const subscriptionKeepAlive = 30 * time.Second

// nodeURI renders the MCP resource URI of a node in the active workspace.
func (m *workspaceRegistry) nodeURI(id ksid.ID) string {
	return fmt.Sprintf("mddb://workspaces/%s/nodes/%s", m.wsID, id)
}

// SubscribeResourceUpdates bridges the workspace SSE broker to MCP resource
// subscriptions. Committed aspects route only affected projections; transport
// forwards these authoritative invalidations without rereading resource bodies.
func (m *workspaceRegistry) SubscribeResourceUpdates(ctx context.Context, params *mcp.SubscriptionsListenParams) (iter.Seq2[mcp.ResourceUpdate, error], error) {
	if m.released && m.wsID == 0 {
		needsWorkspace := params.Meta.Session != ""
		for _, uri := range params.Notifications.ResourceSubscriptions {
			needsWorkspace = needsWorkspace || (uri != voiceItemsURI && uri != voiceContextURI)
		}
		if needsWorkspace {
			return nil, mcp.ErrInvalidParams("Released document subscriptions require an explicit ?workspaceId=<workspace ID> MCP endpoint; GET does not inherit POST conversation metadata")
		}
	}
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
	// A scoped request retains its admission role so setup cannot absorb a
	// concurrent role change before the stream's authorization checks start.
	if base.wsID == 0 {
		if bound, e := base.bindWorkspace(ctx); e == nil {
			m = bound
		}
	}
	subscribed := make(map[string]struct{}, len(filter.ResourceSubscriptions))
	for _, uri := range filter.ResourceSubscriptions {
		if len(uri) > 1024 {
			return nil, mcp.ErrInvalidParams("resource URI exceeds limit")
		}
		if uri != voiceItemsURI && uri != voiceContextURI {
			if query, e := parseChildrenResource(uri); e == nil && query.workspace == m.wsID {
				if _, e := m.decodeCursor(query.cursor, "children", query.parent, 0, mcpNodePageSize); e != nil {
					return nil, mcp.ErrInvalidParams("invalid child-list cursor")
				}
				subscribed[uri] = struct{}{}
				continue
			}
			if record, e := parseRecordResource(uri); e == nil && record.workspace == m.wsID {
				subscribed[uri] = struct{}{}
				continue
			}
			ws, _, raw, e := parseNodeResource(uri)
			if e != nil || raw != "" || ws != m.wsID {
				return nil, mcp.ErrInvalidParams("subscription is outside active workspace")
			}
		}
		subscribed[uri] = struct{}{}
	}
	// Registration precedes the handler's initial reads. Cancellation owns cleanup
	// even if the returned iterator is never started.
	interest := sse.EventInterest{Catalog: filter.ResourcesListChanged}
	for uri := range subscribed {
		if uri == voiceContextURI {
			continue
		}
		if query, e := parseChildrenResource(uri); e == nil {
			interest.Children = append(interest.Children, query.parent)
			continue
		}
		if record, e := parseRecordResource(uri); e == nil {
			interest.Records = append(interest.Records, record.node)
			continue
		}
		if uri == voiceItemsURI {
			interest.Summary = true
			continue
		}
		_, id, _, _ := parseNodeResource(uri)
		interest.Nodes = append(interest.Nodes, id)
	}
	feedCtx, unsubscribe := context.WithCancel(ctx)
	events, err := m.svc.Broker.Subscribe(feedCtx, m.wsID, 0, &interest)
	if err != nil {
		unsubscribe()
		return nil, err
	}
	focusEvents, unsubscribeFocus := m.focus.subscribe(ctx, voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: conversationSession(ctx)})
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
		// A membership event may have been published before registration.
		if !m.streamAuthorized() {
			yield(withdraw(), nil)
			return
		}
		pending := newResourceInvalidations()
		defer pending.stop()
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
				if bound.wsID == m.wsID && bound.role != m.role {
					yield(withdraw(), nil)
					return
				}
				update := mcp.ResourceUpdate{}
				workspaceChanged := bound.wsID != m.wsID
				if workspaceChanged {
					mu.Lock()
					if closed {
						mu.Unlock()
						return
					}
					unsubscribe()
					m = bound
					feedCtx, unsubscribe = context.WithCancel(ctx)
					events, err = m.svc.Broker.Subscribe(feedCtx, m.wsID, 0, &sse.EventInterest{Catalog: filter.ResourcesListChanged, Summary: interest.Summary})
					if err != nil {
						mu.Unlock()
						return
					}
					mu.Unlock()
					// Withdraw old workspace resources, including nodes outside the visible page.
					for uri := range subscribed {
						if uri != voiceItemsURI && uri != voiceContextURI {
							update.ResourceURIs = append(update.ResourceURIs, uri)
							delete(subscribed, uri)
						}
					}
					update.ResourcesListChanged = filter.ResourcesListChanged
				}
				if _, want := subscribed[voiceItemsURI]; want && workspaceChanged {
					update.ResourceURIs = append(update.ResourceURIs, voiceItemsURI)
				}
				if _, want := subscribed[voiceContextURI]; want {
					update.ResourceURIs = append(update.ResourceURIs, voiceContextURI)
				}
				if workspaceChanged {
					update = pending.add(update, true)
				}
				if (update.ResourcesListChanged || len(update.ResourceURIs) > 0) && !yield(update, nil) {
					return
				}
			case evt, ok := <-events.Events:
				if !ok {
					return
				}
				if !m.streamAuthorized() {
					yield(withdraw(), nil)
					return
				}
				gap := evt.Type == dto.EventResync
				update := mcp.ResourceUpdate{ResourcesListChanged: filter.ResourcesListChanged && (gap || slices.Contains(evt.Aspects, dto.EventAspectCatalog))}
				if _, want := subscribed[voiceItemsURI]; want && (gap || slices.Contains(evt.Aspects, dto.EventAspectSummary)) {
					update.ResourceURIs = append(update.ResourceURIs, voiceItemsURI)
				}
				for uri := range subscribed {
					if query, e := parseChildrenResource(uri); e == nil {
						if sse.ChildrenResourceChanged(&evt.WorkspaceEvent, query.parent, nil) {
							update.ResourceURIs = append(update.ResourceURIs, uri)
						}
						continue
					}
					if record, e := parseRecordResource(uri); e == nil {
						if sse.RecordResourceChanged(&evt.WorkspaceEvent, record.node, record.record) {
							update.ResourceURIs = append(update.ResourceURIs, uri)
						}
						continue
					}
					_, id, _, _ := parseNodeResource(uri)
					if uri != voiceItemsURI && uri != voiceContextURI && sse.NodeResourceChanged(&evt.WorkspaceEvent, id) {
						update.ResourceURIs = append(update.ResourceURIs, uri)
					}
				}
				update = pending.add(update, gap)
				if (update.ResourcesListChanged || len(update.ResourceURIs) > 0) && !yield(update, nil) {
					return
				}
			case <-pending.timerC:
				if !m.streamAuthorized() {
					yield(withdraw(), nil)
					return
				}
				update := pending.flush()
				if (update.ResourcesListChanged || len(update.ResourceURIs) > 0) && !yield(update, nil) {
					return
				}

			case <-ticker.C:
				if !m.streamAuthorized() {
					yield(withdraw(), nil)
					return
				}
				if !yield(mcp.ResourceUpdate{KeepAlive: true}, nil) {
					return
				}
			}
		}
	}, nil
}

// streamAuthorized rechecks current workspace memberships and role before every
// workspace event and heartbeat, including otherwise idle streams.
// Login revocation and expiry cancel the owning request independently.
func (m *workspaceRegistry) streamAuthorized() bool {
	if m.wsID == 0 {
		return true
	}
	role, message, _ := m.svc.EffectiveWorkspaceRole(m.user, m.wsID)
	return message == "" && role == m.role
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
			id = m.focus.workspace(voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: session})
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
	if _, applied, err := m.focus.set(ctx, voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: conversationSession(ctx)}, id, 0, nil, voiceFocusTool); err != nil {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput](err.Error())
	} else if !applied {
		return mcp.ToolError[mcpschema.WorkspaceSelectOutput]("browser selection owns this conversation's focus")
	}
	return mcp.TypedToolResult(mcpschema.WorkspaceSelectOutput{Workspace: mcpschema.WorkspaceSummary{ID: id.String(), Title: ws.Name, Role: string(role)}})
}

func (m *workspaceRegistry) publishNode(ctx context.Context, id ksid.ID) {
	if session := conversationSession(ctx); session != "" {
		_, _, _ = m.focus.set(ctx, voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: session}, m.wsID, id, nil, voiceFocusTool)
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
	for _, node := range nodes {
		out.Items = append(out.Items, mcpschema.ServiceItem{ID: node.ID.String(), Reference: m.nodeURI(node.ID), Title: node.Title, State: string(node.Type)})
	}
	out.OmittedCount = page.Total - len(out.Items)
	if out.OmittedCount > 0 {
		out.MoreItemsHint = fmt.Sprintf("%d more root nodes; use nodes_list to discover them.", out.OmittedCount)
	}
	return out, nil
}

const mddbVoiceSystemInstruction = "You are the voice assistant for the authenticated user's " +
	"active mddb workspace of pages and tables.\n\n" +
	"Use nodes_list to discover pages and tables. " +
	"Use node_read before answering questions about a page's contents, and answer only from its result. Follow nextContentCursor or nextContentURI until complete before replacing a long page. " +
	"Editors can create text pages with page_create and Markdown content, tables with table_create and a column schema, and update and append pages. " +
	"Read a page before updating it because node_update replaces both its title and its complete Markdown body. " +
	"Every edit is committed to the workspace git history. Refer to pages and tables by title. " +
	"The JSON with \"focus\" names the selected node, or null, and lists offscreen resources: read one in state changed before relying on it; unavailable means it failed an availability check. " +
	"The JSON with \"items\" is a bounded workspace catalog; use nodes_list for more."

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
	if next != "" {
		n.NextContentURI = m.nodeURI(s.Node.ID) + "?cursor=" + url.QueryEscape(next)
	}
	return n
}

func (m *workspaceRegistry) snapshotNext(s *content.PageSnapshot) string {
	if s.NextOffset >= s.ContentBytes {
		return ""
	}
	return encodeMCPCursor(&mcpschema.Cursor{Version: 1, Workspace: m.wsID, Kind: "content", Node: s.Node.ID, Limit: mcpContentPageSize, Offset: s.NextOffset, Revision: s.Revision})
}

func (m *workspaceRegistry) writtenResult(ctx context.Context, node *content.Node, read resourceRead) mcp.ToolResult[mcpschema.WriteNodeOutput] {
	snap, err := content.PageWindow(node, mcpContentPageSize)
	if err != nil {
		return mcp.ToolError[mcpschema.WriteNodeOutput](err.Error())
	}
	m.finishResourceRead(ctx, read, snap.Revision, false)
	m.publishNode(ctx, node.ID)
	return mcp.TypedToolResult(mcpschema.WriteNodeOutput{Node: m.snapshotDetail(&snap, m.snapshotNext(&snap))})
}

// beginResourceRead admits a dependency before the source read. The returning
// ticket belongs to this entry incarnation and read version; superseded reads
// cannot reinsert evicted dependencies or overwrite newer reconciliation.
func (m *workspaceRegistry) beginResourceRead(ctx context.Context, uri string, node, record ksid.ID, kind watchedResourceKind) (resourceRead, error) {
	if conversationSession(ctx) == "" {
		return resourceRead{}, nil
	}
	key := voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: conversationSession(ctx)}
	f := m.focus
	if err := ctx.Err(); err != nil {
		return resourceRead{}, err
	}
	f.mu.Lock()
	var current *voiceContext
	if el := f.entries[key]; el != nil {
		v := el.Value.(voiceFocus)
		if v.workspace != 0 && v.workspace != m.wsID {
			f.mu.Unlock()
			return resourceRead{}, errors.New("resource is outside this conversation's workspace; select that workspace explicitly first")
		}
		if v.workspace == m.wsID && v.context != nil {
			if _, err := v.context.feed.Position(); err == nil {
				current = v.context
			} else {
				v.context = nil
				el.Value = v
			}
		}
	}
	f.mu.Unlock()
	if current == nil {
		session := reqctx.Session(ctx)
		if session == nil {
			return resourceRead{}, errors.New("conversation dependencies require an authenticated login session")
		}
		lifetime, release, err := m.svc.Session.Context(context.WithoutCancel(ctx), session)
		if err != nil {
			return resourceRead{}, err
		}
		feed, err := m.svc.Broker.Subscribe(lifetime, m.wsID, 0, &sse.EventInterest{})
		if err != nil {
			release()
			return resourceRead{}, err
		}
		created := &voiceContext{feed: feed, cancel: release}
		f.mu.Lock()
		v := voiceFocus{key: key, workspace: m.wsID}
		if el := f.entries[key]; el != nil {
			v = el.Value.(voiceFocus)
		}
		if v.workspace != 0 && v.workspace != m.wsID {
			f.mu.Unlock()
			release()
			return resourceRead{}, errors.New("conversation workspace changed before dependency registration")
		}
		if v.context != nil {
			if _, err := v.context.feed.Position(); err != nil {
				v.context = nil
			}
		}
		if v.workspace == m.wsID && v.context != nil {
			current = v.context
			f.mu.Unlock()
			release()
		} else if err := ctx.Err(); err != nil {
			f.mu.Unlock()
			release()
			return resourceRead{}, err
		} else {
			if v.context != nil {
				v.context.cancel()
			}
			v.workspace = m.wsID
			v.context = created
			f.putLocked(v)
			current = created
			f.mu.Unlock()
			if !m.svc.StartBackground(lifetime, time.Until(session.ExpiresAt.AsTime()), func(worker context.Context) {
				defer release()
				defer f.retireContext(key, created)
				authority := time.NewTicker(subscriptionKeepAlive)
				defer authority.Stop()
				for {
					select {
					case <-worker.Done():
						return
					case <-authority.C:
						if !m.streamAuthorized() {
							return
						}
					case _, ok := <-feed.Wake:
						if !ok || !m.streamAuthorized() {
							return
						}
						f.mu.Lock()
						f.drainContextLocked(key, created)
						f.mu.Unlock()
					}
				}
			}) {
				release()
				f.mu.Lock()
				if el := f.entries[key]; el != nil {
					value := el.Value.(voiceFocus)
					if value.context == created {
						value.context = nil
						el.Value = value
						f.notifyLocked(key)
					}
				}
				f.mu.Unlock()
				return resourceRead{}, errors.New("service is shutting down")
			}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	el := f.entries[key]
	if el == nil || el.Value.(voiceFocus).context != current || ctx.Err() != nil {
		return resourceRead{}, errors.New("conversation context retired")
	}
	var watch *watchedResource
	for i, old := range current.resources {
		if old.resource.URI == uri {
			watch = old
			current.resources = slices.Delete(current.resources, i, i+1)
			break
		}
	}
	if watch == nil {
		if len(current.resources) == maxVoiceContextResources {
			focus := el.Value.(voiceFocus).node
			i := slices.IndexFunc(current.resources, func(w *watchedResource) bool { return w.node != focus || w.kind != watchNode })
			if i < 0 {
				i = 0
			}
			current.resources = slices.Delete(current.resources, i, i+1)
			current.omitted++
		}
		watch = &watchedResource{pending: make(map[dto.EventAspect]*sse.Change), node: node, record: record, kind: kind, resource: mcpschema.ResourceContext{URI: uri, State: mcpschema.ResourceContextChanged, Dependencies: m.resourceDependencies(node, record, kind, nil)}}
	}
	current.resources = append(current.resources, watch)
	watch.readingChildren = kind == watchChildren
	watch.queuedChildren = nil
	watch.childrenGap = ""
	start, err := current.updateInterestLocked()
	if err != nil {
		return resourceRead{}, err
	}
	// Fold all committed changes through the registration barrier before
	// replacing this read baseline. If the source read fails, its previous
	// snapshot must remain invalidated rather than becoming falsely current.
	f.drainContextLocked(key, current)
	watch.readVersion++
	watch.readStart = start
	watch.request = reqctx.RequestID(ctx)
	watch.pending = make(map[dto.EventAspect]*sse.Change)
	f.notifyLocked(key)
	return resourceRead{context: current, watch: watch, version: watch.readVersion}, nil
}

func (m *workspaceRegistry) finishResourceRead(ctx context.Context, read resourceRead, revision string, unavailable bool) {
	if read.context == nil || ctx.Err() != nil {
		return
	}
	f := m.focus
	key := voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: conversationSession(ctx)}
	f.mu.Lock()
	defer f.mu.Unlock()
	el := f.entries[key]
	if el == nil || el.Value.(voiceFocus).context != read.context || read.watch.readVersion != read.version || !slices.Contains(read.context.resources, read.watch) {
		return
	}
	w := read.watch
	w.readRevision = revision
	w.request = reqctx.RequestID(ctx)
	w.resource.Revision = revision
	w.resource.State = mcpschema.ResourceContextRead
	if unavailable {
		w.resource.State = mcpschema.ResourceContextUnavailable
	} else {
		for _, event := range w.pending {
			if event.Sequence() <= w.readStart || readIncludesChange(w, event) {
				continue
			}
			w.resource.State = mcpschema.ResourceContextChanged
			if watchedResourceRemoved(w, event) {
				w.resource.State = mcpschema.ResourceContextUnavailable
				break
			}
		}
	}
	if !unavailable && w.childrenGap != "" {
		w.resource.State = mcpschema.ResourceContextChanged
		w.resource.ChangeID = w.childrenGap
		w.resource.Aspects = nil
	}
	if w.resource.State != mcpschema.ResourceContextChanged {
		w.resource.Aspects = nil
		w.resource.ChangeID = ""
	}
	f.notifyLocked(key)
}

func (m *workspaceRegistry) conversationState(ctx context.Context) (mcpschema.ConversationContext, error) {
	out := mcpschema.ConversationContext{Resources: []mcpschema.ResourceContext{}}
	key := voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: conversationSession(ctx)}
	f := m.focus
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contextVersion++
	out.Epoch = f.contextEpoch
	out.Version = f.contextVersion
	if el := f.entries[key]; el != nil {
		v := el.Value.(voiceFocus)
		if v.workspace == 0 {
			return out, nil
		}
		if v.workspace != m.wsID {
			return out, errors.New("conversation workspace changed; retry context read")
		}
		if v.node != 0 {
			out.Focus = &mcpschema.ServiceFocus{ID: v.node.String(), URI: m.nodeURI(v.node)}
		}
		if v.context != nil {
			// Screen events can reach HTTP before their context worker runs.
			// Synchronize with the publisher, then fold its queued changes under
			// the same owner lock as the worker before returning freshness.
			if _, err := v.context.feed.Position(); err != nil {
				out.Focus = nil
				return out, nil
			}
			f.drainContextLocked(key, v.context)
			out.Version = f.contextVersion
			out.OmittedCount = v.context.omitted
			for _, w := range v.context.resources {
				resource := w.resource
				resource.Aspects = slices.Clone(resource.Aspects)
				resource.Dependencies = slices.Clone(resource.Dependencies)
				for i := range resource.Dependencies {
					resource.Dependencies[i].URIs = slices.Clone(resource.Dependencies[i].URIs)
					resource.Dependencies[i].Aspects = slices.Clone(resource.Dependencies[i].Aspects)
				}
				if v.node == w.node && w.kind == watchNode && resource.State == mcpschema.ResourceContextUnavailable {
					out.Focus = nil
				}
				out.Resources = append(out.Resources, resource)
			}
		}
	}
	return out, nil
}

// resourceDependencies describes the same projection effects as broker routing,
// allowing the existing screen feed to wake only affected conversation watches.
func (m *workspaceRegistry) resourceDependencies(node, record ksid.ID, kind watchedResourceKind, members []ksid.ID) []gomode.ResourceDependency {
	if kind == watchNode {
		return []gomode.ResourceDependency{{URIs: []string{m.nodeURI(node)}, Aspects: []string{"body", "metadata", "schema", "summary", "hierarchy", "removed"}}}
	}
	if kind == watchChildren {
		out := []gomode.ResourceDependency{{URIs: []string{m.nodeURI(node) + "/children"}, Aspects: []string{"hierarchy", "removed"}}}
		if len(members) > 0 {
			uris := make([]string, 0, len(members))
			for _, id := range members {
				uris = append(uris, m.nodeURI(id))
			}
			out = append(out, gomode.ResourceDependency{URIs: uris, Aspects: []string{"summary", "child-presence"}})
		}
		return out
	}
	collection := m.nodeURI(node) + "/records"
	if record != 0 {
		return []gomode.ResourceDependency{{URIs: []string{collection + "/" + record.String()}, Aspects: []string{"records"}}, {URIs: []string{collection}, Aspects: []string{"removed"}}}
	}
	return []gomode.ResourceDependency{{URIs: []string{collection}, Aspects: []string{"records", "schema", "removed"}}}
}

func (m *workspaceRegistry) readRecord(ctx context.Context, req *mcp.ToolRequest[mcpschema.ReadRecordInput]) mcp.ToolResult[mcpschema.RecordsSnapshot] {
	id, err := ksid.Parse(req.Arguments.NodeID)
	if err != nil || id == 0 {
		return mcp.ToolError[mcpschema.RecordsSnapshot]("invalid nodeId")
	}
	record, err := ksid.Parse(req.Arguments.RecordID)
	if err != nil || record == 0 {
		return mcp.ToolError[mcpschema.RecordsSnapshot]("invalid recordId")
	}
	out, err := m.recordSnapshot(ctx, &recordResource{workspace: m.wsID, node: id, record: record})
	if err != nil {
		return mcp.ToolError[mcpschema.RecordsSnapshot](err.Error())
	}
	return mcp.TypedToolResult(out)
}

func (m *workspaceRegistry) queryRecords(ctx context.Context, req *mcp.ToolRequest[mcpschema.QueryRecordsInput]) mcp.ToolResult[mcpschema.RecordsSnapshot] {
	in := req.Arguments
	id, err := ksid.Parse(in.NodeID)
	if err != nil || id == 0 || in.Offset < 0 || in.Offset > math.MaxInt-maxMCPRecords-1 || in.Limit < 0 || in.Limit > maxMCPRecords {
		return mcp.ToolError[mcpschema.RecordsSnapshot]("invalid nodeId, offset or limit (maximum 50)")
	}
	view := ksid.ID(0)
	if in.ViewID != "" {
		view, err = ksid.Parse(in.ViewID)
		if err != nil || view == 0 {
			return mcp.ToolError[mcpschema.RecordsSnapshot]("invalid viewId")
		}
	}
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	out, err := m.recordSnapshot(ctx, &recordResource{workspace: m.wsID, node: id, view: view, offset: in.Offset, limit: limit})
	if err != nil {
		return mcp.ToolError[mcpschema.RecordsSnapshot](err.Error())
	}
	return mcp.TypedToolResult(out)
}

func (m *workspaceRegistry) recordSnapshot(ctx context.Context, resource *recordResource) (mcpschema.RecordsSnapshot, error) {
	uri := resource.uri()
	out := mcpschema.RecordsSnapshot{URI: uri, Records: []dto.DataRecordResponse{}}
	read, err := m.beginResourceRead(ctx, uri, resource.node, resource.record, watchRecords)
	if err != nil {
		return out, err
	}
	if resource.record != 0 {
		record, err := m.nodeH.GetRecord(ctx, m.wsID, m.user, &dto.GetRecordRequest{ID: resource.node, RID: resource.record})
		if err != nil {
			if api, ok := errors.AsType[*dto.APIError](err); ok && api.Code() == dto.ErrorCodeNotFound {
				m.finishResourceRead(ctx, read, "", true)
			}
			return out, err
		}
		out.Records = append(out.Records, dto.DataRecordResponse{ID: record.ID, Data: record.Data, Created: record.Created, Modified: record.Modified})
	} else {
		// The host owns saved-view filtering/sorting. Fetch one look-ahead row to
		// certify continuation without interpreting a bounded page as deletion.
		page, err := m.nodeH.ListRecords(ctx, m.wsID, m.user, &dto.ListRecordsRequest{ID: resource.node, ViewID: resource.view, Offset: resource.offset, Limit: resource.limit + 1})
		if err != nil {
			return out, err
		}
		out.Records = page.Records
		if len(out.Records) > resource.limit {
			out.Records = out.Records[:resource.limit]
			next := *resource
			next.offset += resource.limit
			out.NextURI = next.uri()
		}
	}
	encoded, err := json.Marshal(&out)
	if err != nil {
		return out, err
	}
	if len(encoded) > maxMCPRecordBytes {
		return out, errors.New("record data exceeds the 64 KiB MCP projection limit; query fewer rows or use the workspace HTTP record API")
	}
	digest := sha256.Sum256(encoded)
	out.Revision = hex.EncodeToString(digest[:])
	m.finishResourceRead(ctx, read, out.Revision, false)
	return out, nil
}

func (m *workspaceRegistry) readRecordResource(ctx context.Context, uri string) (mcp.ResourcesReadResult, error) {
	resource, err := parseRecordResource(uri)
	if err != nil || (m.wsID != 0 && m.wsID != resource.workspace) {
		return mcp.ResourcesReadResult{}, mcp.ErrInvalidParams("invalid or out-of-scope record resource")
	}
	bound := *m
	bound.wsID = resource.workspace
	registry, err := bound.bindWorkspace(ctx)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	snapshot, err := registry.recordSnapshot(ctx, &resource)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	result, err := mcp.ResourceJSON(uri, snapshot)
	if projection, contextErr := registry.conversationContext(ctx); contextErr == nil && conversationSession(ctx) != "" {
		result.Meta = mcp.MetaObject{gomode.ConversationContextMetaKey: projection}
	}
	return result, err
}

// ResourceTemplates advertises data resources without enumerating table rows.
func (m *workspaceRegistry) ResourceTemplates(context.Context) ([]mcp.ResourceTemplateDescriptor, error) {
	return []mcp.ResourceTemplateDescriptor{
		{Name: "children", URITemplate: "mddb://workspaces/{workspaceId}/nodes/{parentId}/children{?cursor}", MimeType: "application/json"},
		{Name: "record", URITemplate: "mddb://workspaces/{workspaceId}/nodes/{nodeId}/records/{recordId}", MimeType: "application/json"},
		{Name: "record_query", URITemplate: "mddb://workspaces/{workspaceId}/nodes/{nodeId}/records{?viewId,offset,limit}", MimeType: "application/json"},
	}, nil
}

// childrenSnapshot uses the indexed discovery projection, without reading any
// page bodies. Its cursor and returned HasChildren summaries own one dependency.
func (m *workspaceRegistry) childrenSnapshot(ctx context.Context, query *childrenResource) (out mcpschema.ListNodesOutput, resultErr error) {
	c, err := m.decodeCursor(query.cursor, "children", query.parent, 0, mcpNodePageSize)
	if err != nil {
		return out, err
	}
	canonical := *query
	if canonical.cursor != "" {
		canonical.cursor = encodeMCPCursor(&c)
	}
	out.URI = canonical.uri()
	read, err := m.beginResourceRead(ctx, out.URI, query.parent, 0, watchChildren)
	if err != nil {
		return out, err
	}
	var members []ksid.ID
	defer func() {
		if err := m.completeChildrenRead(ctx, read, members); err != nil {
			// The authorized data read is independent of its optional watch.
			// A retired owner cannot be marked fresh, but does not fail the data.
			if ctx.Err() != nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
				resultErr = errors.Join(resultErr, err)
			}
			return
		}
		if members != nil {
			m.finishResourceRead(ctx, read, out.Revision, false)
		} else if errors.Is(resultErr, os.ErrNotExist) {
			m.finishResourceRead(ctx, read, "", true)
		}
	}()
	store, err := m.svc.FileStore.GetWorkspaceStore(ctx, m.wsID)
	if err != nil {
		return out, err
	}
	if query.parent != 0 {
		if _, err := store.ReadNodeSummary(ctx, query.parent); err != nil {
			return out, err
		}
	}
	page, err := store.ListNodeSummaries(ctx, &query.parent, c.Last, mcpNodePageSize)
	if err != nil {
		return out, err
	}
	out.Nodes = make([]mcpschema.NodeSummary, 0, len(page.Nodes))
	ids := make([]ksid.ID, 0, len(page.Nodes))
	for _, node := range page.Nodes {
		ids = append(ids, node.ID)
		out.Nodes = append(out.Nodes, mcpschema.NodeSummary{ID: node.ID.String(), Title: node.Title, Type: string(node.Type), HasChildren: node.HasChildren})
	}
	if page.Next != 0 {
		c.Last = page.Next
		out.NextCursor = encodeMCPCursor(&c)
		next := canonical
		next.cursor = out.NextCursor
		out.NextURI = next.uri()
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	out.Revision = fmt.Sprintf("%x", sha256.Sum256(encoded))
	members = ids
	return out, nil
}

// completeChildrenRead atomically replaces only bounded represented-summary
// dependencies. Nil members retain the previous window after a failed read.
// Queued bootstrap effects are then filtered against the actual window; none
// require content I/O under the context/broker locks.
func (m *workspaceRegistry) completeChildrenRead(ctx context.Context, read resourceRead, members []ksid.ID) error {
	if read.context == nil {
		return nil
	}
	if _, err := read.context.feed.Position(); err != nil {
		return err
	}

	key := voiceFocusKey{user: m.user.ID, login: reqctx.SessionID(ctx), session: conversationSession(ctx)}
	f := m.focus
	f.mu.Lock()
	defer f.mu.Unlock()
	el := f.entries[key]
	if el == nil || el.Value.(voiceFocus).context != read.context || read.watch.readVersion != read.version || !slices.Contains(read.context.resources, read.watch) || !read.watch.readingChildren {
		return nil
	}
	w := read.watch
	if members != nil {
		w.members = slices.Clone(members)
		w.resource.Dependencies = m.resourceDependencies(w.node, 0, watchChildren, w.members)
	}
	w.readingChildren = false
	if _, err := read.context.updateInterestLocked(); err != nil {
		return err
	}
	f.drainContextLocked(key, read.context)
	changed := false
	for _, event := range w.queuedChildren {
		changed = applyWatchedResourceChange(w, event) || changed
	}
	w.queuedChildren = nil
	if w.childrenGap != "" {
		w.resource.State = mcpschema.ResourceContextChanged
		w.resource.ChangeID = w.childrenGap
		w.resource.Aspects = nil
		changed = true
	}
	if changed {
		f.notifyLocked(key)
	}
	return nil
}

func (m *workspaceRegistry) readChildrenResource(ctx context.Context, uri string) (mcp.ResourcesReadResult, error) {
	query, err := parseChildrenResource(uri)
	if err != nil {
		return mcp.ResourcesReadResult{}, mcp.ErrInvalidParams("invalid child-list resource")
	}
	registry, err := m.bindWorkspace(ctx)
	if err != nil || registry.wsID != query.workspace {
		return mcp.ResourcesReadResult{}, mcp.ErrInvalidParams("resource is outside active workspace")
	}
	out, err := registry.childrenSnapshot(ctx, &query)
	if err != nil {
		return mcp.ResourcesReadResult{}, err
	}
	result, err := mcp.ResourceJSON(uri, out)
	if projection, e := registry.conversationContext(ctx); e == nil && conversationSession(ctx) != "" {
		result.Meta = mcp.MetaObject{gomode.ConversationContextMetaKey: projection}
	}
	return result, err
}

// conversationContext projects the same authorized state for resource reads and
// tool reconciliation. mddb owns selection/freshness meaning; clients route only
// private dependencies and never interpret the public content shape.
func (m *workspaceRegistry) conversationContext(ctx context.Context) (gomode.ConversationContext, error) {
	state, err := m.conversationState(ctx)
	if err != nil {
		return gomode.ConversationContext{}, err
	}
	return projectConversationState(state)
}

func projectConversationState(state mcpschema.ConversationContext) (gomode.ConversationContext, error) {
	out := gomode.ConversationContext{URI: voiceContextURI, Epoch: state.Epoch, Version: state.Version, Dependencies: []gomode.ResourceDependency{}}
	model := mcpschema.ModelContext{Focus: state.Focus, Resources: []mcpschema.ModelResource{}, OmittedCount: state.OmittedCount + len(state.Resources)}
	for _, resource := range state.Resources {
		out.Dependencies = append(out.Dependencies, resource.Dependencies...)
	}
	for _, resource := range state.Resources {
		model.Resources = append(model.Resources, mcpschema.ModelResource{URI: resource.URI, Revision: resource.Revision, Aspects: resource.Aspects, State: resource.State})
		model.OmittedCount--
		encoded, marshalErr := json.MarshalIndent(model, "", "  ")
		if marshalErr != nil {
			return out, marshalErr
		}
		if len(encoded) > 5500 {
			model.Resources = model.Resources[:len(model.Resources)-1]
			model.OmittedCount++
			break
		}
	}
	encoded, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		return out, err
	}
	text := string(encoded)
	out.Content = &text
	return out, nil
}

func contextResource(uri string, projection gomode.ConversationContext) (mcp.ResourcesReadResult, error) {
	result, err := mcp.ResourceJSON(uri, json.RawMessage(*projection.Content))
	projection.Content = nil
	result.Meta = mcp.MetaObject{gomode.ConversationContextMetaKey: projection}
	return result, err
}
