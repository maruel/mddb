package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// AuthHandler handles authentication requests.
type AuthHandler struct {
	userService *identity.UserService
	memService  *identity.MembershipService
	orgService  *identity.OrganizationService
	fs          *content.FileStore
	jwtSecret   []byte
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(userService *identity.UserService, memService *identity.MembershipService, orgService *identity.OrganizationService, fs *content.FileStore, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		memService:  memService,
		orgService:  orgService,
		fs:          fs,
		jwtSecret:   []byte(jwtSecret),
	}
}

// Login handles user login and returns a JWT token.
func (h *AuthHandler) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, dto.MissingField("email or password")
	}

	user, err := h.userService.Authenticate(req.Email, req.Password)
	if err != nil {
		return nil, dto.NewAPIError(401, dto.ErrorCodeUnauthorized, "Invalid credentials")
	}

	token, err := h.GenerateToken(user)
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

	return &dto.LoginResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// Register handles user registration.
// Note: Organization creation is handled by the frontend after registration.
func (h *AuthHandler) Register(ctx context.Context, req dto.RegisterRequest) (*dto.LoginResponse, error) {
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

	token, err := h.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response (will have empty memberships for new users)
	uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	return &dto.LoginResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// GenerateToken generates a JWT token for the given user.
func (h *AuthHandler) GenerateToken(user *identity.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(time.Hour * 24).Unix(), // 24 hours
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

// Me returns the current user info.
func (h *AuthHandler) Me(ctx context.Context, _ jsonldb.ID, user *identity.User, req dto.MeRequest) (*dto.UserResponse, error) {
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
func (h *AuthHandler) CreateOrganization(ctx context.Context, _ jsonldb.ID, user *identity.User, req dto.CreateOrganizationRequest) (*dto.OrganizationResponse, error) {
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
		author := content.Author{Name: user.Name, Email: user.Email}
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
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		return
	}

	userResp.OrganizationID = orgIDStr

	for _, m := range userResp.Memberships {
		if m.OrganizationID == orgIDStr {
			userResp.Role = m.Role
			break
		}
	}

	// Fetch onboarding state
	if org, err := h.orgService.Get(orgID); err == nil {
		userResp.Onboarding = onboardingStatePtrToDTO(&org.Onboarding)
	}
}
