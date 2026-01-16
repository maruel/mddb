package handlers

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// AuthHandler handles authentication requests.
type AuthHandler struct {
	userService *storage.UserService
	jwtSecret   []byte
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(userService *storage.UserService, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		jwtSecret:   []byte(jwtSecret),
	}
}

// LoginRequest is a request to log in.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is a response from logging in.
type LoginResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

// RegisterRequest is a request to register a new user.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Login handles user login and returns a JWT token.
func (h *AuthHandler) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, errors.MissingField("email or password")
	}

	user, err := h.userService.Authenticate(req.Email, req.Password)
	if err != nil {
		return nil, errors.NewAPIError(401, errors.ErrUnauthorized, "Invalid credentials")
	}

	token, err := h.generateToken(user)
	if err != nil {
		return nil, errors.InternalWithError("Failed to generate token", err)
	}

	return &LoginResponse{
		Token: token,
		User:  user,
	}, nil
}

// Register handles user registration.
func (h *AuthHandler) Register(ctx context.Context, req RegisterRequest) (*LoginResponse, error) {
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return nil, errors.MissingField("email, password, or name")
	}

	// Check if user already exists
	_, err := h.userService.GetUserByEmail(req.Email)
	if err == nil {
		return nil, errors.NewAPIError(409, errors.ErrConflict, "User already exists")
	}

	user, err := h.userService.CreateUser(req.Email, req.Password, req.Name)
	if err != nil {
		return nil, errors.InternalWithError("Failed to create user", err)
	}

	token, err := h.generateToken(user)
	if err != nil {
		return nil, errors.InternalWithError("Failed to generate token", err)
	}

	return &LoginResponse{
		Token: token,
		User:  user,
	}, nil
}

func (h *AuthHandler) generateToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"role":  user.Role,
		"org":   user.OrganizationID,
		"exp":   time.Now().Add(time.Hour * 24).Unix(), // 24 hours
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

// MeRequest is a request to get current user info.
type MeRequest struct{}

// Me returns the current user info from the context.
func (h *AuthHandler) Me(ctx context.Context, req MeRequest) (*models.User, error) {
	// User info should be in context if authenticated via middleware
	user, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, errors.NewAPIError(401, errors.ErrUnauthorized, "Unauthorized")
	}
	return user, nil
}
