// External voice gateway discovery and narrowly scoped, authenticated authorization.

package server

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/oauth"
	"github.com/maruel/gomode/oauth/oauthserver"
	"github.com/maruel/gomode/voicegateway"
	voicev1 "github.com/maruel/gomode/voicegateway/api/v1"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
)

const voiceTokenEndpoint = "/api/v1/gomode/voice/token" //nolint:gosec // Route, not a credential.

// VoiceTokenMode selects the external gateway authorization format.
type VoiceTokenMode string

// Validate rejects unknown token modes.
func (m VoiceTokenMode) Validate() error {
	switch m {
	case VoiceTokenModeOAuth, VoiceTokenModeScoped:
		return nil
	default:
		return errors.New("voice-gateway.token_mode must be scoped or oauth")
	}
}

const (
	// VoiceTokenModeOAuth issues narrow OAuth access tokens.
	VoiceTokenModeOAuth VoiceTokenMode = "oauth"
	// VoiceTokenModeScoped issues scoped Ed25519 tokens.
	VoiceTokenModeScoped VoiceTokenMode = "scoped"
)

// ExternalVoiceGateway owns an external gateway's immutable signing configuration.
type ExternalVoiceGateway struct {
	URL        string
	Issuer     string
	InstanceID string

	key    ed25519.PrivateKey
	tokens *oauthserver.AccessTokenService
}

// NewExternalVoiceGateway validates origins and parses the configured signing key.
func NewExternalVoiceGateway(gateway, issuer, instance string, mode VoiceTokenMode, keyPEM []byte) (*ExternalVoiceGateway, error) {
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	if err := validateVoiceOrigins(gateway, issuer, instance); err != nil {
		return nil, err
	}
	c := &ExternalVoiceGateway{URL: strings.TrimSuffix(gateway, "/"), Issuer: strings.TrimSuffix(issuer, "/"), InstanceID: instance}
	if mode == VoiceTokenModeOAuth {
		digest := sha256.Sum256(keyPEM)
		kid := hex.EncodeToString(digest[:16])
		var err error
		c.tokens, err = oauthserver.NewAccessTokenService(keyPEM, kid, 5*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("voice OAuth signing key: %w", err)
		}
		return c, nil
	}
	block, rest := pem.Decode(keyPEM)
	if block == nil || block.Type != "PRIVATE KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("voice signing key must be one PKCS#8 PEM private key")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse voice signing key: %w", err)
	}
	var ok bool
	c.key, ok = k.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("voice scoped signing key must be Ed25519")
	}
	return c, nil
}

// SigningPublicKey returns the scoped public key for the gateway's trusted issuer.
func (c *ExternalVoiceGateway) SigningPublicKey() (string, error) {
	if c.tokens != nil {
		return "", nil
	}
	return gomode.EncodeServiceSigningPublicKey(c.key.Public().(ed25519.PublicKey))
}

func (c *ExternalVoiceGateway) serveToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	u := reqctx.User(r.Context())
	if u == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	var token string
	var err error
	if c.tokens != nil {
		token, err = c.tokens.IssueAccessTokenTTL(c.Issuer, oauth.User{ID: u.ID.String()}, gomode.ScopedTokenAudience, voicegateway.DefaultVoiceScope, "", "", 5*time.Minute)
	} else {
		token, err = gomode.IssueServiceScopedToken(&gomode.ScopedTokenClaims{
			ServiceKind: "mddb", ServiceInstanceID: c.InstanceID, BackendOrigin: c.Issuer,
			Subject: u.ID.String(), Capabilities: []string{voicegateway.DefaultVoiceScope}, Audience: gomode.ScopedTokenAudience, Expiry: time.Now().Add(5 * time.Minute),
		}, c.key)
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "issue voice token", "err", err)
		http.Error(w, "Voice token unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(voicev1.ServiceAuthorization{Kind: "mddb", InstanceID: c.InstanceID, BaseURL: c.Issuer, Token: token}); err != nil {
		slog.WarnContext(r.Context(), "write voice token", "err", err)
	}
}

func (c *ExternalVoiceGateway) serveMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(oauth.AuthorizationServerMetadata{Issuer: c.Issuer, JWKSURI: c.Issuer + "/oauth/jwks", ScopesSupported: []string{voicegateway.DefaultVoiceScope}}); err != nil {
		slog.WarnContext(r.Context(), "write voice issuer metadata", "err", err)
	}
}

func (c *ExternalVoiceGateway) serveJWKS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(oauth.JWKSet{Keys: c.tokens.JWK()}); err != nil {
		slog.WarnContext(r.Context(), "write voice signing keys", "err", err)
	}
}

// validateVoiceOrigins requires secure, fixed origins and a stable instance identity.
func validateVoiceOrigins(gateway, issuer, instance string) error {
	if strings.TrimSpace(instance) == "" {
		return errors.New("voice-gateway.instance_id is required")
	}
	for name, raw := range map[string]string{"voice-gateway.url": gateway, "server.base_url": issuer} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
			return fmt.Errorf("%s must be an absolute origin without credentials, path, query or fragment", name)
		}
		loopback := u.Hostname() == "localhost"
		if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
			loopback = ip.IsLoopback()
		}
		if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
			return fmt.Errorf("%s requires HTTPS except on loopback", name)
		}
	}
	return nil
}
