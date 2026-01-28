// Defines shared service dependencies for handlers.

package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
)

// Services holds all service dependencies for handlers.
type Services struct {
	FileStore     *content.FileStoreService
	Search        *content.SearchService
	User          *identity.UserService
	Organization  *identity.OrganizationService
	Workspace     *identity.WorkspaceService
	OrgInvitation *identity.OrganizationInvitationService
	WSInvitation  *identity.WorkspaceInvitationService
	OrgMembership *identity.OrganizationMembershipService
	WSMembership  *identity.WorkspaceMembershipService
	Session       *identity.SessionService
	EmailVerif    *identity.EmailVerificationService // may be nil
	Email         *email.Service                     // may be nil
}

// Config holds configuration values needed by handlers.
type Config struct {
	storage.ServerConfig
	BaseURL string
	Version string
}

// AssetURLExpiry is the default duration for which signed asset URLs are valid.
const AssetURLExpiry = 1 * time.Hour

// GenerateSignedAssetURL creates a signed URL for asset access.
func (c *Config) GenerateSignedAssetURL(wsID, nodeID jsonldb.ID, name string) string {
	expiry := time.Now().Add(AssetURLExpiry).Unix()
	path := fmt.Sprintf("%s/%s/%s", wsID, nodeID, name)
	sig := c.generateSignature(path, expiry)
	return fmt.Sprintf("%s/assets/%s?sig=%s&exp=%d", c.BaseURL, path, sig, expiry)
}

// generateSignature creates an HMAC-SHA256 signature for asset access.
func (c *Config) generateSignature(path string, expiry int64) string {
	data := fmt.Sprintf("%s:%d", path, expiry)
	mac := hmac.New(sha256.New, c.JWTSecret)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyAssetSignature checks if the provided signature is valid.
func (c *Config) VerifyAssetSignature(path, sig string, expiry int64) bool {
	expected := c.generateSignature(path, expiry)
	return hmac.Equal([]byte(expected), []byte(sig))
}

const tokenExpiration = 24 * time.Hour

// GenerateToken generates a JWT token for the given user (without session tracking).
func (c *Config) GenerateToken(user *identity.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(tokenExpiration).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(c.JWTSecret)
}

// GenerateTokenWithSession creates a session and generates a JWT token with session ID.
func (c *Config) GenerateTokenWithSession(sessionSvc *identity.SessionService, user *identity.User, clientIP, userAgent string) (string, error) {
	expiresAt := time.Now().Add(tokenExpiration)

	// Pre-generate session ID so we can include it in the JWT
	sessionID := jsonldb.NewID()

	// Build claims with session ID
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"sid":   sessionID.String(),
		"exp":   expiresAt.Unix(),
		"iat":   time.Now().Unix(),
	}

	// Generate the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(c.JWTSecret)
	if err != nil {
		return "", err
	}

	// Create session with the pre-generated ID and token hash
	deviceInfo := userAgent
	if len(deviceInfo) > 200 {
		deviceInfo = deviceInfo[:200]
	}
	if _, err := sessionSvc.CreateWithID(sessionID, user.ID, utils.HashToken(tokenString), deviceInfo, clientIP, storage.ToTime(expiresAt), c.Quotas.MaxSessionsPerUser); err != nil {
		if errors.Is(err, identity.ErrSessionQuotaExceeded) {
			return "", dto.QuotaExceeded("sessions per user", c.Quotas.MaxSessionsPerUser)
		}
		return "", err
	}

	return tokenString, nil
}
