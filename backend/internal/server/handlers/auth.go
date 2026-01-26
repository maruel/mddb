// Handles user authentication, registration, and session management.

package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/backend/internal/email"
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
	userService              *identity.UserService
	orgMemService            *identity.OrganizationMembershipService
	wsMemService             *identity.WorkspaceMembershipService
	orgService               *identity.OrganizationService
	wsService                *identity.WorkspaceService
	sessionService           *identity.SessionService
	emailVerificationService *identity.EmailVerificationService
	emailService             *email.Service
	fs                       *content.FileStoreService
	jwtSecret                []byte
	baseURL                  string

	// Rate limiting for verification emails (1 per 10s per user)
	verifyRateLimitMu sync.Mutex
	verifyRateLimit   map[jsonldb.ID]time.Time
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(
	userService *identity.UserService,
	orgMemService *identity.OrganizationMembershipService,
	wsMemService *identity.WorkspaceMembershipService,
	orgService *identity.OrganizationService,
	wsService *identity.WorkspaceService,
	sessionService *identity.SessionService,
	emailVerificationService *identity.EmailVerificationService,
	emailService *email.Service,
	fs *content.FileStoreService,
	jwtSecret string,
	baseURL string,
) *AuthHandler {
	return &AuthHandler{
		userService:              userService,
		orgMemService:            orgMemService,
		wsMemService:             wsMemService,
		orgService:               orgService,
		wsService:                wsService,
		sessionService:           sessionService,
		emailVerificationService: emailVerificationService,
		emailService:             emailService,
		fs:                       fs,
		jwtSecret:                []byte(jwtSecret),
		baseURL:                  baseURL,
		verifyRateLimit:          make(map[jsonldb.ID]time.Time),
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

	// Send verification email if SMTP is configured
	if h.emailService != nil && h.emailVerificationService != nil {
		h.sendVerificationEmailAsync(ctx, user.ID, user.Email, user.Name)
	}

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

// ChangeEmail changes the user's email address after password verification.
func (h *AuthHandler) ChangeEmail(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
	// Verify password
	if !h.userService.VerifyPassword(user.ID, req.Password) {
		return nil, dto.NewAPIError(401, dto.ErrorCodeUnauthorized, "Invalid password")
	}

	// Check if new email is same as current
	if req.NewEmail == user.Email {
		return &dto.ChangeEmailResponse{
			Ok:            true,
			EmailVerified: user.EmailVerified,
			Message:       "Email unchanged",
		}, nil
	}

	// Check if new email is already in use by another account
	existingUser, _ := h.userService.GetByEmail(req.NewEmail)
	if existingUser != nil && existingUser.ID != user.ID {
		return nil, dto.EmailInUse()
	}

	// Update email and reset EmailVerified
	if _, err := h.userService.Modify(user.ID, func(u *identity.User) error {
		u.Email = req.NewEmail
		u.EmailVerified = false
		return nil
	}); err != nil {
		return nil, dto.InternalWithError("Failed to update email", err)
	}

	// Send verification email if SMTP is configured
	if h.emailService != nil && h.emailVerificationService != nil {
		h.sendVerificationEmailAsync(ctx, user.ID, req.NewEmail, user.Name)
	}

	return &dto.ChangeEmailResponse{
		Ok:            true,
		EmailVerified: false,
		Message:       "Email changed successfully. Please verify your new email.",
	}, nil
}

// SendVerificationEmail sends a verification email to the current user.
func (h *AuthHandler) SendVerificationEmail(ctx context.Context, _ jsonldb.ID, user *identity.User, _ *dto.SendVerificationEmailRequest) (*dto.SendVerificationEmailResponse, error) {
	// Check if SMTP is configured
	if h.emailService == nil {
		return nil, dto.NewAPIError(501, dto.ErrorCodeNotImplemented, "Email service not configured")
	}
	if h.emailVerificationService == nil {
		return nil, dto.NewAPIError(501, dto.ErrorCodeNotImplemented, "Email verification service not configured")
	}

	// Check if email is already verified
	if user.EmailVerified {
		return &dto.SendVerificationEmailResponse{
			Ok:      true,
			Message: "Email is already verified",
		}, nil
	}

	// Rate limit: 1 per 10 seconds per user
	h.verifyRateLimitMu.Lock()
	lastSent, exists := h.verifyRateLimit[user.ID]
	if exists && time.Since(lastSent) < 10*time.Second {
		h.verifyRateLimitMu.Unlock()
		return nil, dto.NewAPIError(429, dto.ErrorCodeRateLimitExceeded, "Please wait before requesting another verification email")
	}
	h.verifyRateLimit[user.ID] = time.Now()
	h.verifyRateLimitMu.Unlock()

	// Create verification token
	verification, err := h.emailVerificationService.Create(user.ID, user.Email)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create verification token", err)
	}

	// Build verify URL
	verifyURL := h.baseURL + "/api/auth/email/verify?token=" + verification.Token

	// Send email
	if err := h.emailService.SendVerification(ctx, user.Email, user.Name, verifyURL); err != nil {
		slog.ErrorContext(ctx, "Failed to send verification email", "err", err, "user_id", user.ID)
		return nil, dto.InternalWithError("Failed to send verification email", err)
	}

	slog.InfoContext(ctx, "Verification email sent", "user_id", user.ID, "email", user.Email)

	return &dto.SendVerificationEmailResponse{
		Ok:      true,
		Message: "Verification email sent",
	}, nil
}

// VerifyEmail verifies the user's email via magic link token.
// This is a public endpoint (no auth required) that redirects to the frontend.
func (h *AuthHandler) VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) error {
	if h.emailVerificationService == nil {
		return dto.NewAPIError(501, dto.ErrorCodeNotImplemented, "Email verification service not configured")
	}

	// Get verification by token
	verification, err := h.emailVerificationService.GetByToken(req.Token)
	if err != nil {
		return dto.NewAPIError(400, dto.ErrorCodeValidationFailed, "Invalid or expired verification token")
	}

	// Check if expired
	if h.emailVerificationService.IsExpired(verification) {
		// Delete expired token
		_ = h.emailVerificationService.Delete(verification.ID)
		return dto.NewAPIError(400, dto.ErrorCodeValidationFailed, "Verification token has expired")
	}

	// Update user's EmailVerified status
	_, err = h.userService.Modify(verification.UserID, func(u *identity.User) error {
		// Only verify if the email matches (user might have changed email since token was created)
		if u.Email != verification.Email {
			return dto.NewAPIError(400, dto.ErrorCodeValidationFailed, "Email address has changed since verification was requested")
		}
		u.EmailVerified = true
		return nil
	})
	if err != nil {
		return err
	}

	// Delete the used token
	if err := h.emailVerificationService.Delete(verification.ID); err != nil {
		slog.WarnContext(ctx, "Failed to delete verification token", "err", err, "id", verification.ID)
	}

	slog.InfoContext(ctx, "Email verified", "user_id", verification.UserID, "email", verification.Email)

	return nil
}

// sendVerificationEmailAsync sends a verification email in the background.
// Errors are logged but don't affect the caller.
func (h *AuthHandler) sendVerificationEmailAsync(ctx context.Context, userID jsonldb.ID, toEmail, name string) {
	go func() {
		// Create verification token
		verification, err := h.emailVerificationService.Create(userID, toEmail)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to create verification token", "err", err, "user_id", userID)
			return
		}

		// Build verify URL
		verifyURL := h.baseURL + "/api/auth/email/verify?token=" + verification.Token

		// Send email
		if err := h.emailService.SendVerification(ctx, toEmail, name, verifyURL); err != nil {
			slog.ErrorContext(ctx, "Failed to send verification email", "err", err, "user_id", userID)
			return
		}

		slog.InfoContext(ctx, "Verification email sent", "user_id", userID, "email", toEmail)
	}()
}

// VerifyEmailRedirect is an HTTP handler that verifies email and redirects to frontend.
func (h *AuthHandler) VerifyEmailRedirect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := r.URL.Query().Get("token")

	// Redirect URL base
	successURL := h.baseURL + "/settings?email_verified=true"
	errorURL := h.baseURL + "/settings?email_verified=false&error="

	if token == "" {
		http.Redirect(w, r, errorURL+"missing_token", http.StatusFound)
		return
	}

	req := &dto.VerifyEmailRequest{Token: token}
	if err := h.VerifyEmail(ctx, req); err != nil {
		slog.WarnContext(ctx, "Email verification failed", "err", err)
		http.Redirect(w, r, errorURL+"invalid_or_expired", http.StatusFound)
		return
	}

	http.Redirect(w, r, successURL, http.StatusFound)
}
