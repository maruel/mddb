// Handles OAuth authentication, independent primary-address proof, and invitation-bound identity linking.

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

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gomodedev/gomode/httpserver"
	"github.com/gomodedev/gomode/oauth/oauthserver"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
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
	loginStatePrefix     = "login:"
)

// OAuthHandler handles OAuth2 authentication for multiple providers.
type OAuthHandler struct {
	svc       *Services
	cfg       *Config
	stateKey  []byte
	providers map[identityv1.OAuthProvider]*oauth2.Config
}

// NewOAuthHandler creates a new OAuth handler.
func NewOAuthHandler(svc *Services, cfg *Config) *OAuthHandler {
	// Derive a dedicated state key so a state signature never doubles as a
	// JWT or signed asset URL signature, which use the raw JWT secret.
	mac := hmac.New(sha256.New, cfg.Snapshot().JWTSecret)
	mac.Write([]byte("mddb oauth state v1"))
	return &OAuthHandler{
		svc:       svc,
		cfg:       cfg,
		stateKey:  mac.Sum(nil),
		providers: make(map[identityv1.OAuthProvider]*oauth2.Config),
	}
}

// AddProvider adds an OAuth2 provider configuration.
func (h *OAuthHandler) AddProvider(name identityv1.OAuthProvider, clientID, clientSecret, redirectURL string) {
	var endpoint oauth2.Endpoint
	var scopes []string

	switch name {
	case identityv1.OAuthProviderGoogle:
		endpoint = google.Endpoint
		scopes = []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		}
	case identityv1.OAuthProviderMicrosoft:
		endpoint = microsoft.AzureADEndpoint("common")
		scopes = []string{"openid", "profile", "email", "User.Read"}
	case identityv1.OAuthProviderGitHub:
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
	next, err := invitationReturnPath(req.Next)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}
	provider := identityv1.OAuthProvider(req.Provider)
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

	state, err := h.issueState(w, linkingStatePrefix+user.ID.String()+":"+string(provider)+":"+base64.RawURLEncoding.EncodeToString([]byte(next))+":")
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
func (h *OAuthHandler) UnlinkOAuth(_ context.Context, user *identityv1.User, req *dto.UnlinkOAuthAccountRequest) (*dto.OkResponse, error) {
	provider := identityv1.OAuthProvider(req.Provider)

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
	if _, err := h.svc.User.Modify(user.ID, func(u *identityv1.User) error {
		newIdentities := make([]identityv1.OAuthIdentity, 0, len(u.OAuthIdentities)-1)
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

	return &dto.OkResponse{Ok: true}, nil
}

// parseLinkingState parses a verified linking state and returns the user ID
// and provider. Returns zero ID if the state is not a linking state.
func parseLinkingState(state string) (userID ksid.ID, provider identityv1.OAuthProvider) {
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
	return userID, identityv1.OAuthProvider(parts[1])
}

// LoginRedirect redirects the user to the OAuth provider.
//
// The browser navigates here, so a failure redirects to the login page with
// the error code; see redirectOAuthError.
func (h *OAuthHandler) LoginRedirect(w http.ResponseWriter, r *http.Request) {
	provider := identityv1.OAuthProvider(r.PathValue("provider"))
	config, ok := h.providers[provider]
	if !ok {
		redirectOAuthError(w, r, loginPath, dto.InvalidProvider())
		return
	}

	next, err := loginReturnPath(r.URL.Query().Get("next"))
	if err != nil {
		redirectOAuthError(w, r, loginPath, err)
		return
	}
	state, err := h.issueState(w, loginStatePrefix+base64.RawURLEncoding.EncodeToString([]byte(next))+":")
	if err != nil {
		redirectOAuthError(w, r, loginErrorPath(next), dto.InternalWithError("state_generation", err))
		return
	}
	http.Redirect(w, r, authCodeURL(config, provider, state), http.StatusTemporaryRedirect) //nolint:gosec // G710: the authorization URL of a configured provider.
}

// Callback handles the OAuth provider callback.
//
// The browser navigates here from the provider, so every outcome is a
// redirect: a login returns to its destination and a link to the profile
// settings. A failure carries its dto.ErrorCode in oauthErrorParam to the page
// that started the flow. Declining at the provider returns there without one.
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// The state cookie is single use: clear it whatever the outcome.
	http.SetCookie(w, h.stateCookie("", -1))
	provider := identityv1.OAuthProvider(r.PathValue("provider"))
	config, ok := h.providers[provider]
	if !ok {
		redirectOAuthError(w, r, loginPath, dto.InvalidProvider())
		return
	}
	state, err := h.verifyState(r)
	if err != nil {
		slog.WarnContext(r.Context(), "OAuth: rejected callback state", "err", err, "provider", provider)
		redirectOAuthError(w, r, loginPath, err)
		return
	}
	linkingUserID, linkingProvider := parseLinkingState(state)
	start, next := profileSettingsPath, "/"
	if linkingUserID.IsZero() {
		if next, err = loginStateReturnPath(state); err != nil {
			redirectOAuthError(w, r, loginPath, err)
			return
		}
		start = loginErrorPath(next)
	}
	if !linkingUserID.IsZero() {
		if linkingProvider != provider {
			redirectOAuthError(w, r, start, dto.BadRequest("Provider mismatch"))
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(state, linkingStatePrefix), ":", 4)
		if len(parts) != 4 {
			redirectOAuthError(w, r, start, dto.BadRequest("Invalid linking destination"))
			return
		}
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(parts[2])
		if decodeErr != nil {
			redirectOAuthError(w, r, start, dto.BadRequest("Invalid linking destination"))
			return
		}
		next, err = invitationReturnPath(string(decoded))
		if err != nil {
			redirectOAuthError(w, r, start, err)
			return
		}
		if next != "" {
			start = next
		}
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		// RFC 6749 section 4.1.2.1: the user declined, or the provider refused.
		if e == "access_denied" {
			http.Redirect(w, r, start, http.StatusSeeOther)
			return
		}
		redirectOAuthError(w, r, start, dto.OAuthError("authorization: "+e))
		return
	}
	code := q.Get("code")
	if code == "" {
		redirectOAuthError(w, r, start, dto.OAuthError("authorization: missing code"))
		return
	}
	ctx := r.Context()
	token, err := config.Exchange(ctx, code)
	if err != nil {
		redirectOAuthError(w, r, start, dto.OAuthError("token_exchange: "+err.Error()))
		return
	}
	client := config.Client(ctx, token)
	userInfo, err := fetchUserInfo(ctx, provider, client, token, config.ClientID)
	if err != nil {
		redirectOAuthError(w, r, start, err)
		return
	}
	switch {
	case linkingUserID.IsZero():
		err = finishOAuthLogin(h.svc, h.cfg, w, r, provider, userInfo)
	case linkingProvider != provider:
		err = dto.BadRequest("Provider mismatch")
	default:
		err = h.link(ctx, linkingUserID, provider, userInfo)
		if next == "" {
			next = profileSettingsPath + "?oauth_linked=true"
		}
	}
	if err != nil {
		redirectOAuthError(w, r, start, err)
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// fetchUserInfo reads the signed-in account from the provider's API.
func fetchUserInfo(ctx context.Context, provider identityv1.OAuthProvider, client *http.Client, token *oauth2.Token, clientID string) (oauthUserInfo, error) {
	var info oauthUserInfo
	switch provider {
	case identityv1.OAuthProviderGoogle:
		var u googleUserInfo
		if err := getProviderJSON(ctx, client, "https://www.googleapis.com/oauth2/v2/userinfo", &u); err != nil {
			return info, err
		}
		info.ID = u.ID
		verified := ""
		if u.VerifiedEmail {
			verified = u.Email
		}
		info.VerifiedEmail = knownEmail(verified)
		info.Name = u.Name
		info.AvatarURL = u.Picture
	case identityv1.OAuthProviderMicrosoft:
		// A tenant admin sets mail to any address, and Graph does not report
		// whether anyone verified mail or userPrincipalName; the email comes
		// from the ID token instead.
		var u graphUser
		if err := getProviderJSON(ctx, client, "https://graph.microsoft.com/v1.0/me", &u); err != nil {
			return info, err
		}
		info.ID = u.ID
		info.Name = u.DisplayName
		info.VerifiedEmail = func() (string, error) {
			email, err := microsoftVerifiedEmail(token, clientID)
			if err != nil {
				return "", dto.OAuthError("Microsoft ID token: " + err.Error())
			}
			return email, nil
		}
		info.AvatarURL = fetchMicrosoftPhoto(ctx, client)
	case identityv1.OAuthProviderGitHub:
		// The profile email is the user's chosen public address, which GitHub
		// does not document as verified; read the email from /user/emails.
		var u githubUser
		if err := getProviderJSON(ctx, client, "https://api.github.com/user", &u); err != nil {
			return info, err
		}
		info.ID = strconv.FormatInt(u.ID, 10)
		info.Name = u.Name
		if info.Name == "" {
			info.Name = u.Login
		}
		info.AvatarURL = u.AvatarURL
		info.VerifiedEmail = func() (string, error) {
			email, err := fetchGitHubVerifiedEmail(client)
			if err != nil {
				return "", dto.OAuthError("GitHub emails: " + err.Error())
			}
			return email, nil
		}
	}
	return info, nil
}

// getProviderJSON decodes the JSON response of a provider API GET into v.
func getProviderJSON(ctx context.Context, client *http.Client, apiURL string, v any) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return dto.OAuthError("user_info: " + err.Error())
	}
	resp, err := client.Do(req)
	if err != nil {
		return dto.OAuthError("user_info: " + err.Error())
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()
	if resp.StatusCode != http.StatusOK {
		return dto.OAuthError("user_info: status " + strconv.Itoa(resp.StatusCode))
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return dto.OAuthError("decode: " + err.Error())
	}
	return nil
}

// link attaches the provider identity to the signed-in user who started the
// link. Linking an already linked provider succeeds.
func (h *OAuthHandler) link(ctx context.Context, userID ksid.ID, provider identityv1.OAuthProvider, info oauthUserInfo) error {
	if existing, _ := h.svc.User.GetByOAuth(provider, info.ID); existing != nil && existing.ID != userID {
		slog.WarnContext(ctx, "OAuth linking: identity already claimed", "oauthID", info.ID, "existingUser", existing.ID)
		return dto.NewAPIError(http.StatusConflict, dto.ErrorCodeConflict, "identity linked to another account")
	}
	user, err := h.svc.User.Get(userID)
	if err != nil {
		return dto.InternalWithError("linking user", err)
	}
	for _, ident := range user.OAuthIdentities {
		if ident.Provider == provider {
			return nil
		}
	}
	// The signed-in user proves the link; the email is only a label, so a
	// failed lookup stores none instead of blocking the link.
	email, err := info.VerifiedEmail()
	if err != nil {
		email = ""
	}
	if _, err := h.svc.User.Modify(userID, func(u *identityv1.User) error {
		if email != "" && u.Email == email {
			u.EmailVerified = true
		}
		u.OAuthIdentities = append(u.OAuthIdentities, identityv1.OAuthIdentity{
			Provider:   provider,
			ProviderID: info.ID,
			Email:      email,
			AvatarURL:  info.AvatarURL,
			LastLogin:  storagev1.Now(),
		})
		return nil
	}); err != nil {
		return dto.InternalWithError("oauth_link", err)
	}
	if err := h.svc.RootRepo.CommitDBChanges(ctx, GitAuthor(user), "OAuth link "+string(provider)); err != nil {
		slog.ErrorContext(ctx, "OAuth linking: failed to commit", "err", err)
	}
	slog.InfoContext(ctx, "OAuth: linked identity", "userID", userID, "provider", provider)
	return nil
}

const (
	// oauthErrorParam carries the dto.ErrorCode of a failed browser OAuth flow
	// to the page that displays it.
	oauthErrorParam     = "oauth_error"
	loginPath           = "/login"
	profileSettingsPath = "/settings/user"
)

// redirectOAuthError ends a browser OAuth navigation at dest, a rooted path,
// with the code of err in oauthErrorParam. A browser navigation renders a JSON
// error body as raw text, so the app explains the failure instead. The code
// defaults to dto.ErrorCodeInternal; err itself only reaches the log.
func redirectOAuthError(w http.ResponseWriter, r *http.Request, dest string, err error) {
	code := dto.ErrorCodeInternal
	if e, ok := errors.AsType[dto.ErrorWithStatus](err); ok {
		code = e.Code()
	}
	slog.WarnContext(r.Context(), "OAuth: sign-in failed", "err", err, "code", code)
	u, perr := url.Parse(dest)
	if perr != nil {
		u = &url.URL{Path: loginPath}
	}
	q := u.Query()
	q.Set(oauthErrorParam, string(code))
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther) //nolint:gosec // G710: dest is a constant path or a loginReturnPath result.
}

// loginErrorPath returns the page that reports a failed login to next. The
// root only redirects, so the login page reports it instead.
func loginErrorPath(next string) string {
	if next == "/" {
		return loginPath
	}
	return next
}

// loginStateReturnPath returns the validated destination in a verified login
// state.
func loginStateReturnPath(state string) (string, error) {
	encoded, _, ok := strings.Cut(strings.TrimPrefix(state, loginStatePrefix), ":")
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if !strings.HasPrefix(state, loginStatePrefix) || !ok || err != nil {
		return "", dto.BadRequest("Invalid login destination")
	}
	return loginReturnPath(string(decoded))
}

// invitationReturnPath permits only the supported invitation landing routes.
// The token remains opaque; neither linking nor returning to the page claims it.
func invitationReturnPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if _, err := loginReturnPath(path); err != nil {
		return "", err
	}
	u, err := url.Parse(path)
	if err != nil || (u.Path != "/accept-invitation/org" && u.Path != "/accept-invitation/ws") || u.EscapedPath() != u.Path {
		return "", dto.BadRequest("Invalid invitation destination")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 1 || len(q["token"]) != 1 || q.Get("token") == "" {
		return "", dto.BadRequest("Invalid invitation destination")
	}
	return path, nil
}

// loginReturnPath accepts only rooted same-origin paths. Check decoded forms too:
// browsers and intermediaries can normalize escaped separators before navigation.
func loginReturnPath(path string) (string, error) {
	if path == "" {
		return "/", nil
	}
	u, err := url.Parse(path)
	if err != nil || len(path) > 2048 || u.Scheme != "" || u.Host != "" || u.Fragment != "" {
		return "", dto.BadRequest("Invalid login destination")
	}
	for decoded := path; ; {
		if !strings.HasPrefix(decoded, "/") || strings.HasPrefix(decoded, "//") || strings.ContainsAny(decoded, "\\\r\n\t") {
			return "", dto.BadRequest("Invalid login destination")
		}
		for _, c := range decoded {
			if c < 32 || c == 127 {
				return "", dto.BadRequest("Invalid login destination")
			}
		}
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return "", dto.BadRequest("Invalid login destination")
		}
		if next == decoded {
			return path, nil
		}
		decoded = next
	}
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
//
// Every failure reports dto.ErrorCodeExpired: the user usually took longer
// than oauthStateMaxAge or finished in another browser, and starts again.
func (h *OAuthHandler) verifyState(r *http.Request) (string, error) {
	c, err := r.Cookie(oauthStateCookie)
	if err != nil {
		return "", oauthStateError("missing OAuth state cookie")
	}
	state, ok := oauthserver.ValidateState(c.Value, h.stateKey)
	if !ok {
		return "", oauthStateError("invalid OAuth state cookie")
	}
	if r.URL.Query().Get("state") != state {
		return "", oauthStateError("OAuth state mismatch")
	}
	return state, nil
}

func oauthStateError(message string) error {
	return dto.NewAPIError(http.StatusBadRequest, dto.ErrorCodeExpired, message)
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
func authCodeURL(config *oauth2.Config, provider identityv1.OAuthProvider, state string) string {
	var opts []oauth2.AuthCodeOption
	if provider == identityv1.OAuthProviderGoogle {
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

// finishOAuthLogin finds or creates a user from OAuth info and sets the
// session cookie. The caller redirects.
//
// A provider identity that no account holds needs a verified email: it
// attaches only to an account that already verified that email, or creates one. Without a
// verified email the login fails, and the user can link the provider from a
// signed-in session instead.
func finishOAuthLogin(svc *Services, cfg *Config, w http.ResponseWriter, r *http.Request, provider identityv1.OAuthProvider, info oauthUserInfo) error {
	ctx := r.Context()
	// Try to find user by OAuth ID
	user, err := svc.User.GetByOAuth(provider, info.ID)
	if err != nil {
		email, err := info.VerifiedEmail()
		if err != nil {
			return err
		}
		if email == "" {
			return dto.EmailNotVerified()
		}
		user, err = svc.User.GetByEmail(email)
		// A password registrant has not proved the claimed address. Do not merge
		// a verified provider into their account, content, or active sessions.
		// Resolve the collision by verifying that account's email, or moving its
		// claimed address before retrying provider login.
		if err == nil && !user.EmailVerified {
			return dto.NewAPIError(http.StatusConflict, dto.ErrorCodeEmailInUse,
				"An account already claims this email. Verify its email before linking, or change its email and retry provider sign-in.")
		}
		created := err != nil
		// TODO: Enforce Quotas.MaxUsers here. AuthHandler.Register (auth.go)
		// rejects a password registration once User.Count() reaches MaxUsers
		// (50 by default), but this provider path creates the account without
		// that check, so Google, GitHub, and Microsoft sign-in can exceed the
		// cap. Move the check into one place that every account creation uses,
		// such as the user service, and return dto.QuotaExceeded("users", ...)
		// here too. docs.mddb.app's authentication guide states the cap applies
		// to password registration only; update it with the fix.
		if err != nil {
			// Create new user without organization (frontend will prompt for org creation)
			// Password is not used for OAuth users
			password, err := utils.GenerateToken(32)
			if err != nil {
				return dto.InternalWithError("password_generation", err)
			}
			user, err = svc.User.Create(email, password, info.Name)
			if err != nil {
				return dto.InternalWithError("user_creation", err)
			}
		}

		if _, err := svc.User.Modify(user.ID, func(u *identityv1.User) error {
			// Bind proof and existing-account eligibility at the atomic mutation:
			// a concurrent email change must not verify a different address.
			if u.Email != email || (!created && !u.EmailVerified) {
				return dto.EmailNotVerified()
			}
			u.EmailVerified = true
			u.OAuthIdentities = append(u.OAuthIdentities, identityv1.OAuthIdentity{
				Provider:   provider,
				ProviderID: info.ID,
				Email:      email,
				AvatarURL:  info.AvatarURL,
				LastLogin:  storagev1.Now(),
			})
			return nil
		}); err != nil {
			return dto.InternalWithError("oauth_link", err)
		}
	} else {
		// A known identity authenticates this account without email-based linking.
		// Refresh proof independently; cached identity email is never evidence.
		email, err := info.VerifiedEmail()
		if err != nil {
			email = ""
		}
		if _, err := svc.User.Modify(user.ID, func(u *identityv1.User) error {
			for i := range u.OAuthIdentities {
				if u.OAuthIdentities[i].Provider == provider && u.OAuthIdentities[i].ProviderID == info.ID {
					if email != "" && u.Email == email {
						u.EmailVerified = true
					}
					u.OAuthIdentities[i].AvatarURL = info.AvatarURL
					u.OAuthIdentities[i].LastLogin = storagev1.Now()
					break
				}
			}
			return nil
		}); err != nil {
			slog.WarnContext(ctx, "OAuth: failed to update identity", "err", err)
		}
	}

	// Generate JWT token with session tracking
	clientIP := httpserver.ClientIP(ctx)
	userAgent := r.Header.Get("User-Agent")
	countryCode := httpserver.Origin(r.Context())
	jwtToken, sid, err := cfg.IssueToken(svc.Session, user, clientIP, userAgent, countryCode)
	if err != nil {
		if sid != 0 {
			err = errors.Join(err, svc.Session.Revoke(sid))
		}
		return err
	}

	if err := svc.RootRepo.CommitDBChanges(ctx, GitAuthor(user), "OAuth login "+string(provider)); err != nil {
		slog.ErrorContext(ctx, "OAuth: failed to commit", "err", err, "userID", user.ID)
	}

	if err := cfg.WriteSessionCookie(w, jwtToken); err != nil {
		return dto.InternalWithError("cookie", err)
	}
	slog.InfoContext(ctx, "OAuth: login successful", "userID", user.ID)
	return nil
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
// the app registration; see https://docs.mddb.app/guide/authentication#microsoft.
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
