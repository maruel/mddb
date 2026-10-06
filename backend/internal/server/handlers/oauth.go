// Handles OAuth2 authentication with external providers.

package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/gomode/oauth/oauthserver"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/microsoft"
)

const (
	// oauthStateCookie holds the HMAC-signed state of the OAuth flow the
	// browser started. The callback accepts only the state it carries, which
	// stops login CSRF and binds account linking to the user who requested it.
	oauthStateCookie     = "mddb_oauth_state"
	oauthStateCookiePath = "/api/v1/auth/oauth"
	oauthStateMaxAge     = 600 // seconds
	linkingStatePrefix   = "link:"
)

// OAuthHandler handles OAuth2 authentication for multiple providers.
type OAuthHandler struct {
	svc       *Services
	cfg       *Config
	stateKey  []byte
	providers map[identity.OAuthProvider]*oauth2.Config
}

// NewOAuthHandler creates a new OAuth handler.
func NewOAuthHandler(svc *Services, cfg *Config) *OAuthHandler {
	// Derive a dedicated state key so a state signature never doubles as a
	// JWT or signed asset URL signature, which use the raw JWT secret.
	mac := hmac.New(sha256.New, cfg.JWTSecret)
	mac.Write([]byte("mddb oauth state v1"))
	return &OAuthHandler{
		svc:       svc,
		cfg:       cfg,
		stateKey:  mac.Sum(nil),
		providers: make(map[identity.OAuthProvider]*oauth2.Config),
	}
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

// LinkOAuth initiates linking an OAuth provider to the authenticated user's
// account and responds with a dto.LinkOAuthAccountResponse.
//
// It reads the user from reqctx. The response also sets the state cookie that
// names this user, so only the browser that holds the user's session can
// complete the link in Callback.
func (h *OAuthHandler) LinkOAuth(w http.ResponseWriter, r *http.Request) {
	user := reqctx.User(r.Context())
	var req dto.LinkOAuthAccountRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		writeErrorResponse(w, dto.BadRequest("Invalid request body"))
		return
	}
	if err := req.Validate(); err != nil {
		writeErrorResponse(w, err)
		return
	}
	provider := identity.OAuthProvider(req.Provider)
	config, ok := h.providers[provider]
	if !ok {
		writeErrorResponse(w, dto.InvalidProvider())
		return
	}
	for _, ident := range user.OAuthIdentities {
		if ident.Provider == provider {
			writeErrorResponse(w, dto.ProviderAlreadyLinked(dto.OAuthProvider(provider)))
			return
		}
	}

	state, err := h.issueState(w, linkingStatePrefix+user.ID.String()+":"+string(provider)+":")
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to generate OAuth state", "err", err)
		writeErrorResponse(w, dto.Internal("state_generation"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&dto.LinkOAuthAccountResponse{RedirectURL: authCodeURL(config, provider, state)}); err != nil {
		slog.ErrorContext(r.Context(), "Failed to encode response", "err", err)
	}
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
		return nil, dto.ProviderNotLinked(dto.OAuthProvider(provider))
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

// parseLinkingState parses a verified linking state and returns the user ID
// and provider. Returns zero ID if the state is not a linking state.
func parseLinkingState(state string) (ksid.ID, identity.OAuthProvider) {
	if !strings.HasPrefix(state, linkingStatePrefix) {
		return 0, ""
	}
	state = strings.TrimPrefix(state, linkingStatePrefix)
	parts := strings.SplitN(state, ":", 3)
	if len(parts) < 2 {
		return 0, ""
	}
	userID, err := ksid.Parse(parts[0])
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

	state, err := h.issueState(w, "")
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to generate OAuth state", "err", err)
		writeErrorResponse(w, dto.Internal("state_generation"))
		return
	}
	http.Redirect(w, r, authCodeURL(config, provider, state), http.StatusTemporaryRedirect)
}

// Callback handles the OAuth provider callback.
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	provider := identity.OAuthProvider(r.PathValue("provider"))
	config, ok := h.providers[provider]
	if !ok {
		writeErrorResponse(w, dto.InvalidProvider())
		return
	}

	// The state cookie is single use: clear it whatever the outcome.
	http.SetCookie(w, h.stateCookie("", -1))
	state, err := h.verifyState(r)
	if err != nil {
		slog.WarnContext(r.Context(), "OAuth: rejected callback state", "err", err, "provider", provider)
		writeErrorResponse(w, err)
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
	var userInfo oauthUserInfo

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
		var googleUser googleUserInfo
		if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
			writeErrorResponse(w, dto.OAuthError("decode"))
			return
		}
		userInfo.ID = googleUser.ID
		verified := ""
		if googleUser.VerifiedEmail {
			verified = googleUser.Email
		}
		userInfo.VerifiedEmail = knownEmail(verified)
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

		// A tenant admin sets mail to any address, and Graph does not report
		// whether anyone verified mail or userPrincipalName; the email comes
		// from the ID token instead.
		var msUser graphUser
		if err := json.NewDecoder(resp.Body).Decode(&msUser); err != nil {
			writeErrorResponse(w, dto.OAuthError("decode"))
			return
		}
		userInfo.ID = msUser.ID
		userInfo.Name = msUser.DisplayName
		userInfo.VerifiedEmail = func() (string, error) {
			email, err := microsoftVerifiedEmail(token, config.ClientID)
			if err != nil {
				slog.WarnContext(ctx, "OAuth: rejected Microsoft ID token", "err", err)
				return "", dto.BadRequest("Invalid Microsoft ID token")
			}
			return email, nil
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
		// The profile email is the user's chosen public address, which GitHub
		// does not document as verified; read the email from /user/emails.
		var ghUser githubUser
		if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
			writeErrorResponse(w, dto.OAuthError("decode"))
			return
		}
		userInfo.ID = strconv.FormatInt(ghUser.ID, 10)
		userInfo.Name = ghUser.Name
		if userInfo.Name == "" {
			userInfo.Name = ghUser.Login
		}
		userInfo.AvatarURL = ghUser.AvatarURL
		userInfo.VerifiedEmail = func() (string, error) {
			email, err := fetchGitHubVerifiedEmail(client)
			if err != nil {
				slog.ErrorContext(ctx, "OAuth: failed to fetch GitHub emails", "err", err)
				return "", dto.OAuthError("user_info")
			}
			return email, nil
		}
	}

	// Check if this is a linking request
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

		// The signed-in user proves the link; the email is only a label, so a
		// failed lookup stores none instead of blocking the link.
		email, err := userInfo.VerifiedEmail()
		if err != nil {
			email = ""
		}

		// Link the OAuth identity
		if _, err := h.svc.User.Modify(linkingUserID, func(u *identity.User) error {
			u.OAuthIdentities = append(u.OAuthIdentities, identity.OAuthIdentity{
				Provider:   provider,
				ProviderID: userInfo.ID,
				Email:      email,
				AvatarURL:  userInfo.AvatarURL,
				LastLogin:  storage.Now(),
			})
			return nil
		}); err != nil {
			slog.ErrorContext(ctx, "OAuth linking: failed to link identity", "error", err)
			writeErrorResponse(w, dto.Internal("oauth_link"))
			return
		}

		if err := h.svc.RootRepo.CommitDBChanges(ctx, GitAuthor(linkingUser), "OAuth link "+string(provider)); err != nil {
			slog.ErrorContext(ctx, "OAuth linking: failed to commit", "error", err)
			writeErrorResponse(w, dto.Internal("commit"))
			return
		}

		slog.InfoContext(ctx, "OAuth: linked identity", "userID", linkingUserID, "provider", provider)
		http.Redirect(w, r, "/?oauth_linked=true", http.StatusFound)
		return
	}

	finishOAuthLogin(h.svc, h.cfg, w, r, provider, userInfo)
}

// issueState returns a fresh OAuth state that starts with prefix and sets its
// signed copy as the state cookie on w.
func (h *OAuthHandler) issueState(w http.ResponseWriter, prefix string) (string, error) {
	nonce, err := oauthserver.GenerateState()
	if err != nil {
		return "", err
	}
	state := prefix + nonce
	http.SetCookie(w, h.stateCookie(oauthserver.SignState(state, h.stateKey), oauthStateMaxAge))
	return state, nil
}

// verifyState returns the callback state after checking that it equals the
// state in the signed cookie the browser received when the flow started.
func (h *OAuthHandler) verifyState(r *http.Request) (string, error) {
	c, err := r.Cookie(oauthStateCookie)
	if err != nil {
		return "", dto.BadRequest("Missing OAuth state cookie")
	}
	state, ok := oauthserver.ValidateState(c.Value, h.stateKey)
	if !ok {
		return "", dto.BadRequest("Invalid OAuth state cookie")
	}
	if r.URL.Query().Get("state") != state {
		return "", dto.BadRequest("OAuth state mismatch")
	}
	return state, nil
}

// stateCookie returns the state cookie with the given value and max age.
//
// SameSite=Lax lets the browser send it on the provider's top-level redirect
// back to the callback.
func (h *OAuthHandler) stateCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // G124: Secure follows the base URL scheme so plain-HTTP deployments work.
		Name:     oauthStateCookie,
		Value:    value,
		Path:     oauthStateCookiePath,
		MaxAge:   maxAge,
		Secure:   strings.HasPrefix(h.cfg.BaseURL, "https://"),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// authCodeURL returns the provider authorization URL for state.
func authCodeURL(config *oauth2.Config, provider identity.OAuthProvider, state string) string {
	var opts []oauth2.AuthCodeOption
	if provider == identity.OAuthProviderGoogle {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	return config.AuthCodeURL(state, opts...)
}

// oauthUserInfo holds user info fetched from an OAuth provider.
type oauthUserInfo struct {
	ID        string
	Name      string
	AvatarURL string
	// VerifiedEmail returns an address the provider asserts the user controls,
	// or empty. Only a new or linking identity calls it, so a returning user
	// signs in by provider ID even when the email lookup fails.
	VerifiedEmail func() (string, error)
}

// knownEmail returns a VerifiedEmail for an address already in hand.
func knownEmail(email string) func() (string, error) {
	return func() (string, error) { return email, nil }
}

// finishOAuthLogin finds or creates a user from OAuth info, generates a JWT, and redirects.
//
// A provider identity that no account holds needs a verified email: it
// attaches to the account holding that email, or creates one. Without a
// verified email the login fails, and the user can link the provider from a
// signed-in session instead.
func finishOAuthLogin(svc *Services, cfg *Config, w http.ResponseWriter, r *http.Request, provider identity.OAuthProvider, info oauthUserInfo) {
	ctx := r.Context()

	// Try to find user by OAuth ID
	user, err := svc.User.GetByOAuth(provider, info.ID)
	if err != nil {
		email, err := info.VerifiedEmail()
		if err != nil {
			writeErrorResponse(w, err)
			return
		}
		if email == "" {
			slog.WarnContext(ctx, "OAuth: new identity without a verified email", "provider", provider)
			writeErrorResponse(w, dto.EmailNotVerified())
			return
		}
		user, err = svc.User.GetByEmail(email)
		if err != nil {
			// Create new user without organization (frontend will prompt for org creation)
			// Password is not used for OAuth users
			password, err := utils.GenerateToken(32)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to generate password for OAuth user", "err", err)
				writeErrorResponse(w, dto.Internal("password_generation"))
				return
			}
			user, err = svc.User.Create(email, password, info.Name)
			if err != nil {
				writeErrorResponse(w, dto.Internal("user_creation"))
				return
			}
		}

		if _, err := svc.User.Modify(user.ID, func(u *identity.User) error {
			u.EmailVerified = true
			u.OAuthIdentities = append(u.OAuthIdentities, identity.OAuthIdentity{
				Provider:   provider,
				ProviderID: info.ID,
				Email:      email,
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
	clientIP := reqctx.ClientIP(ctx)
	userAgent := r.Header.Get("User-Agent")
	countryCode := reqctx.CountryCode(r.Context())
	jwtToken, err := cfg.GenerateTokenWithSession(svc.Session, user, clientIP, userAgent, countryCode)
	if err != nil {
		slog.ErrorContext(ctx, "OAuth: failed to generate token", "err", err, "userID", user.ID)
		writeErrorResponse(w, dto.Internal("token_generation"))
		return
	}

	if err := svc.RootRepo.CommitDBChanges(ctx, GitAuthor(user), "OAuth login "+string(provider)); err != nil {
		slog.ErrorContext(ctx, "OAuth: failed to commit", "err", err, "userID", user.ID)
		writeErrorResponse(w, dto.Internal("commit"))
		return
	}

	slog.InfoContext(ctx, "OAuth: login successful, redirecting with token", "userID", user.ID, "email", user.Email)
	http.Redirect(w, r, "/?token="+url.QueryEscape(jwtToken), http.StatusFound)
}

// googleUserInfo is the Google OAuth2 v2 userinfo response.
type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// graphUser is the subset of the Microsoft Graph /me response that login
// trusts.
type graphUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// githubUser is the GitHub /user response.
type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// githubEmail is one entry of the GitHub /user/emails response.
type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// microsoftIDClaims holds the Microsoft ID token claims that mddb reads.
type microsoftIDClaims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tid"`
	Email    string `json:"email"`
	// EmailDomainOwnerVerified is the xms_edov optional claim. Microsoft
	// documents it as a boolean; any other value counts as unverified.
	EmailDomainOwnerVerified any `json:"xms_edov"`
}

// microsoftVerifiedEmail returns the email in the ID token of t when Microsoft
// asserts that its domain owner verified it, else an empty string.
//
// It skips the signature check because the token came straight from the token
// endpoint over TLS (OpenID Connect Core 1.0 section 3.1.3.7), and checks the
// audience, the expiry, and the v2.0 issuer of the token's tenant.
//
// The email and xms_edov claims are optional claims that the operator adds to
// the app registration; see SELF_HOSTING.md.
func microsoftVerifiedEmail(t *oauth2.Token, clientID string) (string, error) {
	raw, ok := t.Extra("id_token").(string)
	if !ok || raw == "" {
		return "", errors.New("no ID token")
	}
	var c microsoftIDClaims
	if _, _, err := jwt.NewParser().ParseUnverified(raw, &c); err != nil {
		return "", err
	}
	if c.TenantID == "" {
		return "", errors.New("ID token has no tid")
	}
	v := jwt.NewValidator(
		jwt.WithAudience(clientID),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer("https://login.microsoftonline.com/"+c.TenantID+"/v2.0"),
		// Tolerate clock skew with Microsoft on exp and nbf.
		jwt.WithLeeway(time.Minute),
	)
	if err := v.Validate(c); err != nil {
		return "", err
	}
	if c.EmailDomainOwnerVerified != true {
		return "", nil
	}
	return c.Email, nil
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

// fetchGitHubVerifiedEmail returns the user's primary email if GitHub
// verified it, else another verified email, else an empty string.
func fetchGitHubVerifiedEmail(client *http.Client) (email string, err error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub emails: status %d", resp.StatusCode)
	}
	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("GitHub emails: %w", err)
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	return "", nil
}
