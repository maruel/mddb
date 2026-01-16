package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// OAuthHandler handles OAuth2 authentication.
type OAuthHandler struct {
	userService *storage.UserService
	orgService  *storage.OrganizationService
	authHandler *AuthHandler
	config      *oauth2.Config
}

// NewOAuthHandler creates a new OAuth handler.
func NewOAuthHandler(userService *storage.UserService, orgService *storage.OrganizationService, authHandler *AuthHandler, clientID, clientSecret, redirectURL string) *OAuthHandler {
	return &OAuthHandler{
		userService: userService,
		orgService:  orgService,
		authHandler: authHandler,
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
			Endpoint:     google.Endpoint,
		},
	}
}

// LoginRedirect redirects the user to the OAuth provider.
func (h *OAuthHandler) LoginRedirect(w http.ResponseWriter, r *http.Request) {
	// In a real app, use a secure state from session/cookie
	state, _ := storage.GenerateToken(16)
	url := h.config.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// Callback handles the OAuth provider callback.
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code missing", http.StatusBadRequest)
		return
	}

	token, err := h.config.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	client := h.config.Client(r.Context(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var googleUser struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
		http.Error(w, "Failed to decode user info", http.StatusInternalServerError)
		return
	}

	// Try to find user by OAuth ID
	user, err := h.userService.GetUserByOAuth("google", googleUser.ID)
	if err != nil {
		// Try to find user by email
		user, err = h.userService.GetUserByEmail(googleUser.Email)
		if err != nil {
			// Create new user if not found
			// For now, new OAuth users get a default organization like Register
			count, _ := h.userService.CountUsers()
			role := models.RoleViewer
			orgID := ""
			if count == 0 {
				role = models.RoleAdmin
				org, _ := h.orgService.CreateOrganization(r.Context(), "Default Organization")
				if org != nil {
					orgID = org.ID
				}
			} else {
				orgs, _ := h.orgService.ListOrganizations()
				if len(orgs) > 0 {
					orgID = orgs[0].ID
				}
			}

			// Password is not used for OAuth users
			password, _ := storage.GenerateToken(32)
			user, err = h.userService.CreateUser(googleUser.Email, password, googleUser.Name, role)
			if err != nil {
				http.Error(w, "Failed to create user", http.StatusInternalServerError)
				return
			}
			_ = h.userService.UpdateUserOrg(user.ID, orgID)
		}

		// Link OAuth identity
		_ = h.userService.LinkOAuthIdentity(user.ID, models.OAuthIdentity{
			Provider:   "google",
			ProviderID: googleUser.ID,
			Email:      googleUser.Email,
			LastLogin:  time.Now(),
		})
	}

	// Get fully populated user
	user, _ = h.userService.GetUser(user.ID)

	// Generate JWT token
	jwtToken, err := h.authHandler.GenerateToken(user)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Redirect back to frontend with token
	// In a real app, use a secure way to pass the token (e.g. cookie or temporary code)
	frontendURL := "/" // Default redirect
	http.Redirect(w, r, fmt.Sprintf("%s?token=%s", frontendURL, jwtToken), http.StatusTemporaryRedirect)
}
