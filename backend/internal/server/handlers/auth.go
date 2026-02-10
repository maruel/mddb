// Handles user authentication, registration, and session management.

package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// AuthHandler handles authentication requests.
type AuthHandler struct {
	svc *Services
	cfg *Config

	// Rate limiting for verification emails (1 per 10s per user)
	verifyRateLimitMu sync.Mutex
	verifyRateLimit   map[ksid.ID]time.Time
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(svc *Services, cfg *Config) *AuthHandler {
	return &AuthHandler{
		svc:             svc,
		cfg:             cfg,
		verifyRateLimit: make(map[ksid.ID]time.Time),
	}
}

// Login handles user login and returns a JWT token.
func (h *AuthHandler) Login(ctx context.Context, req *dto.LoginRequest) (*dto.AuthResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, dto.MissingField("email or password")
	}

	user, err := h.svc.User.Authenticate(req.Email, req.Password)
	if err != nil {
		return nil, dto.NewAPIError(401, dto.ErrorCodeUnauthorized, "Invalid credentials")
	}

	// Get request metadata from context
	clientIP := reqctx.ClientIP(ctx)
	userAgent := reqctx.UserAgent(ctx)
	countryCode := reqctx.CountryCode(ctx)

	token, err := h.cfg.GenerateTokenWithSession(h.svc.Session, user, clientIP, userAgent, countryCode)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response
	uwm, err := getUserWithMemberships(h.svc.User, h.svc.OrgMembership, h.svc.WSMembership, h.svc.Organization, h.svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set active context to first org/workspace
	uwm.populateActiveContext(userResp)

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

	// Check server-wide user quota
	if h.cfg.Quotas.MaxUsers > 0 && h.svc.User.Count() >= h.cfg.Quotas.MaxUsers {
		return nil, dto.QuotaExceeded("users", h.cfg.Quotas.MaxUsers)
	}

	// Check if user already exists
	_, err := h.svc.User.GetByEmail(req.Email)
	if err == nil {
		return nil, dto.NewAPIError(409, dto.ErrorCodeConflict, "User already exists")
	}

	user, err := h.svc.User.Create(req.Email, req.Password, req.Name)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create user", err)
	}

	// Get request metadata from context
	clientIP := reqctx.ClientIP(ctx)
	userAgent := reqctx.UserAgent(ctx)
	countryCode := reqctx.CountryCode(ctx)

	token, err := h.cfg.GenerateTokenWithSession(h.svc.Session, user, clientIP, userAgent, countryCode)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response (will have empty memberships for new users)
	uwm, err := getUserWithMemberships(h.svc.User, h.svc.OrgMembership, h.svc.WSMembership, h.svc.Organization, h.svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Send verification email if SMTP is configured (use default locale for new users)
	if h.svc.Email != nil && h.svc.EmailVerif != nil {
		h.sendVerificationEmailAsync(ctx, user.ID, user.Email, user.Name, email.DefaultLocale)
	}

	return &dto.AuthResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// GetMe returns the current user info.
func (h *AuthHandler) GetMe(ctx context.Context, user *identity.User, req *dto.GetMeRequest) (*dto.UserResponse, error) {
	// Build user response with memberships
	uwm, err := getUserWithMemberships(h.svc.User, h.svc.OrgMembership, h.svc.WSMembership, h.svc.Organization, h.svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Populate active context
	uwm.populateActiveContext(userResp)

	return userResp, nil
}

// CreateOrganization creates a new organization for the authenticated user.
func (h *AuthHandler) CreateOrganization(ctx context.Context, user *identity.User, req *dto.CreateOrganizationRequest) (*dto.OrganizationResponse, error) {
	if req.Name == "" {
		return nil, dto.MissingField("name")
	}

	// Check server-wide organization quota
	if h.cfg.Quotas.MaxOrganizations > 0 && h.svc.Organization.Count() >= h.cfg.Quotas.MaxOrganizations {
		return nil, dto.QuotaExceeded("organizations", h.cfg.Quotas.MaxOrganizations)
	}

	// Create the organization
	org, err := h.svc.Organization.Create(ctx, req.Name, user.Email)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create organization", err)
	}

	// Create org membership (user becomes owner of new org)
	if _, err := h.svc.OrgMembership.Create(user.ID, org.ID, identity.OrgRoleOwner); err != nil {
		return nil, dto.InternalWithError("Failed to create membership", err)
	}

	memberCount := h.svc.OrgMembership.CountOrgMemberships(org.ID)
	workspaceCount := h.svc.Workspace.CountByOrg(org.ID)
	return organizationToResponse(org, memberCount, workspaceCount), nil
}

// populateActiveContext populates organization/workspace context in the UserResponse.
func (uwm *userWithMemberships) populateActiveContext(userResp *dto.UserResponse) {
	// Try workspaces from LRU list in order (most recently used first)
	for _, savedWsID := range uwm.User.Settings.LastActiveWorkspaces {
		for _, ws := range uwm.WSMemberships {
			if ws.WorkspaceID != savedWsID {
				continue
			}
			// Found an accessible workspace from the LRU list
			userResp.WorkspaceID = ws.WorkspaceID
			userResp.WorkspaceName = ws.WorkspaceName
			userResp.WorkspaceRole = dto.WorkspaceRole(ws.Role)
			uwm.CurrentWSID = ws.WorkspaceID
			uwm.CurrentWSRole = ws.Role
			// Set the org context for this workspace
			userResp.OrganizationID = ws.OrganizationID
			uwm.CurrentOrgID = ws.OrganizationID
			// Find org role
			for _, org := range uwm.OrgMemberships {
				if org.OrganizationID == ws.OrganizationID {
					userResp.OrgRole = dto.OrganizationRole(org.Role)
					uwm.CurrentOrgRole = org.Role
					break
				}
			}
			return
		}
	}

	// No saved workspace accessible, fall through to default

	// Default: Set first org as active
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
		userResp.WorkspaceName = ws.WorkspaceName
		userResp.WorkspaceRole = dto.WorkspaceRole(ws.Role)
		uwm.CurrentWSID = ws.WorkspaceID
		uwm.CurrentWSRole = ws.Role
		break
	}
}

// Logout revokes the current session.
func (h *AuthHandler) Logout(ctx context.Context, _ *identity.User, _ *dto.LogoutRequest) (*dto.LogoutResponse, error) {
	sessionID := reqctx.SessionID(ctx)
	if sessionID.IsZero() {
		return &dto.LogoutResponse{Ok: true}, nil
	}

	if err := h.svc.Session.Revoke(sessionID); err != nil {
		slog.ErrorContext(ctx, "Failed to revoke session", "error", err, "session_id", sessionID)
		return nil, dto.InternalWithError("Failed to logout", err)
	}

	return &dto.LogoutResponse{Ok: true}, nil
}

// ListSessions returns all active sessions for the current user.
func (h *AuthHandler) ListSessions(ctx context.Context, user *identity.User, _ *dto.ListSessionsRequest) (*dto.ListSessionsResponse, error) {
	currentSessionID := reqctx.SessionID(ctx)

	sessions := make([]dto.SessionResponse, 0, 8)
	for session := range h.svc.Session.GetActiveByUserID(user.ID) {
		sessions = append(sessions, dto.SessionResponse{
			ID:          session.ID,
			DeviceInfo:  session.DeviceInfo,
			IPAddress:   session.IPAddress,
			CountryCode: session.CountryCode,
			Created:     session.Created,
			LastUsed:    session.LastUsed,
			IsCurrent:   session.ID == currentSessionID,
		})
	}

	return &dto.ListSessionsResponse{Sessions: sessions}, nil
}

// RevokeSession revokes a specific session.
func (h *AuthHandler) RevokeSession(ctx context.Context, user *identity.User, req *dto.RevokeSessionRequest) (*dto.RevokeSessionResponse, error) {
	// Verify the session belongs to the user
	session, err := h.svc.Session.Get(req.SessionID)
	if err != nil {
		return nil, dto.NewAPIError(404, dto.ErrorCodeNotFound, "Session not found")
	}
	if session.UserID != user.ID {
		return nil, dto.NewAPIError(403, dto.ErrorCodeForbidden, "Cannot revoke another user's session")
	}

	if err := h.svc.Session.Revoke(req.SessionID); err != nil {
		return nil, dto.InternalWithError("Failed to revoke session", err)
	}

	return &dto.RevokeSessionResponse{Ok: true}, nil
}

// RevokeAllSessions revokes all sessions for the current user except the current one.
func (h *AuthHandler) RevokeAllSessions(ctx context.Context, user *identity.User, _ *dto.RevokeAllSessionsRequest) (*dto.RevokeAllSessionsResponse, error) {
	currentSessionID := reqctx.SessionID(ctx)

	var toRevoke []ksid.ID
	for session := range h.svc.Session.GetActiveByUserID(user.ID) {
		if session.ID != currentSessionID {
			toRevoke = append(toRevoke, session.ID)
		}
	}

	revokedCount := 0
	for _, id := range toRevoke {
		if err := h.svc.Session.Revoke(id); err != nil {
			slog.ErrorContext(ctx, "Failed to revoke session", "error", err, "session_id", id)
			continue
		}
		revokedCount++
	}

	return &dto.RevokeAllSessionsResponse{RevokedCount: revokedCount}, nil
}

// ChangeEmail changes the user's email address after password verification.
func (h *AuthHandler) ChangeEmail(ctx context.Context, user *identity.User, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
	// Verify password
	if !h.svc.User.VerifyPassword(user.ID, req.Password) {
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
	existingUser, _ := h.svc.User.GetByEmail(req.NewEmail)
	if existingUser != nil && existingUser.ID != user.ID {
		return nil, dto.EmailInUse()
	}

	// Update email and reset EmailVerified
	if _, err := h.svc.User.Modify(user.ID, func(u *identity.User) error {
		u.Email = req.NewEmail
		u.EmailVerified = false
		return nil
	}); err != nil {
		return nil, dto.InternalWithError("Failed to update email", err)
	}

	// Send verification email if SMTP is configured
	if h.svc.Email != nil && h.svc.EmailVerif != nil {
		locale := email.ParseLocale(user.Settings.Language)
		h.sendVerificationEmailAsync(ctx, user.ID, req.NewEmail, user.Name, locale)
	}

	return &dto.ChangeEmailResponse{
		Ok:            true,
		EmailVerified: false,
		Message:       "Email changed successfully. Please verify your new email.",
	}, nil
}

// SendVerificationEmail sends a verification email to the current user.
func (h *AuthHandler) SendVerificationEmail(ctx context.Context, user *identity.User, _ *dto.SendVerificationEmailRequest) (*dto.SendVerificationEmailResponse, error) {
	// Check if SMTP is configured
	if h.svc.Email == nil {
		return nil, dto.NewAPIError(501, dto.ErrorCodeNotImplemented, "Email service not configured")
	}
	if h.svc.EmailVerif == nil {
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
	verification, err := h.svc.EmailVerif.Create(user.ID, user.Email)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create verification token", err)
	}

	// Build verify URL
	verifyURL := h.cfg.BaseURL + "/api/v1/auth/email/verify?token=" + verification.Token

	// Send email using user's language preference
	locale := email.ParseLocale(user.Settings.Language)
	if err := h.svc.Email.SendVerification(ctx, user.Email, user.Name, verifyURL, locale); err != nil {
		slog.ErrorContext(ctx, "Failed to send verification email", "err", err, "user_id", user.ID)
		return nil, dto.InternalWithError("Failed to send verification email", err)
	}

	slog.InfoContext(ctx, "Verification email sent", "user_id", user.ID, "email", user.Email, "locale", locale)

	return &dto.SendVerificationEmailResponse{
		Ok:      true,
		Message: "Verification email sent",
	}, nil
}

// VerifyEmail verifies the user's email via magic link token.
// This is a public endpoint (no auth required) that redirects to the frontend.
func (h *AuthHandler) VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) error {
	if h.svc.EmailVerif == nil {
		return dto.NewAPIError(501, dto.ErrorCodeNotImplemented, "Email verification service not configured")
	}

	// Get verification by token
	verification, err := h.svc.EmailVerif.GetByToken(req.Token)
	if err != nil {
		return dto.NewAPIError(400, dto.ErrorCodeValidationFailed, "Invalid or expired verification token")
	}

	// Check if expired
	if h.svc.EmailVerif.IsExpired(verification) {
		// Delete expired token
		_ = h.svc.EmailVerif.Delete(verification.ID)
		return dto.NewAPIError(400, dto.ErrorCodeValidationFailed, "Verification token has expired")
	}

	// Update user's EmailVerified status
	_, err = h.svc.User.Modify(verification.UserID, func(u *identity.User) error {
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
	if err := h.svc.EmailVerif.Delete(verification.ID); err != nil {
		slog.WarnContext(ctx, "Failed to delete verification token", "err", err, "id", verification.ID)
	}

	slog.InfoContext(ctx, "Email verified", "user_id", verification.UserID, "email", verification.Email)

	return nil
}

// sendVerificationEmailAsync sends a verification email in the background.
// Errors are logged but don't affect the caller.
func (h *AuthHandler) sendVerificationEmailAsync(ctx context.Context, userID ksid.ID, toEmail, name string, locale email.Locale) {
	go func() {
		// Create verification token
		verification, err := h.svc.EmailVerif.Create(userID, toEmail)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to create verification token", "err", err, "user_id", userID)
			return
		}

		if err := h.svc.RootRepo.CommitDBChanges(ctx, git.Author{}, "create email verification"); err != nil {
			slog.ErrorContext(ctx, "Failed to commit email verification", "err", err, "user_id", userID)
		}

		// Build verify URL
		verifyURL := h.cfg.BaseURL + "/api/v1/auth/email/verify?token=" + verification.Token

		// Send email
		if err := h.svc.Email.SendVerification(ctx, toEmail, name, verifyURL, locale); err != nil {
			slog.ErrorContext(ctx, "Failed to send verification email", "err", err, "user_id", userID)
			return
		}

		slog.InfoContext(ctx, "Verification email sent", "user_id", userID, "email", toEmail, "locale", locale)
	}()
}

// SetPassword sets or changes the user's password.
func (h *AuthHandler) SetPassword(_ context.Context, user *identity.User, req *dto.SetPasswordRequest) (*dto.OkResponse, error) {
	if err := h.svc.User.SetPassword(user.ID, req.CurrentPassword, req.NewPassword); err != nil {
		// Check for invalid credentials error (wrong current password)
		if err.Error() == "invalid credentials" {
			return nil, dto.NewAPIError(401, dto.ErrorCodeUnauthorized, "Invalid current password")
		}
		return nil, dto.InternalWithError("Failed to set password", err)
	}
	return &dto.OkResponse{Ok: true}, nil
}

// VerifyEmailRedirect is an HTTP handler that verifies email and redirects to frontend.
func (h *AuthHandler) VerifyEmailRedirect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := r.URL.Query().Get("token")

	// Redirect URL base
	successURL := h.cfg.BaseURL + "/settings?email_verified=true"
	errorURL := h.cfg.BaseURL + "/settings?email_verified=false&error="

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

	if err := h.svc.RootRepo.CommitDBChanges(ctx, git.Author{}, "verify email"); err != nil {
		slog.WarnContext(ctx, "Failed to commit email verification", "err", err)
		http.Redirect(w, r, errorURL+"commit_failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, successURL, http.StatusFound)
}
