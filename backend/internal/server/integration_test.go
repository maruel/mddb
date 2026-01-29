package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

var testJWTSecret = []byte("test-secret-key-32-bytes-long!!!")

type testEnv struct {
	server        *httptest.Server
	userService   *identity.UserService
	orgService    *identity.OrganizationService
	wsService     *identity.WorkspaceService
	orgMemService *identity.OrganizationMembershipService
	wsMemService  *identity.WorkspaceMembershipService
	orgInvService *identity.OrganizationInvitationService
	wsInvService  *identity.WorkspaceInvitationService
	fileStore     *content.FileStoreService
}

func setupTestEnv(t *testing.T) *testEnv {
	tempDir := t.TempDir()

	userService, err := identity.NewUserService(filepath.Join(tempDir, "users.jsonl"))
	if err != nil {
		t.Fatalf("NewUserService: %v", err)
	}

	orgService, err := identity.NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("NewOrganizationService: %v", err)
	}

	wsService, err := identity.NewWorkspaceService(filepath.Join(tempDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatalf("NewWorkspaceService: %v", err)
	}

	orgMemService, err := identity.NewOrganizationMembershipService(filepath.Join(tempDir, "org_memberships.jsonl"), userService, orgService)
	if err != nil {
		t.Fatalf("NewOrganizationMembershipService: %v", err)
	}

	wsMemService, err := identity.NewWorkspaceMembershipService(filepath.Join(tempDir, "ws_memberships.jsonl"), wsService, orgService)
	if err != nil {
		t.Fatalf("NewWorkspaceMembershipService: %v", err)
	}

	orgInvService, err := identity.NewOrganizationInvitationService(filepath.Join(tempDir, "org_invitations.jsonl"))
	if err != nil {
		t.Fatalf("NewOrganizationInvitationService: %v", err)
	}

	wsInvService, err := identity.NewWorkspaceInvitationService(filepath.Join(tempDir, "ws_invitations.jsonl"))
	if err != nil {
		t.Fatalf("NewWorkspaceInvitationService: %v", err)
	}

	gitMgr := git.NewManager(tempDir, "test", "test@example.com")

	fileStore, err := content.NewFileStoreService(tempDir, gitMgr, wsService, orgService)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	sessionService, err := identity.NewSessionService(filepath.Join(tempDir, "sessions.jsonl"))
	if err != nil {
		t.Fatalf("NewSessionService: %v", err)
	}

	svc := &handlers.Services{
		FileStore:     fileStore,
		Search:        content.NewSearchService(fileStore),
		User:          userService,
		Organization:  orgService,
		Workspace:     wsService,
		OrgInvitation: orgInvService,
		WSInvitation:  wsInvService,
		OrgMembership: orgMemService,
		WSMembership:  wsMemService,
		Session:       sessionService,
		EmailVerif:    nil, // disabled
		Email:         nil, // disabled
	}
	serverCfg := &storage.ServerConfig{
		JWTSecret:  testJWTSecret,
		Quotas:     storage.DefaultServerQuotas(),
		RateLimits: storage.DefaultRateLimits(),
	}
	cfg := &Config{
		ServerConfig: serverCfg,
		DataDir:      t.TempDir(),
		BaseURL:      "http://localhost:8080",
		Version:      "test",
		GoVersion:    "go1.24.0",
		Revision:     "abc1234",
		Dirty:        false,
		OAuth:        OAuthConfig{}, // all disabled
	}
	router := NewRouter(svc, cfg)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return &testEnv{
		server:        server,
		userService:   userService,
		orgService:    orgService,
		wsService:     wsService,
		orgMemService: orgMemService,
		wsMemService:  wsMemService,
		orgInvService: orgInvService,
		wsInvService:  wsInvService,
		fileStore:     fileStore,
	}
}

// doJSON performs an HTTP request, decodes the JSON response, and returns the status code.
// Body is always read and closed before returning.
func (e *testEnv) doJSON(t *testing.T, method, path string, body, response any, token string) int {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, e.server.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do request: %v", err)
	}

	data, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("ReadAll/Close: %v", err)
	}

	if response != nil && len(data) > 0 {
		if err := json.Unmarshal(data, response); err != nil {
			t.Fatalf("Unmarshal response: %v\nBody: %s", err, string(data))
		}
	}

	return resp.StatusCode
}

func TestIntegration(t *testing.T) {
	t.Parallel()
	t.Run("Health", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		var health dto.HealthResponse
		status := env.doJSON(t, http.MethodGet, "/api/health", nil, &health, "")
		if status != http.StatusOK {
			t.Errorf("GET /api/health: got status %d, want %d", status, http.StatusOK)
		}

		if health.Status != "ok" {
			t.Errorf("Health status: got %q, want %q", health.Status, "ok")
		}
		if health.Version != "test" {
			t.Errorf("Health version: got %q, want %q", health.Version, "test")
		}
		if health.GoVersion != "go1.24.0" {
			t.Errorf("Health go_version: got %q, want %q", health.GoVersion, "go1.24.0")
		}
		if health.Revision != "abc1234" {
			t.Errorf("Health revision: got %q, want %q", health.Revision, "abc1234")
		}
		if health.Dirty {
			t.Error("Health dirty: got true, want false")
		}
	})

	t.Run("UserWorkflow", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		// Register a new user
		registerReq := dto.RegisterRequest{
			Email:    "alice@example.com",
			Password: "securePass1234",
			Name:     "Alice",
		}
		var loginResp dto.AuthResponse
		status := env.doJSON(t, http.MethodPost, "/api/auth/register", registerReq, &loginResp, "")
		if status != http.StatusOK {
			t.Fatalf("POST /api/auth/register: got status %d, want %d", status, http.StatusOK)
		}

		if loginResp.Token == "" {
			t.Fatal("Register should return a token")
		}
		if loginResp.User.Email != "alice@example.com" {
			t.Errorf("User email: got %q, want %q", loginResp.User.Email, "alice@example.com")
		}
		if loginResp.User.Name != "Alice" {
			t.Errorf("User name: got %q, want %q", loginResp.User.Name, "Alice")
		}

		token := loginResp.Token

		// Get current user (authenticated)
		var meResp dto.UserResponse
		status = env.doJSON(t, http.MethodGet, "/api/auth/me", nil, &meResp, token)
		if status != http.StatusOK {
			t.Fatalf("GET /api/auth/me: got status %d, want %d", status, http.StatusOK)
		}

		if meResp.Email != "alice@example.com" {
			t.Errorf("Me email: got %q, want %q", meResp.Email, "alice@example.com")
		}

		// Login with the same credentials
		loginReq := dto.LoginRequest{
			Email:    "alice@example.com",
			Password: "securePass1234",
		}
		var loginResp2 dto.AuthResponse
		status = env.doJSON(t, http.MethodPost, "/api/auth/login", loginReq, &loginResp2, "")
		if status != http.StatusOK {
			t.Fatalf("POST /api/auth/login: got status %d, want %d", status, http.StatusOK)
		}

		if loginResp2.Token == "" {
			t.Fatal("Login should return a token")
		}

		// Login with wrong password should fail
		loginReq.Password = "wrongpassword"
		status = env.doJSON(t, http.MethodPost, "/api/auth/login", loginReq, nil, "")
		if status != http.StatusUnauthorized {
			t.Errorf("Login with wrong password: got status %d, want %d", status, http.StatusUnauthorized)
		}
	})

	t.Run("AuthMiddleware", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		// Request without token should be unauthorized
		status := env.doJSON(t, http.MethodGet, "/api/auth/me", nil, nil, "")
		if status != http.StatusUnauthorized {
			t.Errorf("GET /api/auth/me without token: got status %d, want %d", status, http.StatusUnauthorized)
		}

		// Request with invalid token should be unauthorized
		status = env.doJSON(t, http.MethodGet, "/api/auth/me", nil, nil, "invalid-token")
		if status != http.StatusUnauthorized {
			t.Errorf("GET /api/auth/me with invalid token: got status %d, want %d", status, http.StatusUnauthorized)
		}
	})

	t.Run("OrganizationWorkflow", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		// Register user first
		registerReq := dto.RegisterRequest{
			Email:    "bob@example.com",
			Password: "Pass1234",
			Name:     "Bob",
		}
		var loginResp dto.AuthResponse
		status := env.doJSON(t, http.MethodPost, "/api/auth/register", registerReq, &loginResp, "")
		if status != http.StatusOK {
			t.Fatalf("Register: got status %d", status)
		}
		token := loginResp.Token

		// Create organization
		createOrgReq := dto.CreateOrganizationRequest{
			Name: "Bob's Organization",
		}
		var orgResp dto.OrganizationResponse
		status = env.doJSON(t, http.MethodPost, "/api/organizations", createOrgReq, &orgResp, token)
		if status != http.StatusOK {
			t.Fatalf("POST /api/organizations: got status %d, want %d", status, http.StatusOK)
		}

		if orgResp.Name != "Bob's Organization" {
			t.Errorf("Org name: got %q, want %q", orgResp.Name, "Bob's Organization")
		}
		if orgResp.ID.IsZero() {
			t.Fatal("Org should have an ID")
		}

		orgID := orgResp.ID

		// Get organization settings
		var getOrgResp dto.OrganizationResponse
		status = env.doJSON(t, http.MethodGet, "/api/organizations/"+orgID.String(), nil, &getOrgResp, token)
		if status != http.StatusOK {
			t.Fatalf("GET /api/organizations/%s: got status %d", orgID, status)
		}

		if getOrgResp.Name != "Bob's Organization" {
			t.Errorf("Get org name: got %q, want %q", getOrgResp.Name, "Bob's Organization")
		}
	})

	t.Run("PageWorkflow", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		// Setup: register user and create org
		registerReq := dto.RegisterRequest{
			Email:    "charlie@example.com",
			Password: "Pass1234",
			Name:     "Charlie",
		}
		var loginResp dto.AuthResponse
		env.doJSON(t, http.MethodPost, "/api/auth/register", registerReq, &loginResp, "")
		token := loginResp.Token

		var orgResp dto.OrganizationResponse
		env.doJSON(t, http.MethodPost, "/api/organizations", dto.CreateOrganizationRequest{Name: "Charlie's Organization"}, &orgResp, token)
		orgID := orgResp.ID

		// Create a workspace
		var wsResp dto.WorkspaceResponse
		env.doJSON(t, http.MethodPost, "/api/organizations/"+orgID.String()+"/workspaces", dto.CreateWorkspaceRequest{Name: "My Workspace"}, &wsResp, token)
		wsID := wsResp.ID

		if wsID.IsZero() {
			t.Fatal("Workspace creation should return a workspace ID")
		}

		// Create a top-level page (parent_id=0 means top-level, no root node).
		createPageReq := dto.CreatePageRequest{
			Title:   "My First Page",
			Content: "",
		}
		var createPageResp dto.CreatePageResponse
		status := env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/nodes/0/page/create", createPageReq, &createPageResp, token)
		if status != http.StatusOK {
			t.Fatalf("POST /api/workspaces/%s/nodes/0/page/create: got status %d", wsID, status)
		}

		// Top-level page should have a non-zero ID.
		page1ID := createPageResp.ID
		if page1ID.IsZero() {
			t.Fatal("Top-level page should have a non-zero ID")
		}

		// Create another top-level page.
		createPage2Req := dto.CreatePageRequest{
			Title:   "Second Page",
			Content: "Some content",
		}
		var createPage2Resp dto.CreatePageResponse
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/nodes/0/page/create", createPage2Req, &createPage2Resp, token)
		if status != http.StatusOK {
			t.Fatalf("POST /api/workspaces/%s/nodes/0/page/create (page2): got status %d", wsID, status)
		}

		// Second page should also have a non-zero ID.
		nodeID := createPage2Resp.ID
		if nodeID.IsZero() {
			t.Fatal("Second page should have a non-zero ID")
		}

		// Get node 0 should return 404 (no root node exists).
		var notFoundResp dto.NodeResponse
		status = env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/0", nil, &notFoundResp, token)
		if status != http.StatusNotFound {
			t.Fatalf("GET /api/workspaces/%s/nodes/0: got status %d, want 404", wsID, status)
		}

		// Get single node should work.
		var getNodeResp dto.NodeResponse
		status = env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/"+page1ID.String(), nil, &getNodeResp, token)
		if status != http.StatusOK {
			t.Fatalf("GET /api/workspaces/%s/nodes/%s: got status %d", wsID, page1ID, status)
		}

		if getNodeResp.Title != "My First Page" {
			t.Errorf("Get node title: got %q, want %q", getNodeResp.Title, "My First Page")
		}

		// Update page
		updatePageReq := dto.UpdatePageRequest{
			Title:   "Updated Title",
			Content: "# Updated Content\n\nThis page has been updated.",
		}
		var updateResp dto.UpdatePageResponse
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/nodes/"+nodeID.String()+"/page", updatePageReq, &updateResp, token)
		if status != http.StatusOK {
			t.Fatalf("POST /api/workspaces/%s/nodes/%s/page: got status %d", wsID, nodeID, status)
		}

		if updateResp.ID != nodeID {
			t.Errorf("Updated page ID: got %v, want %v", updateResp.ID, nodeID)
		}

		// Verify update by getting the node again
		var getNodeResp2 dto.NodeResponse
		env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/"+nodeID.String(), nil, &getNodeResp2, token)

		if getNodeResp2.Title != "Updated Title" {
			t.Errorf("Updated node title: got %q, want %q", getNodeResp2.Title, "Updated Title")
		}

		// Delete node
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/nodes/"+nodeID.String()+"/delete", nil, nil, token)
		if status != http.StatusOK {
			t.Fatalf("POST /api/workspaces/%s/nodes/%s/delete: got status %d", wsID, nodeID, status)
		}

		// Verify node is deleted
		status = env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/"+nodeID.String(), nil, nil, token)
		if status != http.StatusNotFound {
			t.Errorf("Get deleted node: got status %d, want %d", status, http.StatusNotFound)
		}
	})

	t.Run("PageHierarchy", func(t *testing.T) {
		// Tests navigation and child page creation in the no-root workspace model.
		t.Parallel()
		env := setupTestEnv(t)

		// Setup: register user, create org and workspace
		var loginResp dto.AuthResponse
		env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "hierarchy@example.com", Password: "Pass1234", Name: "Hierarchy Test",
		}, &loginResp, "")
		token := loginResp.Token

		var orgResp dto.OrganizationResponse
		env.doJSON(t, http.MethodPost, "/api/organizations", dto.CreateOrganizationRequest{
			Name: "Hierarchy Org",
		}, &orgResp, token)

		var wsResp dto.WorkspaceResponse
		env.doJSON(t, http.MethodPost, "/api/organizations/"+orgResp.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{
			Name: "Hierarchy Workspace",
		}, &wsResp, token)
		wsID := wsResp.ID

		// 1. List children of 0 (top-level nodes) - should be empty initially
		var emptyChildren dto.ListNodeChildrenResponse
		status := env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/0/children", nil, &emptyChildren, token)
		if status != http.StatusOK {
			t.Fatalf("GET /nodes/0/children: got status %d", status)
		}
		if len(emptyChildren.Nodes) != 0 {
			t.Errorf("expected 0 top-level nodes initially, got %d", len(emptyChildren.Nodes))
		}

		// 2. Create a top-level page (parent=0)
		var topLevelResp dto.CreatePageResponse
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/nodes/0/page/create", dto.CreatePageRequest{
			Title: "Top Level Page",
		}, &topLevelResp, token)
		if status != http.StatusOK {
			t.Fatalf("POST create top-level page: got status %d", status)
		}
		topLevelID := topLevelResp.ID
		if topLevelID.IsZero() {
			t.Fatal("top-level page should have non-zero ID")
		}

		// 3. List children of 0 - should now have 1 top-level node
		var topLevelChildren dto.ListNodeChildrenResponse
		status = env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/0/children", nil, &topLevelChildren, token)
		if status != http.StatusOK {
			t.Fatalf("GET /nodes/0/children: got status %d", status)
		}
		if len(topLevelChildren.Nodes) != 1 {
			t.Fatalf("expected 1 top-level node, got %d", len(topLevelChildren.Nodes))
		}
		if topLevelChildren.Nodes[0].ID != topLevelID {
			t.Errorf("expected top-level node ID %s, got %s", topLevelID, topLevelChildren.Nodes[0].ID)
		}
		if topLevelChildren.Nodes[0].Title != "Top Level Page" {
			t.Errorf("expected title 'Top Level Page', got %q", topLevelChildren.Nodes[0].Title)
		}

		// 4. Create a child page under the top-level page
		var childResp dto.CreatePageResponse
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/nodes/"+topLevelID.String()+"/page/create", dto.CreatePageRequest{
			Title:   "Child Page",
			Content: "This is a child page",
		}, &childResp, token)
		if status != http.StatusOK {
			t.Fatalf("POST create child page: got status %d", status)
		}
		childID := childResp.ID
		if childID.IsZero() {
			t.Fatal("child page should have non-zero ID")
		}

		// 5. List children of top-level page - should have 1 child
		var childChildren dto.ListNodeChildrenResponse
		status = env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/"+topLevelID.String()+"/children", nil, &childChildren, token)
		if status != http.StatusOK {
			t.Fatalf("GET /nodes/%s/children: got status %d", topLevelID, status)
		}
		if len(childChildren.Nodes) != 1 {
			t.Fatalf("expected 1 child node, got %d", len(childChildren.Nodes))
		}
		if childChildren.Nodes[0].ID != childID {
			t.Errorf("expected child node ID %s, got %s", childID, childChildren.Nodes[0].ID)
		}

		// 6. Create a grandchild page under the child
		var grandchildResp dto.CreatePageResponse
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/nodes/"+childID.String()+"/page/create", dto.CreatePageRequest{
			Title:   "Grandchild Page",
			Content: "This is a grandchild page",
		}, &grandchildResp, token)
		if status != http.StatusOK {
			t.Fatalf("POST create grandchild page: got status %d", status)
		}
		grandchildID := grandchildResp.ID

		// 7. Navigate: read each node and verify parent relationships
		var topLevelNode dto.NodeResponse
		env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/"+topLevelID.String(), nil, &topLevelNode, token)
		if !topLevelNode.ParentID.IsZero() {
			t.Errorf("top-level node parent should be 0, got %s", topLevelNode.ParentID)
		}

		var childNode dto.NodeResponse
		env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/"+childID.String(), nil, &childNode, token)
		if childNode.ParentID != topLevelID {
			t.Errorf("child node parent should be %s, got %s", topLevelID, childNode.ParentID)
		}

		var grandchildNode dto.NodeResponse
		env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/"+grandchildID.String(), nil, &grandchildNode, token)
		if grandchildNode.ParentID != childID {
			t.Errorf("grandchild node parent should be %s, got %s", childID, grandchildNode.ParentID)
		}

		// 8. Verify top-level still has only 1 node (child creation doesn't affect top-level list)
		var finalTopLevel dto.ListNodeChildrenResponse
		env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/0/children", nil, &finalTopLevel, token)
		if len(finalTopLevel.Nodes) != 1 {
			t.Errorf("expected 1 top-level node after hierarchy creation, got %d", len(finalTopLevel.Nodes))
		}

		// 9. List children of grandchild - should be empty (leaf node)
		var grandchildChildren dto.ListNodeChildrenResponse
		env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/nodes/"+grandchildID.String()+"/children", nil, &grandchildChildren, token)
		if len(grandchildChildren.Nodes) != 0 {
			t.Errorf("expected grandchild to have 0 children (leaf), got %d", len(grandchildChildren.Nodes))
		}
	})

	t.Run("ForbiddenAccess", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		// Register two users
		var daveLogin dto.AuthResponse
		env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "dave@example.com", Password: "Pass1234", Name: "Dave",
		}, &daveLogin, "")
		daveToken := daveLogin.Token

		var eveLogin dto.AuthResponse
		env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "eve@example.com", Password: "Pass1234", Name: "Eve",
		}, &eveLogin, "")
		eveToken := eveLogin.Token

		// Dave creates an organization
		var orgResp dto.OrganizationResponse
		env.doJSON(t, http.MethodPost, "/api/organizations", dto.CreateOrganizationRequest{
			Name: "Dave's Organization",
		}, &orgResp, daveToken)
		orgID := orgResp.ID

		// Dave creates a workspace
		var wsResp dto.WorkspaceResponse
		env.doJSON(t, http.MethodPost, "/api/organizations/"+orgID.String()+"/workspaces", dto.CreateWorkspaceRequest{
			Name: "Dave's Workspace",
		}, &wsResp, daveToken)
		daveWSID := wsResp.ID

		// Eve tries to access Dave's workspace - should be forbidden
		status := env.doJSON(t, http.MethodGet, "/api/workspaces/"+daveWSID.String()+"/nodes/0", nil, nil, eveToken)
		if status != http.StatusForbidden {
			t.Errorf("Eve accessing Dave's workspace: got status %d, want %d", status, http.StatusForbidden)
		}

		// Eve tries to create a page in Dave's workspace - should be forbidden
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+daveWSID.String()+"/nodes/0/page/create", dto.CreatePageRequest{
			Title: "Sneaky Page",
		}, nil, eveToken)
		if status != http.StatusForbidden {
			t.Errorf("Eve creating page in Dave's workspace: got status %d, want %d", status, http.StatusForbidden)
		}
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		// Register with empty email
		status := env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "", Password: "Pass1234", Name: "Test",
		}, nil, "")
		if status != http.StatusBadRequest {
			t.Errorf("Register with empty email: got status %d, want %d", status, http.StatusBadRequest)
		}

		// Register with empty name
		status = env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "valid@example.com", Password: "Pass1234", Name: "",
		}, nil, "")
		if status != http.StatusBadRequest {
			t.Errorf("Register with empty name: got status %d, want %d", status, http.StatusBadRequest)
		}

		// Register with empty password
		status = env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "valid@example.com", Password: "", Name: "Test",
		}, nil, "")
		if status != http.StatusBadRequest {
			t.Errorf("Register with empty password: got status %d, want %d", status, http.StatusBadRequest)
		}
	})

	t.Run("DuplicateUser", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		// Register user first time
		status := env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "duplicate@example.com", Password: "Pass1234", Name: "First",
		}, nil, "")
		if status != http.StatusOK {
			t.Fatalf("First registration: got status %d", status)
		}

		// Register user second time with same email - should fail
		status = env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "duplicate@example.com", Password: "Pass4567", Name: "Second",
		}, nil, "")
		if status != http.StatusConflict {
			t.Errorf("Duplicate registration: got status %d, want %d", status, http.StatusConflict)
		}
	})

	t.Run("WorkspaceCreationEnforcement", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)

		// 1. Register owner
		var ownerLogin dto.AuthResponse
		env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "owner@example.com", Password: "Pass1234", Name: "Owner",
		}, &ownerLogin, "")
		ownerToken := ownerLogin.Token

		// 2. Create organization
		var orgResp dto.OrganizationResponse
		env.doJSON(t, http.MethodPost, "/api/organizations", dto.CreateOrganizationRequest{
			Name: "Enforced Org",
		}, &orgResp, ownerToken)
		orgID := orgResp.ID

		// 3. Register member
		var memberLogin dto.AuthResponse
		env.doJSON(t, http.MethodPost, "/api/auth/register", dto.RegisterRequest{
			Email: "member@example.com", Password: "Pass1234", Name: "Member",
		}, &memberLogin, "")
		memberToken := memberLogin.Token

		// 4. Invite member to organization with 'member' role
		inviteReq := dto.CreateOrgInvitationRequest{
			Email: "member@example.com",
			Role:  dto.OrgRoleMember,
		}
		status := env.doJSON(t, http.MethodPost, "/api/organizations/"+orgID.String()+"/invitations", inviteReq, nil, ownerToken)
		if status != http.StatusOK {
			t.Fatalf("Owner inviting member: got status %d, want %d", status, http.StatusOK)
		}

		// 5. Member accepts invitation (need to list invitations first to get token)
		// Since we don't have a direct way to get the token without email integration,
		// we'll fetch invitations as the owner to simulate the member finding it?
		// Actually, ListOrgInvitations is for the org.
		var invitesResp dto.ListOrgInvitationsResponse
		env.doJSON(t, http.MethodGet, "/api/organizations/"+orgID.String()+"/invitations", nil, &invitesResp, ownerToken)
		if len(invitesResp.Invitations) == 0 {
			t.Fatal("No invitation found")
		}
		// The API response doesn't include the secret token for security.
		// We have to cheat and look it up in the database or use the direct service if possible.
		// BUT: Since we are in an integration test with `testEnv`, we can use `orgInvService`.
		var inviteToken string
		for inv := range env.orgInvService.IterByOrg(orgID) {
			if inv.Email == "member@example.com" {
				inviteToken = inv.Token
				break
			}
		}
		if inviteToken == "" {
			t.Fatal("Invitation token not found")
		}

		// Let's use the authenticated accept flow if possible?
		// The `AcceptOrgInvitation` handler logic:
		// If Authorization header is present:
		//    Validate user.
		//    Accept invitation for that user.
		// Else:
		//    Create user (if new) or login (if exists + password match)
		//    Then accept.

		// Since member is already registered and logged in (has memberToken),
		// we can just call accept with the token in the body and Auth header.
		acceptBody := struct {
			Token    string `json:"token"`
			Password string `json:"password"`
			Name     string `json:"name"`
		}{
			Token:    inviteToken,
			Password: "Pass1234",
			Name:     "Member",
		}

		status = env.doJSON(t, http.MethodPost, "/api/auth/invitations/org/accept", acceptBody, nil, memberToken)
		if status != http.StatusOK {
			t.Fatalf("Member accepting invitation: got status %d, want %d", status, http.StatusOK)
		}

		// 6. Member attempts to create workspace
		createWSReq := dto.CreateWorkspaceRequest{
			Name: "Unauthorized Workspace",
		}
		status = env.doJSON(t, http.MethodPost, "/api/organizations/"+orgID.String()+"/workspaces", createWSReq, nil, memberToken)

		// 7. Expect 403 Forbidden
		if status != http.StatusForbidden {
			t.Errorf("Member creating workspace: got status %d, want %d", status, http.StatusForbidden)
		}
	})
}
