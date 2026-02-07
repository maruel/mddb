// Handles OAuth2 authentication with external providers.

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/microsoft"
)

const linkingStatePrefix = "link:"

// OAuthHandler handles OAuth2 authentication for multiple providers.
type OAuthHandler struct {
	svc       *Services
	cfg       *Config
	providers map[identity.OAuthProvider]*oauth2.Config
}

// NewOAuthHandler creates a new OAuth handler.
func NewOAuthHandler(svc *Services, cfg *Config) *OAuthHandler {
	return &OAuthHandler{
		svc:       svc,
		cfg:       cfg,
		providers: make(map[identity.OAuthProvider]*oauth2.Config),
	}
}

// oauthUserInfo holds user info fetched from an OAuth provider.
type oauthUserInfo struct {
	ID        string
	Email     string
	Name      string
	AvatarURL string
}

// AddProvider adds an OAuth2 provider configuration.
func (h *OAuthHandler) AddProvider(name identity.OAuthProvider, clientID, clientSecret, redirectURL string) {
	var endpoint oauth2.Endpoint
	var scopes []string

	switch name {
	case identity.OAuthProviderGoogle:
		endpoint = google.Endpoint
		scopes = []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		}
	case identity.OAuthProviderMicrosoft:
		endpoint = microsoft.AzureADEndpoint("common")
		scopes = []string{"openid", "profile", "email", "User.Read"}
	case identity.OAuthProviderGitHub:
		endpoint = github.Endpoint
		scopes = []string{"read:user", "user:email"}
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
		providers = append(providers, dto.OAuthProvider(string(name)))
	}
	return &dto.ProvidersResponse{Providers: providers}, nil
}

// LinkOAuth initiates linking an OAuth provider to an existing account.
func (h *OAuthHandler) LinkOAuth(_ context.Context, user *identity.User, req *dto.LinkOAuthAccountRequest) (*dto.LinkOAuthAccountResponse, error) {
	provider := identity.OAuthProvider(req.Provider)
	config, ok := h.providers[provider]
	if !ok {
		return nil, dto.InvalidProvider()
	}

	// Check if provider is already linked
	for _, ident := range user.OAuthIdentities {
		if ident.Provider == provider {
			return nil, dto.ProviderAlreadyLinked(string(provider))
		}
	}

	// Generate linking state with user ID
	state := generateLinkingState(user.ID, provider)

	var opts []oauth2.AuthCodeOption
	if provider == identity.OAuthProviderGoogle {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	authURL := config.AuthCodeURL(state, opts...)

	return &dto.LinkOAuthAccountResponse{RedirectURL: authURL}, nil
}

// UnlinkOAuth removes an OAuth provider from the user's account.
func (h *OAuthHandler) UnlinkOAuth(_ context.Context, user *identity.User, req *dto.UnlinkOAuthAccountRequest) (*dto.UnlinkOAuthAccountResponse, error) {
	provider := identity.OAuthProvider(req.Provider)

	// Check if provider is linked
	found := false
	for _, ident := range user.OAuthIdentities {
		if ident.Provider == provider {
			found = true
			break
		}
	}
	if !found {
		return nil, dto.ProviderNotLinked(string(provider))
	}

	// Check if user has another auth method (password or other OAuth)
	hasPassword := h.svc.User.HasPassword(user.ID)
	otherOAuthCount := len(user.OAuthIdentities) - 1

	if !hasPassword && otherOAuthCount == 0 {
		return nil, dto.CannotUnlinkOnlyAuth()
	}

	// Remove the OAuth identity
	if _, err := h.svc.User.Modify(user.ID, func(u *identity.User) error {
		newIdentities := make([]identity.OAuthIdentity, 0, len(u.OAuthIdentities)-1)
		for _, ident := range u.OAuthIdentities {
			if ident.Provider != provider {
				newIdentities = append(newIdentities, ident)
			}
		}
		u.OAuthIdentities = newIdentities
		return nil
	}); err != nil {
		return nil, dto.InternalWithError("Failed to unlink provider", err)
	}

	return &dto.UnlinkOAuthAccountResponse{Ok: true}, nil
}

// generateLinkingState creates an OAuth state for linking that includes the user ID.
func generateLinkingState(userID jsonldb.ID, provider identity.OAuthProvider) string {
	randomPart, _ := utils.GenerateToken(16)
	return fmt.Sprintf("%s%s:%s:%s", linkingStatePrefix, userID.String(), string(provider), randomPart)
}

// parseLinkingState parses a linking state and returns the user ID and provider.
// Returns zero ID if the state is not a linking state.
func parseLinkingState(state string) (jsonldb.ID, identity.OAuthProvider) {
	if !strings.HasPrefix(state, linkingStatePrefix) {
		return 0, ""
	}
	state = strings.TrimPrefix(state, linkingStatePrefix)
	parts := strings.SplitN(state, ":", 3)
	if len(parts) < 2 {
		return 0, ""
	}
	userID, err := jsonldb.DecodeID(parts[0])
	if err != nil {
		return 0, ""
	}
	return userID, identity.OAuthProvider(parts[1])
}

// LoginRedirect redirects the user to the OAuth provider.
func (h *OAuthHandler) LoginRedirect(w http.ResponseWriter, r *http.Request) {
	provider := identity.OAuthProvider(r.PathValue("provider"))
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
	if provider == identity.OAuthProviderGoogle {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	authURL := config.AuthCodeURL(state, opts...)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// Callback handles the OAuth provider callback.
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	provider := identity.OAuthProvider(r.PathValue("provider"))
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
		slog.ErrorContext(r.Context(), "OAuth: token exchange failed", "error", err, "provider", provider)
		writeErrorResponse(w, dto.OAuthError("token_exchange"))
		return
	}

	ctx := r.Context()
	client := config.Client(ctx, token)
	var userInfo struct {
		ID        string `json:"id"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		AvatarURL string
	}

	switch provider {
	case identity.OAuthProviderGoogle:
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
		var googleUser struct {
			ID      string `json:"id"`
			Email   string `json:"email"`
			Name    string `json:"name"`
			Picture string `json:"picture"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
			writeErrorResponse(w, dto.OAuthError("decode"))
			return
		}
		userInfo.ID = googleUser.ID
		userInfo.Email = googleUser.Email
		userInfo.Name = googleUser.Name
		userInfo.AvatarURL = googleUser.Picture
	case identity.OAuthProviderMicrosoft:
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

		// Fetch Microsoft profile photo and convert to base64 data URL
		userInfo.AvatarURL = fetchMicrosoftPhoto(ctx, client)
	case identity.OAuthProviderGitHub:
		resp, err := client.Get("https://api.github.com/user")
		if err != nil {
			writeErrorResponse(w, dto.OAuthError("user_info"))
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				slog.ErrorContext(ctx, "Failed to close GitHub API response body", "error", err)
			}
		}()
		var ghUser struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
			writeErrorResponse(w, dto.OAuthError("decode"))
			return
		}
		userInfo.ID = strconv.FormatInt(ghUser.ID, 10)
		userInfo.Name = ghUser.Name
		if userInfo.Name == "" {
			userInfo.Name = ghUser.Login
		}
		userInfo.Email = ghUser.Email
		userInfo.AvatarURL = ghUser.AvatarURL

		// GitHub may not return email if set to private - fetch from emails endpoint
		if userInfo.Email == "" {
			userInfo.Email = fetchGitHubPrimaryEmail(ctx, client)
		}
	}

	// Check if this is a linking request
	state := r.URL.Query().Get("state")
	linkingUserID, linkingProvider := parseLinkingState(state)
	if !linkingUserID.IsZero() {
		// This is a linking callback - verify provider matches
		if linkingProvider != provider {
			slog.WarnContext(ctx, "OAuth linking: provider mismatch", "expected", linkingProvider, "got", provider)
			writeErrorResponse(w, dto.BadRequest("Provider mismatch"))
			return
		}

		// Check if this OAuth identity is already claimed by another user
		existingUser, _ := h.svc.User.GetByOAuth(provider, userInfo.ID)
		if existingUser != nil && existingUser.ID != linkingUserID {
			slog.WarnContext(ctx, "OAuth linking: identity already claimed", "oauthID", userInfo.ID, "existingUser", existingUser.ID)
			// Redirect to frontend with error
			http.Redirect(w, r, "/?oauth_error=identity_claimed", http.StatusFound)
			return
		}

		// Get the user we're linking to
		linkingUser, err := h.svc.User.Get(linkingUserID)
		if err != nil {
			slog.ErrorContext(ctx, "OAuth linking: user not found", "userID", linkingUserID, "error", err)
			writeErrorResponse(w, dto.NotFound("user"))
			return
		}

		// Check if provider is already linked to this user
		for _, ident := range linkingUser.OAuthIdentities {
			if ident.Provider == provider {
				// Already linked - just redirect to success
				http.Redirect(w, r, "/?oauth_linked=true", http.StatusFound)
				return
			}
		}

		// Link the OAuth identity
		if _, err := h.svc.User.Modify(linkingUserID, func(u *identity.User) error {
			u.OAuthIdentities = append(u.OAuthIdentities, identity.OAuthIdentity{
				Provider:   provider,
				ProviderID: userInfo.ID,
				Email:      userInfo.Email,
				AvatarURL:  userInfo.AvatarURL,
				LastLogin:  storage.Now(),
			})
			return nil
		}); err != nil {
			slog.ErrorContext(ctx, "OAuth linking: failed to link identity", "error", err)
			writeErrorResponse(w, dto.Internal("oauth_link"))
			return
		}

		if err := h.svc.RootRepo.CommitDBChanges(ctx, git.Author{Name: linkingUser.Name, Email: linkingUser.Email}, "OAuth link "+string(provider)); err != nil {
			slog.ErrorContext(ctx, "OAuth linking: failed to commit", "error", err)
			writeErrorResponse(w, dto.Internal("commit"))
			return
		}

		slog.InfoContext(ctx, "OAuth: linked identity", "userID", linkingUserID, "provider", provider)
		http.Redirect(w, r, "/?oauth_linked=true", http.StatusFound)
		return
	}

	finishOAuthLogin(h.svc, h.cfg, w, r, provider, oauthUserInfo{
		ID:        userInfo.ID,
		Email:     userInfo.Email,
		Name:      userInfo.Name,
		AvatarURL: userInfo.AvatarURL,
	})
}

// finishOAuthLogin finds or creates a user from OAuth info, generates a JWT, and redirects.
func finishOAuthLogin(svc *Services, cfg *Config, w http.ResponseWriter, r *http.Request, provider identity.OAuthProvider, info oauthUserInfo) {
	ctx := r.Context()

	// Try to find user by OAuth ID
	user, err := svc.User.GetByOAuth(provider, info.ID)
	if err != nil {
		// Try to find user by email
		user, err = svc.User.GetByEmail(info.Email)
		if err != nil {
			// Create new user without organization (frontend will prompt for org creation)
			// Password is not used for OAuth users
			password, err := utils.GenerateToken(32)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to generate password for OAuth user", "err", err)
				writeErrorResponse(w, dto.Internal("password_generation"))
				return
			}
			user, err = svc.User.Create(info.Email, password, info.Name)
			if err != nil {
				writeErrorResponse(w, dto.Internal("user_creation"))
				return
			}
		}

		// Link OAuth identity and mark email as verified (OAuth emails are trusted)
		if _, err := svc.User.Modify(user.ID, func(u *identity.User) error {
			u.EmailVerified = true
			u.OAuthIdentities = append(u.OAuthIdentities, identity.OAuthIdentity{
				Provider:   provider,
				ProviderID: info.ID,
				Email:      info.Email,
				AvatarURL:  info.AvatarURL,
				LastLogin:  storage.Now(),
			})
			return nil
		}); err != nil {
			writeErrorResponse(w, dto.Internal("oauth_link"))
			return
		}
	} else {
		// Existing OAuth identity - update avatar URL and last login
		if _, err := svc.User.Modify(user.ID, func(u *identity.User) error {
			for i := range u.OAuthIdentities {
				if u.OAuthIdentities[i].Provider == provider && u.OAuthIdentities[i].ProviderID == info.ID {
					u.OAuthIdentities[i].AvatarURL = info.AvatarURL
					u.OAuthIdentities[i].LastLogin = storage.Now()
					break
				}
			}
			return nil
		}); err != nil {
			slog.WarnContext(ctx, "OAuth: failed to update identity", "err", err)
		}
	}

	// Generate JWT token with session tracking
	clientIP := reqctx.GetClientIP(r)
	userAgent := r.Header.Get("User-Agent")
	countryCode := reqctx.CountryCode(r.Context())
	jwtToken, err := cfg.GenerateTokenWithSession(svc.Session, user, clientIP, userAgent, countryCode)
	if err != nil {
		slog.ErrorContext(ctx, "OAuth: failed to generate token", "err", err, "userID", user.ID)
		writeErrorResponse(w, dto.Internal("token_generation"))
		return
	}

	if err := svc.RootRepo.CommitDBChanges(ctx, git.Author{Name: user.Name, Email: user.Email}, "OAuth login "+string(provider)); err != nil {
		slog.ErrorContext(ctx, "OAuth: failed to commit", "err", err, "userID", user.ID)
		writeErrorResponse(w, dto.Internal("commit"))
		return
	}

	slog.InfoContext(ctx, "OAuth: login successful, redirecting with token", "userID", user.ID, "email", user.Email)
	http.Redirect(w, r, "/?token="+url.QueryEscape(jwtToken), http.StatusFound)
}

// fetchMicrosoftPhoto fetches the user's profile photo from Microsoft Graph API
// and returns it as a base64 data URL. Returns empty string on failure.
func fetchMicrosoftPhoto(ctx context.Context, client *http.Client) string {
	resp, err := client.Get("https://graph.microsoft.com/v1.0/me/photo/$value")
	if err != nil {
		slog.DebugContext(ctx, "Failed to fetch Microsoft photo", "error", err)
		return ""
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.ErrorContext(ctx, "Failed to close Microsoft photo response body", "error", err)
		}
	}()

	// 404 means no photo set
	if resp.StatusCode == http.StatusNotFound {
		return ""
	}
	if resp.StatusCode != http.StatusOK {
		slog.DebugContext(ctx, "Microsoft photo request failed", "status", resp.StatusCode)
		return ""
	}

	// Read photo data (limit to 1MB to prevent abuse)
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		slog.DebugContext(ctx, "Failed to read Microsoft photo data", "error", err)
		return ""
	}

	// Determine content type from response header, default to JPEG
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// Encode as data URL
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", contentType, encoded)
}

// fetchGitHubPrimaryEmail fetches the user's primary verified email from GitHub API.
// Returns empty string on failure or if no verified primary email exists.
func fetchGitHubPrimaryEmail(ctx context.Context, client *http.Client) string {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		slog.DebugContext(ctx, "Failed to fetch GitHub emails", "error", err)
		return ""
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.ErrorContext(ctx, "Failed to close GitHub emails response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		slog.DebugContext(ctx, "GitHub emails request failed", "status", resp.StatusCode)
		return ""
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		slog.DebugContext(ctx, "Failed to decode GitHub emails", "error", err)
		return ""
	}

	// Find primary verified email
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}

	// Fall back to any verified email
	for _, e := range emails {
		if e.Verified {
			return e.Email
		}
	}

	return ""
}
