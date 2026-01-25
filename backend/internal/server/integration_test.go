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
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

const testJWTSecret = "test-secret-key-for-integration-tests"

type testEnv struct {
	server        *httptest.Server
	userService   *identity.UserService
	orgService    *identity.OrganizationService
	wsService     *identity.WorkspaceService
	orgMemService *identity.OrganizationMembershipService
	wsMemService  *identity.WorkspaceMembershipService
	orgInvService *identity.OrganizationInvitationService
	wsInvService  *identity.WorkspaceInvitationService
	fileStore     *content.FileStore
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

	fileStore, err := content.NewFileStore(tempDir, gitMgr, wsService, orgService)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	sessionService, err := identity.NewSessionService(filepath.Join(tempDir, "sessions.jsonl"))
	if err != nil {
		t.Fatalf("NewSessionService: %v", err)
	}

	router := NewRouter(
		fileStore, userService, orgService, wsService,
		orgInvService, wsInvService, orgMemService, wsMemService, sessionService,
		testJWTSecret,
		"http://localhost:8080",
		"", "", // google OAuth (disabled)
		"", "", // microsoft OAuth (disabled)
	)

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
		if health.Version != "1.0.0" {
			t.Errorf("Health version: got %q, want %q", health.Version, "1.0.0")
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

		// Get the default workspace ID from user response
		var meResp dto.UserResponse
		env.doJSON(t, http.MethodGet, "/api/auth/me", nil, &meResp, token)
		wsID := meResp.WorkspaceID

		if wsID.IsZero() {
			t.Fatal("User should have a default workspace ID")
		}

		// Create a page
		createPageReq := dto.CreatePageRequest{
			Title:   "My First Page",
			Content: "# Hello World\n\nThis is my first page.",
		}
		var createPageResp dto.CreatePageResponse
		status := env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/pages", createPageReq, &createPageResp, token)
		if status != http.StatusOK {
			t.Fatalf("POST /api/workspaces/%s/pages: got status %d", wsID, status)
		}

		if createPageResp.ID.IsZero() {
			t.Fatal("CreatePage should return an ID")
		}
		pageID := createPageResp.ID

		// List pages
		var listResp dto.ListPagesResponse
		status = env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/pages", nil, &listResp, token)
		if status != http.StatusOK {
			t.Fatalf("GET /api/workspaces/%s/pages: got status %d", wsID, status)
		}

		// There should be 2 pages: the welcome page and our new page
		if len(listResp.Pages) < 1 {
			t.Errorf("List pages: got %d pages, want at least 1", len(listResp.Pages))
		}

		// Get single page
		var getPageResp dto.GetPageResponse
		status = env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/pages/"+pageID.String(), nil, &getPageResp, token)
		if status != http.StatusOK {
			t.Fatalf("GET /api/workspaces/%s/pages/%s: got status %d", wsID, pageID, status)
		}

		if getPageResp.Title != "My First Page" {
			t.Errorf("Get page title: got %q, want %q", getPageResp.Title, "My First Page")
		}

		// Update page
		updatePageReq := dto.UpdatePageRequest{
			Title:   "Updated Title",
			Content: "# Updated Content\n\nThis page has been updated.",
		}
		var updateResp dto.UpdatePageResponse
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/pages/"+pageID.String(), updatePageReq, &updateResp, token)
		if status != http.StatusOK {
			t.Fatalf("POST /api/workspaces/%s/pages/%s: got status %d", wsID, pageID, status)
		}

		if updateResp.ID != pageID {
			t.Errorf("Updated page ID: got %v, want %v", updateResp.ID, pageID)
		}

		// Verify update by getting the page again
		var getPageResp2 dto.GetPageResponse
		env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/pages/"+pageID.String(), nil, &getPageResp2, token)

		if getPageResp2.Title != "Updated Title" {
			t.Errorf("Updated page title: got %q, want %q", getPageResp2.Title, "Updated Title")
		}

		// Delete page
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+wsID.String()+"/pages/"+pageID.String()+"/delete", nil, nil, token)
		if status != http.StatusOK {
			t.Fatalf("POST /api/workspaces/%s/pages/%s/delete: got status %d", wsID, pageID, status)
		}

		// Verify page is deleted
		status = env.doJSON(t, http.MethodGet, "/api/workspaces/"+wsID.String()+"/pages/"+pageID.String(), nil, nil, token)
		if status != http.StatusNotFound {
			t.Errorf("Get deleted page: got status %d, want %d", status, http.StatusNotFound)
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

		// Get Dave's workspace ID
		var daveMe dto.UserResponse
		env.doJSON(t, http.MethodGet, "/api/auth/me", nil, &daveMe, daveToken)
		daveWSID := daveMe.WorkspaceID

		// Eve tries to access Dave's workspace - should be forbidden
		status := env.doJSON(t, http.MethodGet, "/api/workspaces/"+daveWSID.String()+"/pages", nil, nil, eveToken)
		if status != http.StatusForbidden {
			t.Errorf("Eve accessing Dave's workspace: got status %d, want %d", status, http.StatusForbidden)
		}

		// Eve tries to create a page in Dave's workspace - should be forbidden
		status = env.doJSON(t, http.MethodPost, "/api/workspaces/"+daveWSID.String()+"/pages", dto.CreatePageRequest{
			Title: "Sneaky Page", Content: "Should not work",
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
}
