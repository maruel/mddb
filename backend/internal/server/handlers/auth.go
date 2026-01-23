package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
)

const tokenExpiration = 24 * time.Hour

// AuthHandler handles authentication requests.
type AuthHandler struct {
	userService    *identity.UserService
	memService     *identity.MembershipService
	orgService     *identity.OrganizationService
	sessionService *identity.SessionService
	fs             *content.FileStore
	jwtSecret      []byte
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(userService *identity.UserService, memService *identity.MembershipService, orgService *identity.OrganizationService, sessionService *identity.SessionService, fs *content.FileStore, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		userService:    userService,
		memService:     memService,
		orgService:     orgService,
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
	uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set active context to first membership
	if len(userResp.Memberships) > 0 {
		h.PopulateActiveContext(userResp, userResp.Memberships[0].OrganizationID)
	}

	return &dto.AuthResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// Register handles user registration.
// Note: Organization creation is handled by the frontend after registration.
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
	uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, user.ID)
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
// Prefer GenerateTokenWithSession for proper session management.
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
	// Store user agent directly (truncate if too long)
	deviceInfo := userAgent
	if len(deviceInfo) > 200 {
		deviceInfo = deviceInfo[:200]
	}
	if _, err := h.sessionService.CreateWithID(sessionID, user.ID, utils.HashToken(tokenString), deviceInfo, clientIP, expiresAt); err != nil {
		return "", err
	}

	return tokenString, nil
}

// GetMe returns the current user info.
func (h *AuthHandler) GetMe(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.GetMeRequest) (*dto.UserResponse, error) {
	// Build user response with memberships
	uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// For /api/auth/me, we need to decide which org is "active"
	// For now, use the first membership if not specified
	if len(userResp.Memberships) > 0 {
		h.PopulateActiveContext(userResp, userResp.Memberships[0].OrganizationID)
	}

	return userResp, nil
}

// CreateOrganization creates a new organization for the authenticated user.
func (h *AuthHandler) CreateOrganization(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.CreateOrganizationRequest) (*dto.OrganizationResponse, error) {
	if req.Name == "" {
		return nil, dto.MissingField("name")
	}

	// Create the organization
	org, err := h.orgService.Create(ctx, req.Name)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create organization", err)
	}

	// Initialize organization storage
	if err := h.fs.InitOrg(ctx, org.ID); err != nil {
		return nil, dto.InternalWithError("Failed to initialize organization storage", err)
	}

	// Create welcome page if content provided
	if req.WelcomePageTitle != "" && req.WelcomePageContent != "" {
		pageID := jsonldb.NewID()
		author := git.Author{Name: user.Name, Email: user.Email}
		if _, err := h.fs.WritePage(ctx, org.ID, pageID, req.WelcomePageTitle, req.WelcomePageContent, author); err != nil {
			slog.ErrorContext(ctx, "Failed to create welcome page", "error", err, "org_id", org.ID)
			return nil, dto.InternalWithError("Failed to initialize organization", err)
		}
	}

	// Create membership (user becomes admin of new org)
	if _, err := h.memService.Create(user.ID, org.ID, identity.UserRoleAdmin); err != nil {
		return nil, dto.InternalWithError("Failed to create membership", err)
	}

	return organizationToResponse(org), nil
}

// PopulateActiveContext populates organization-specific fields in the UserResponse.
func (h *AuthHandler) PopulateActiveContext(userResp *dto.UserResponse, orgIDStr string) {
	userResp.OrganizationID = orgIDStr

	for _, m := range userResp.Memberships {
		if m.OrganizationID == orgIDStr {
			userResp.Role = m.Role
			break
		}
	}
}

// Logout revokes the current session.
func (h *AuthHandler) Logout(ctx context.Context, _ jsonldb.ID, _ *identity.User, _ *dto.LogoutRequest) (*dto.LogoutResponse, error) {
	sessionID := reqctx.SessionID(ctx)
	if sessionID.IsZero() {
		// No session ID in token - old token without session tracking
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

	sessions := make([]dto.SessionResponse, 0, 8) // Preallocate for typical session count
	for session := range h.sessionService.GetActiveByUserID(user.ID) {
		sessions = append(sessions, dto.SessionResponse{
			ID:         session.ID.String(),
			DeviceInfo: session.DeviceInfo,
			IPAddress:  session.IPAddress,
			Created:    session.Created.Format(time.RFC3339),
			LastUsed:   session.LastUsed.Format(time.RFC3339),
			IsCurrent:  session.ID == currentSessionID,
		})
	}

	return &dto.ListSessionsResponse{Sessions: sessions}, nil
}

// RevokeSession revokes a specific session.
func (h *AuthHandler) RevokeSession(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.RevokeSessionRequest) (*dto.RevokeSessionResponse, error) {
	sessionID, err := jsonldb.DecodeID(req.SessionID)
	if err != nil {
		return nil, dto.InvalidField("session_id", "invalid session ID format")
	}

	// Verify the session belongs to the user
	session, err := h.sessionService.Get(sessionID)
	if err != nil {
		return nil, dto.NewAPIError(404, dto.ErrorCodeNotFound, "Session not found")
	}
	if session.UserID != user.ID {
		return nil, dto.NewAPIError(403, dto.ErrorCodeForbidden, "Cannot revoke another user's session")
	}

	if err := h.sessionService.Revoke(sessionID); err != nil {
		return nil, dto.InternalWithError("Failed to revoke session", err)
	}

	return &dto.RevokeSessionResponse{Ok: true}, nil
}

// RevokeAllSessions revokes all sessions for the current user except the current one.
func (h *AuthHandler) RevokeAllSessions(ctx context.Context, _ jsonldb.ID, user *identity.User, _ *dto.RevokeAllSessionsRequest) (*dto.RevokeAllSessionsResponse, error) {
	currentSessionID := reqctx.SessionID(ctx)

	// Collect session IDs to revoke (excluding current)
	var toRevoke []jsonldb.ID
	for session := range h.sessionService.GetActiveByUserID(user.ID) {
		if session.ID != currentSessionID {
			toRevoke = append(toRevoke, session.ID)
		}
	}

	// Revoke each session
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
