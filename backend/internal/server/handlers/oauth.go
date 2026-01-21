package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/entity"
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
	authHandler *AuthHandler
	providers   map[string]*oauth2.Config
}

// NewOAuthHandler creates a new OAuth handler.
func NewOAuthHandler(userService *identity.UserService, memService *identity.MembershipService, orgService *identity.OrganizationService, authHandler *AuthHandler) *OAuthHandler {
	return &OAuthHandler{
		userService: userService,
		memService:  memService,
		orgService:  orgService,
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

// LoginRedirect redirects the user to the OAuth provider.
func (h *OAuthHandler) LoginRedirect(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	config, ok := h.providers[provider]
	if !ok {
		writeErrorResponse(w, dto.InvalidProvider())
		return
	}

	// In a real app, use a secure state from session/cookie
	state, _ := utils.GenerateToken(16)
	url := config.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
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

	client := config.Client(r.Context(), token)
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
		defer func() { _ = resp.Body.Close() }()
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
		defer func() { _ = resp.Body.Close() }()

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
			// Create new user if not found
			orgName := userInfo.Name + "'s Organization"
			org, _ := h.orgService.Create(r.Context(), orgName)

			// Password is not used for OAuth users
			password, _ := utils.GenerateToken(32)
			user, err = h.userService.Create(userInfo.Email, password, userInfo.Name)
			if err != nil {
				writeErrorResponse(w, dto.Internal("user_creation"))
				return
			}
			if org != nil && !org.ID.IsZero() {
				_, _ = h.memService.Create(user.ID, org.ID, entity.UserRoleAdmin)
			}
		}

		// Link OAuth identity
		_, _ = h.userService.Modify(user.ID, func(u *entity.User) error {
			u.OAuthIdentities = append(u.OAuthIdentities, entity.OAuthIdentity{
				Provider:   provider,
				ProviderID: userInfo.ID,
				Email:      userInfo.Email,
				LastLogin:  time.Now(),
			})
			return nil
		})
	}

	// Generate JWT token
	jwtToken, err := h.authHandler.GenerateToken(user)
	if err != nil {
		writeErrorResponse(w, dto.Internal("token_generation"))
		return
	}

	// Redirect back to frontend with token
	frontendURL := "/" // Default redirect
	http.Redirect(w, r, fmt.Sprintf("%s?token=%s", frontendURL, jwtToken), http.StatusTemporaryRedirect)
}
