// Handles user authentication, registration, and session management.

package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
)

const tokenExpiration = 24 * time.Hour

// AuthHandler handles authentication requests.
type AuthHandler struct {
	userService    *identity.UserService
	orgMemService  *identity.OrganizationMembershipService
	wsMemService   *identity.WorkspaceMembershipService
	orgService     *identity.OrganizationService
	wsService      *identity.WorkspaceService
	sessionService *identity.SessionService
	fs             *content.FileStoreService
	jwtSecret      []byte
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(
	userService *identity.UserService,
	orgMemService *identity.OrganizationMembershipService,
	wsMemService *identity.WorkspaceMembershipService,
	orgService *identity.OrganizationService,
	wsService *identity.WorkspaceService,
	sessionService *identity.SessionService,
	fs *content.FileStoreService,
	jwtSecret string,
) *AuthHandler {
	return &AuthHandler{
		userService:    userService,
		orgMemService:  orgMemService,
		wsMemService:   wsMemService,
		orgService:     orgService,
		wsService:      wsService,
		sessionService: sessionService,
		fs:             fs,
		jwtSecret:      []byte(jwtSecret),
	}
}

// Login handles user login and returns a JWT token.
func (h *AuthHandler) Login(ctx context.Context, req *dto.LoginRequest) (*dto.AuthResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, dto.MissingField("email or password")
	}

	user, err := h.userService.Authenticate(req.Email, req.Password)
	if err != nil {
		return nil, dto.NewAPIError(401, dto.ErrorCodeUnauthorized, "Invalid credentials")
	}

	// Get request metadata from context
	clientIP := reqctx.ClientIP(ctx)
	userAgent := reqctx.UserAgent(ctx)

	token, err := h.GenerateTokenWithSession(user, clientIP, userAgent)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response
	uwm, err := getUserWithMemberships(h.userService, h.orgMemService, h.wsMemService, h.orgService, h.wsService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set active context to first org/workspace
	h.PopulateActiveContext(userResp, uwm)

	return &dto.AuthResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// Register handles user registration.
func (h *AuthHandler) Register(ctx context.Context, req *dto.RegisterRequest) (*dto.AuthResponse, error) {
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return nil, dto.MissingField("email, password, or name")
	}

	// Check if user already exists
	_, err := h.userService.GetByEmail(req.Email)
	if err == nil {
		return nil, dto.NewAPIError(409, dto.ErrorCodeConflict, "User already exists")
	}

	user, err := h.userService.Create(req.Email, req.Password, req.Name)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create user", err)
	}

	// Get request metadata from context
	clientIP := reqctx.ClientIP(ctx)
	userAgent := reqctx.UserAgent(ctx)

	token, err := h.GenerateTokenWithSession(user, clientIP, userAgent)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response (will have empty memberships for new users)
	uwm, err := getUserWithMemberships(h.userService, h.orgMemService, h.wsMemService, h.orgService, h.wsService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	return &dto.AuthResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// GenerateToken generates a JWT token for the given user (without session tracking).
func (h *AuthHandler) GenerateToken(user *identity.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(tokenExpiration).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

// GenerateTokenWithSession creates a session and generates a JWT token with session ID.
func (h *AuthHandler) GenerateTokenWithSession(user *identity.User, clientIP, userAgent string) (string, error) {
	expiresAt := time.Now().Add(tokenExpiration)

	// Pre-generate session ID so we can include it in the JWT
	sessionID := jsonldb.NewID()

	// Build claims with session ID
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"sid":   sessionID.String(),
		"exp":   expiresAt.Unix(),
		"iat":   time.Now().Unix(),
	}

	// Generate the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		return "", err
	}

	// Create session with the pre-generated ID and token hash
	deviceInfo := userAgent
	if len(deviceInfo) > 200 {
		deviceInfo = deviceInfo[:200]
	}
	if _, err := h.sessionService.CreateWithID(sessionID, user.ID, utils.HashToken(tokenString), deviceInfo, clientIP, storage.ToTime(expiresAt)); err != nil {
		return "", err
	}

	return tokenString, nil
}

// GetMe returns the current user info.
func (h *AuthHandler) GetMe(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.GetMeRequest) (*dto.UserResponse, error) {
	// Build user response with memberships
	uwm, err := getUserWithMemberships(h.userService, h.orgMemService, h.wsMemService, h.orgService, h.wsService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Populate active context
	h.PopulateActiveContext(userResp, uwm)

	return userResp, nil
}

// CreateOrganization creates a new organization for the authenticated user.
func (h *AuthHandler) CreateOrganization(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.CreateOrganizationRequest) (*dto.OrganizationResponse, error) {
	if req.Name == "" {
		return nil, dto.MissingField("name")
	}

	// Create the organization
	org, err := h.orgService.Create(ctx, req.Name, user.Email)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create organization", err)
	}

	// Create org membership (user becomes owner of new org)
	if _, err := h.orgMemService.Create(user.ID, org.ID, identity.OrgRoleOwner); err != nil {
		return nil, dto.InternalWithError("Failed to create membership", err)
	}

	memberCount := h.orgMemService.CountOrgMemberships(org.ID)
	workspaceCount := h.wsService.CountByOrg(org.ID)
	return organizationToResponse(org, memberCount, workspaceCount), nil
}

// PopulateActiveContext populates organization/workspace context in the UserResponse.
func (h *AuthHandler) PopulateActiveContext(userResp *dto.UserResponse, uwm *userWithMemberships) {
	// Set first org as active
	if len(uwm.OrgMemberships) > 0 {
		userResp.OrganizationID = uwm.OrgMemberships[0].OrganizationID
		userResp.OrgRole = dto.OrganizationRole(uwm.OrgMemberships[0].Role)
		uwm.CurrentOrgID = uwm.OrgMemberships[0].OrganizationID
		uwm.CurrentOrgRole = uwm.OrgMemberships[0].Role
	}

	// Set first workspace in that org as active
	for _, ws := range uwm.WSMemberships {
		if ws.OrganizationID != uwm.CurrentOrgID {
			continue
		}
		userResp.WorkspaceID = ws.WorkspaceID
		userResp.WorkspaceRole = dto.WorkspaceRole(ws.Role)
		uwm.CurrentWSID = ws.WorkspaceID
		uwm.CurrentWSRole = ws.Role
		break
	}
}

// Logout revokes the current session.
func (h *AuthHandler) Logout(ctx context.Context, _ jsonldb.ID, _ *identity.User, _ *dto.LogoutRequest) (*dto.LogoutResponse, error) {
	sessionID := reqctx.SessionID(ctx)
	if sessionID.IsZero() {
		return &dto.LogoutResponse{Ok: true}, nil
	}

	if err := h.sessionService.Revoke(sessionID); err != nil {
		slog.ErrorContext(ctx, "Failed to revoke session", "error", err, "session_id", sessionID)
		return nil, dto.InternalWithError("Failed to logout", err)
	}

	return &dto.LogoutResponse{Ok: true}, nil
}

// ListSessions returns all active sessions for the current user.
func (h *AuthHandler) ListSessions(ctx context.Context, _ jsonldb.ID, user *identity.User, _ *dto.ListSessionsRequest) (*dto.ListSessionsResponse, error) {
	currentSessionID := reqctx.SessionID(ctx)

	sessions := make([]dto.SessionResponse, 0, 8)
	for session := range h.sessionService.GetActiveByUserID(user.ID) {
		sessions = append(sessions, dto.SessionResponse{
			ID:         session.ID,
			DeviceInfo: session.DeviceInfo,
			IPAddress:  session.IPAddress,
			Created:    session.Created,
			LastUsed:   session.LastUsed,
			IsCurrent:  session.ID == currentSessionID,
		})
	}

	return &dto.ListSessionsResponse{Sessions: sessions}, nil
}

// RevokeSession revokes a specific session.
func (h *AuthHandler) RevokeSession(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.RevokeSessionRequest) (*dto.RevokeSessionResponse, error) {
	// Verify the session belongs to the user
	session, err := h.sessionService.Get(req.SessionID)
	if err != nil {
		return nil, dto.NewAPIError(404, dto.ErrorCodeNotFound, "Session not found")
	}
	if session.UserID != user.ID {
		return nil, dto.NewAPIError(403, dto.ErrorCodeForbidden, "Cannot revoke another user's session")
	}

	if err := h.sessionService.Revoke(req.SessionID); err != nil {
		return nil, dto.InternalWithError("Failed to revoke session", err)
	}

	return &dto.RevokeSessionResponse{Ok: true}, nil
}

// RevokeAllSessions revokes all sessions for the current user except the current one.
func (h *AuthHandler) RevokeAllSessions(ctx context.Context, _ jsonldb.ID, user *identity.User, _ *dto.RevokeAllSessionsRequest) (*dto.RevokeAllSessionsResponse, error) {
	currentSessionID := reqctx.SessionID(ctx)

	var toRevoke []jsonldb.ID
	for session := range h.sessionService.GetActiveByUserID(user.ID) {
		if session.ID != currentSessionID {
			toRevoke = append(toRevoke, session.ID)
		}
	}

	revokedCount := 0
	for _, id := range toRevoke {
		if err := h.sessionService.Revoke(id); err != nil {
			slog.ErrorContext(ctx, "Failed to revoke session", "error", err, "session_id", id)
			continue
		}
		revokedCount++
	}

	return &dto.RevokeAllSessionsResponse{RevokedCount: revokedCount}, nil
}
