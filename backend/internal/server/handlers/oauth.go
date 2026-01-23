package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/microsoft"
)

// OAuthHandler handles OAuth2 authentication for multiple providers.
type OAuthHandler struct {
	userService *identity.UserService
	memService  *identity.MembershipService
	orgService  *identity.OrganizationService
	fs          *content.FileStore
	authHandler *AuthHandler
	providers   map[string]*oauth2.Config
}

// NewOAuthHandler creates a new OAuth handler.
func NewOAuthHandler(userService *identity.UserService, memService *identity.MembershipService, orgService *identity.OrganizationService, fs *content.FileStore, authHandler *AuthHandler) *OAuthHandler {
	return &OAuthHandler{
		userService: userService,
		memService:  memService,
		orgService:  orgService,
		fs:          fs,
		authHandler: authHandler,
		providers:   make(map[string]*oauth2.Config),
	}
}

// AddProvider adds an OAuth2 provider configuration.
func (h *OAuthHandler) AddProvider(name, clientID, clientSecret, redirectURL string) {
	var endpoint oauth2.Endpoint
	var scopes []string

	switch name {
	case "google":
		endpoint = google.Endpoint
		scopes = []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		}
	case "microsoft":
		endpoint = microsoft.AzureADEndpoint("common")
		scopes = []string{"openid", "profile", "email", "User.Read"}
	default:
		return
	}

	h.providers[name] = &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint:     endpoint,
	}
}

// ListProviders returns the list of configured OAuth providers.
func (h *OAuthHandler) ListProviders(_ context.Context, _ *dto.ProvidersRequest) (*dto.ProvidersResponse, error) {
	providers := make([]dto.OAuthProvider, 0, len(h.providers))
	for name := range h.providers {
		providers = append(providers, dto.OAuthProvider(name))
	}
	return &dto.ProvidersResponse{Providers: providers}, nil
}

// LoginRedirect redirects the user to the OAuth provider.
func (h *OAuthHandler) LoginRedirect(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	config, ok := h.providers[provider]
	if !ok {
		writeErrorResponse(w, dto.InvalidProvider())
		return
	}

	// In a real app, use a secure state from session/cookie
	state, err := utils.GenerateToken(16)
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to generate OAuth state token", "error", err)
		writeErrorResponse(w, dto.Internal("state_generation"))
		return
	}
	var opts []oauth2.AuthCodeOption
	if provider == "google" {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	authURL := config.AuthCodeURL(state, opts...)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// Callback handles the OAuth provider callback.
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	config, ok := h.providers[provider]
	if !ok {
		writeErrorResponse(w, dto.InvalidProvider())
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeErrorResponse(w, dto.MissingField("code"))
		return
	}

	token, err := config.Exchange(r.Context(), code)
	if err != nil {
		writeErrorResponse(w, dto.OAuthError("token_exchange"))
		return
	}

	ctx := r.Context()
	client := config.Client(ctx, token)
	var userInfo struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	switch provider {
	case "google":
		resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
		if err != nil {
			writeErrorResponse(w, dto.OAuthError("user_info"))
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				slog.ErrorContext(ctx, "Failed to close Google API response body", "error", err)
			}
		}()
		if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
			writeErrorResponse(w, dto.OAuthError("decode"))
			return
		}
	case "microsoft":
		resp, err := client.Get("https://graph.microsoft.com/v1.0/me")
		if err != nil {
			writeErrorResponse(w, dto.OAuthError("user_info"))
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				slog.ErrorContext(ctx, "Failed to close Microsoft API response body", "error", err)
			}
		}()

		var msUser struct {
			ID                string `json:"id"`
			DisplayName       string `json:"displayName"`
			UserPrincipalName string `json:"userPrincipalName"`
			Mail              string `json:"mail"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&msUser); err != nil {
			writeErrorResponse(w, dto.OAuthError("decode"))
			return
		}
		userInfo.ID = msUser.ID
		userInfo.Name = msUser.DisplayName
		userInfo.Email = msUser.Mail
		if userInfo.Email == "" {
			userInfo.Email = msUser.UserPrincipalName
		}
	}

	// Try to find user by OAuth ID
	user, err := h.userService.GetByOAuth(provider, userInfo.ID)
	if err != nil {
		// Try to find user by email
		user, err = h.userService.GetByEmail(userInfo.Email)
		if err != nil {
			// Create new user without organization (frontend will prompt for org creation)
			// Password is not used for OAuth users
			password, err := utils.GenerateToken(32)
			if err != nil {
				slog.ErrorContext(r.Context(), "Failed to generate password for OAuth user", "error", err)
				writeErrorResponse(w, dto.Internal("password_generation"))
				return
			}
			user, err = h.userService.Create(userInfo.Email, password, userInfo.Name)
			if err != nil {
				writeErrorResponse(w, dto.Internal("user_creation"))
				return
			}
		}

		// Link OAuth identity
		if _, err := h.userService.Modify(user.ID, func(u *identity.User) error {
			u.OAuthIdentities = append(u.OAuthIdentities, identity.OAuthIdentity{
				Provider:   provider,
				ProviderID: userInfo.ID,
				Email:      userInfo.Email,
				LastLogin:  time.Now(),
			})
			return nil
		}); err != nil {
			writeErrorResponse(w, dto.Internal("oauth_link"))
			return
		}
	}

	// Generate JWT token with session tracking
	clientIP := reqctx.GetClientIP(r)
	userAgent := r.Header.Get("User-Agent")
	jwtToken, err := h.authHandler.GenerateTokenWithSession(user, clientIP, userAgent)
	if err != nil {
		slog.ErrorContext(r.Context(), "OAuth: failed to generate token", "error", err, "userID", user.ID)
		writeErrorResponse(w, dto.Internal("token_generation"))
		return
	}

	slog.InfoContext(r.Context(), "OAuth: login successful, redirecting with token", "userID", user.ID, "email", user.Email)

	// Redirect back to frontend with token (URL-encode for safety)
	frontendURL := "/" // Default redirect
	http.Redirect(w, r, fmt.Sprintf("%s?token=%s", frontendURL, url.QueryEscape(jwtToken)), http.StatusFound)
}
