package handlers

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
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
func (h *AuthHandler) Login(ctx context.Context, req models.LoginRequest) (*models.LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, models.MissingField("email or password")
	}

	user, err := h.userService.Authenticate(req.Email, req.Password)
	if err != nil {
		return nil, models.NewAPIError(401, models.ErrorCodeUnauthorized, "Invalid credentials")
	}

	token, err := h.GenerateToken(user)
	if err != nil {
		return nil, models.InternalWithError("Failed to generate token", err)
	}

	// Build user response
	userResp, err := h.userService.GetUserResponse(user.ID.String())
	if err != nil {
		return nil, models.InternalWithError("Failed to get user response", err)
	}

	// Set active context to first membership
	if len(userResp.Memberships) > 0 {
		h.PopulateActiveContext(userResp, userResp.Memberships[0].OrganizationID)
	}

	return &models.LoginResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// Register handles user registration.
func (h *AuthHandler) Register(ctx context.Context, req models.RegisterRequest) (*models.LoginResponse, error) {
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return nil, models.MissingField("email, password, or name")
	}

	// Check if user already exists
	_, err := h.userService.GetUserByEmail(req.Email)
	if err == nil {
		return nil, models.NewAPIError(409, models.ErrorCodeConflict, "User already exists")
	}

	// Create an organization only for this user
	orgName := req.Name + "'s Organization"
	org, err := h.orgService.CreateOrganization(ctx, orgName)
	if err != nil {
		return nil, models.InternalWithError("Failed to create organization", err)
	}
	orgID := org.ID

	user, err := h.userService.CreateUser(req.Email, req.Password, req.Name, models.UserRoleAdmin)
	if err != nil {
		return nil, models.InternalWithError("Failed to create user", err)
	}

	// Create initial membership (admin of their own org)
	if err := h.userService.UpdateUserRole(user.ID.String(), orgID.String(), models.UserRoleAdmin); err != nil {
		return nil, models.InternalWithError("Failed to create initial membership", err)
	}

	token, err := h.GenerateToken(user)
	if err != nil {
		return nil, models.InternalWithError("Failed to generate token", err)
	}

	// Build user response with memberships
	userResp, err := h.userService.GetUserResponse(user.ID.String())
	if err != nil {
		return nil, models.InternalWithError("Failed to get user response", err)
	}

	// Set active context to first membership (the newly created org)
	if len(userResp.Memberships) > 0 {
		h.PopulateActiveContext(userResp, userResp.Memberships[0].OrganizationID)
	}

	return &models.LoginResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// GenerateToken generates a JWT token for the given user.
func (h *AuthHandler) GenerateToken(user *models.User) (string, error) {
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
func (h *AuthHandler) Me(ctx context.Context, req models.MeRequest) (*models.UserResponse, error) {
	// User info should be in context if authenticated via middleware
	user, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, models.NewAPIError(401, models.ErrorCodeUnauthorized, "Unauthorized")
	}

	// Build user response with memberships
	userResp, err := h.userService.GetUserResponse(user.ID.String())
	if err != nil {
		return nil, models.InternalWithError("Failed to get user response", err)
	}

	// For /api/auth/me, we need to decide which org is "active"
	// For now, use the first membership if not specified
	if len(userResp.Memberships) > 0 {
		h.PopulateActiveContext(userResp, userResp.Memberships[0].OrganizationID)
	}

	return userResp, nil
}

// PopulateActiveContext populates organization-specific fields in the UserResponse.
func (h *AuthHandler) PopulateActiveContext(userResp *models.UserResponse, orgIDStr string) {
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
		userResp.Onboarding = &org.Onboarding
	}
}
