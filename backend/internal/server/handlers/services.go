// Defines shared service dependencies for handlers.

package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/syncsvc"
	"github.com/maruel/mddb/backend/internal/utils"
)

// Services holds all service dependencies for handlers.
type Services struct {
	FileStore        *content.FileStoreService
	Search           *content.SearchService
	User             *identity.UserService
	Organization     *identity.OrganizationService
	Workspace        *identity.WorkspaceService
	OrgInvitation    *identity.OrganizationInvitationService
	WSInvitation     *identity.WorkspaceInvitationService
	OrgMembership    *identity.OrganizationMembershipService
	WSMembership     *identity.WorkspaceMembershipService
	Session          *identity.SessionService
	EmailVerif       *identity.EmailVerificationService // may be nil
	Email            *email.Service                     // may be nil
	RootRepo         *git.RootRepo
	SyncService      *syncsvc.Service                  // may be nil
	Notification     *identity.NotificationService     // may be nil
	PushSubscription *identity.PushSubscriptionService // may be nil
	Broker           *sse.Broker
}

// PublishEvent publishes a workspace SSE event if the broker is configured.
func (s *Services) PublishEvent(wsID ksid.ID, eventType dto.EventType, nodeID, actorID ksid.ID) {
	if s.Broker == nil {
		return
	}
	s.Broker.Publish(wsID, dto.WorkspaceEvent{
		Type:     eventType,
		NodeID:   nodeID,
		ActorID:  actorID,
		Modified: storage.Now(),
	})
}

// PublishNodeEvent publishes a node change carrying the node's own revision, so
// subscribers can tell their own write apart from a concurrent edit.
func (s *Services) PublishNodeEvent(wsID ksid.ID, eventType dto.EventType, node *content.Node, actorID ksid.ID) {
	if s.Broker == nil {
		return
	}
	s.Broker.Publish(wsID, dto.WorkspaceEvent{
		Type:     eventType,
		NodeID:   node.ID,
		ActorID:  actorID,
		Modified: node.Modified,
	})
}

// PublishRecordEvent publishes a record-level workspace SSE event.
func (s *Services) PublishRecordEvent(wsID, nodeID, recordID, actorID ksid.ID) {
	if s.Broker == nil {
		return
	}
	s.Broker.Publish(wsID, dto.WorkspaceEvent{
		Type:     dto.EventRecordsChanged,
		NodeID:   nodeID,
		RecordID: recordID,
		ActorID:  actorID,
		Modified: storage.Now(),
	})
}

// Emit creates a notification and asynchronously dispatches it via enabled channels.
// It never blocks or returns errors — delivery failures are logged and ignored.
func (svc *Services) Emit(ctx context.Context, vapid *vapidKeys, userID ksid.ID, notifType identity.NotificationType, title, body, resourceID string, actorID ksid.ID) {
	if svc.Notification == nil {
		return
	}

	// Persist notification to the database.
	n, err := svc.Notification.Create(userID, notifType, title, body, resourceID, actorID)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create notification", "err", err, "user_id", userID, "type", notifType)
		return
	}

	// Determine channels from user preferences.
	user, err := svc.User.Get(userID)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get user for notification dispatch", "err", err, "user_id", userID)
		return
	}
	channels := user.Settings.NotificationPrefs.EffectiveChannels(notifType)

	// Web Push (async, fire-and-forget).
	if channels.Web && vapid != nil && svc.PushSubscription != nil {
		go func() {
			payload, _ := json.Marshal(map[string]string{
				"id":    n.ID.String(),
				"title": n.Title,
				"body":  n.Body,
				"type":  string(n.Type),
			})
			for sub := range svc.PushSubscription.ListByUser(userID) {
				resp, err := webpush.SendNotification(payload, &webpush.Subscription{
					Endpoint: sub.Endpoint,
					Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
				}, &webpush.Options{
					VAPIDPublicKey:  vapid.Public,
					VAPIDPrivateKey: vapid.Private,
					TTL:             86400,
				})
				if err != nil {
					slog.ErrorContext(ctx, "Web push send failed", "err", err, "endpoint", sub.Endpoint)
					continue
				}
				_ = resp.Body.Close()
				// 410 Gone means subscription is invalid — auto-delete.
				if resp.StatusCode == http.StatusGone {
					if err := svc.PushSubscription.Delete(sub.ID); err != nil {
						slog.ErrorContext(ctx, "Failed to delete expired push subscription", "err", err, "sub_id", sub.ID)
					}
				}
			}
		}()
	}
}

// ActiveWorkspaceID resolves the workspace the API treats as active for user:
// the most recent accessible entry from the user's LRU, otherwise the default
// workspace chosen from their memberships. It reuses populateActiveContext so
// non-HTTP surfaces such as the Go Mode MCP endpoint agree with the workspace
// the UI shows instead of failing when nothing was recorded yet. Returns the
// zero ID when the user has no accessible workspace.
func (s *Services) ActiveWorkspaceID(user *identity.User) ksid.ID {
	if user == nil {
		return 0
	}
	uwm, err := getUserWithMemberships(s.User, s.OrgMembership, s.WSMembership, s.Organization, s.Workspace, user.ID)
	if err != nil {
		return 0
	}
	var resp dto.UserResponse
	uwm.populateActiveContext(&resp)
	return resp.WorkspaceID
}

// BandwidthUpdater allows updating bandwidth limits at runtime.
type BandwidthUpdater interface {
	Update(maxBytesPerSecond int64)
}

// RateLimitsUpdater allows updating rate limits at runtime.
type RateLimitsUpdater interface {
	Update(authRate, writeRate, readAuthRate, readUnauthRate int)
}

// Config holds configuration values needed by handlers.
type Config struct {
	storage.ServerConfig
	BaseURL   string
	Version   string
	GoVersion string
	Revision  string
	Dirty     bool
}

// GenerateSignedAssetURL creates a signed rooted path for asset access.
func (c *Config) GenerateSignedAssetURL(wsID, nodeID ksid.ID, name string) string {
	expiry := time.Now().Add(AssetURLExpiry).Unix()
	path := fmt.Sprintf("%s/%s/%s", wsID, nodeID, name)
	sig := c.generateSignature(path, expiry)
	return fmt.Sprintf("/assets/%s?sig=%s&exp=%d", path, sig, expiry)
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

// AssetURLExpiry is the default duration for which signed asset URLs are valid.
const AssetURLExpiry = 1 * time.Hour

const tokenExpiration = 31 * 24 * time.Hour

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
func (c *Config) GenerateTokenWithSession(sessionSvc *identity.SessionService, user *identity.User, clientIP, userAgent, countryCode string) (string, error) {
	expiresAt := time.Now().Add(tokenExpiration)

	// Pre-generate session ID so we can include it in the JWT
	sessionID := ksid.NewID()

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
	if _, err := sessionSvc.CreateWithID(sessionID, user.ID, utils.HashToken(tokenString), deviceInfo, clientIP, countryCode, storage.ToTime(expiresAt), c.Quotas.MaxSessionsPerUser); err != nil {
		if errors.Is(err, identity.ErrSessionQuotaExceeded) {
			return "", dto.QuotaExceeded("sessions per user", c.Quotas.MaxSessionsPerUser)
		}
		return "", err
	}

	return tokenString, nil
}

// VAPIDKeys returns a VAPIDKeys pair from the Config, or nil if unconfigured.
func (c *Config) VAPIDKeys() *vapidKeys {
	if c.VAPID.PublicKey == "" || c.VAPID.PrivateKey == "" {
		return nil
	}
	return &vapidKeys{Public: c.VAPID.PublicKey, Private: c.VAPID.PrivateKey}
}
