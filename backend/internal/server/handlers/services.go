// Defines shared service dependencies for handlers.

package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
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
	JWTSecret    string
	BaseURL      string
	Version      string
	ServerQuotas identity.ServerQuotas
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
	mac := hmac.New(sha256.New, []byte(c.JWTSecret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyAssetSignature checks if the provided signature is valid.
func (c *Config) VerifyAssetSignature(path, sig string, expiry int64) bool {
	expected := c.generateSignature(path, expiry)
	return hmac.Equal([]byte(expected), []byte(sig))
}
