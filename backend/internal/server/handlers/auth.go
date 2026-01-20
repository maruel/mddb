package handlers

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/backend/internal/dto"
	"github.com/maruel/mddb/backend/internal/entity"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

// AuthHandler handles authentication requests.
type AuthHandler struct {
	userService *storage.UserService
	orgService  *storage.OrganizationService
	jwtSecret   []byte
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(userService *storage.UserService, orgService *storage.OrganizationService, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		orgService:  orgService,
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
	uwm, err := h.userService.GetUserWithMemberships(user.ID.String())
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
func (h *AuthHandler) Register(ctx context.Context, req dto.RegisterRequest) (*dto.LoginResponse, error) {
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return nil, dto.MissingField("email, password, or name")
	}

	// Check if user already exists
	_, err := h.userService.GetUserByEmail(req.Email)
	if err == nil {
		return nil, dto.NewAPIError(409, dto.ErrorCodeConflict, "User already exists")
	}

	// Create an organization only for this user
	orgName := req.Name + "'s Organization"
	org, err := h.orgService.CreateOrganization(ctx, orgName)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create organization", err)
	}
	orgID := org.ID

	user, err := h.userService.CreateUser(req.Email, req.Password, req.Name, entity.UserRoleAdmin)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create user", err)
	}

	// Create initial membership (admin of their own org)
	if err := h.userService.UpdateUserRole(user.ID.String(), orgID.String(), entity.UserRoleAdmin); err != nil {
		return nil, dto.InternalWithError("Failed to create initial membership", err)
	}

	token, err := h.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response with memberships
	uwm, err := h.userService.GetUserWithMemberships(user.ID.String())
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set active context to first membership (the newly created org)
	if len(userResp.Memberships) > 0 {
		h.PopulateActiveContext(userResp, userResp.Memberships[0].OrganizationID)
	}

	return &dto.LoginResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// GenerateToken generates a JWT token for the given user.
func (h *AuthHandler) GenerateToken(user *entity.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(time.Hour * 24).Unix(), // 24 hours
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

// Me returns the current user info from the context.
func (h *AuthHandler) Me(ctx context.Context, req dto.MeRequest) (*dto.UserResponse, error) {
	// User info should be in context if authenticated via middleware
	user, ok := ctx.Value(entity.UserKey).(*entity.User)
	if !ok {
		return nil, dto.NewAPIError(401, dto.ErrorCodeUnauthorized, "Unauthorized")
	}

	// Build user response with memberships
	uwm, err := h.userService.GetUserWithMemberships(user.ID.String())
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
	if org, err := h.orgService.GetOrganization(orgID); err == nil {
		userResp.Onboarding = onboardingStatePtrToDTO(&org.Onboarding)
	}
}
