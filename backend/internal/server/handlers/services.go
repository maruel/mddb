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
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/gomode/devicelogin"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
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

	userCreationLocks     keyedMutex
	rootPageCreationLocks keyedMutex

	background sync.WaitGroup

	backgroundMu     sync.Mutex
	backgroundCancel context.CancelFunc
	backgroundCtx    context.Context
	backgroundDone   chan struct{}
	// The count closes backgroundDone without starting another goroutine to wait.
	backgroundCount  int
	backgroundClosed bool
}

// StartBackground starts work owned by the services. It returns false after shutdown.
func (s *Services) StartBackground(ctx context.Context, timeout time.Duration, fn func(context.Context)) bool {
	s.backgroundMu.Lock()
	defer s.backgroundMu.Unlock()
	if s.backgroundClosed {
		return false
	}
	if s.backgroundCtx == nil {
		s.backgroundCtx, s.backgroundCancel = context.WithCancel(context.Background())
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	stop := context.AfterFunc(s.backgroundCtx, cancel) //nolint:contextcheck // Shutdown also cancels the task's request-derived context.
	s.backgroundCount++
	s.background.Go(func() {
		defer func() {
			stop()
			cancel()
			s.backgroundMu.Lock()
			s.backgroundCount--
			if s.backgroundClosed && s.backgroundCount == 0 {
				close(s.backgroundDone)
			}
			s.backgroundMu.Unlock()
		}()
		fn(ctx)
	})
	return true
}

// StopBackground cancels owned work and waits until it completes or ctx expires.
// No new work is accepted after this call starts.
func (s *Services) StopBackground(ctx context.Context) error {
	s.backgroundMu.Lock()
	if !s.backgroundClosed {
		s.backgroundClosed = true
		if s.backgroundCancel != nil {
			s.backgroundCancel()
		}
		if s.backgroundCount != 0 {
			s.backgroundDone = make(chan struct{})
		}
	}
	done := s.backgroundDone
	s.backgroundMu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		s.background.Wait()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishEvent publishes a workspace SSE event if the broker is configured.
func (s *Services) PublishEvent(ctx context.Context, wsID ksid.ID, eventType dto.EventType, nodeID, actorID ksid.ID) {
	if s.Broker == nil {
		return
	}
	s.Broker.Publish(wsID, dto.WorkspaceEvent{
		Type:      eventType,
		NodeID:    nodeID,
		ActorID:   actorID,
		Modified:  storage.Now(),
		RequestID: reqctx.RequestID(ctx),
	})
}

// PublishNodeEvent publishes a node change carrying the node's own revision, so
// subscribers can tell their own write apart from a concurrent edit.
func (s *Services) PublishNodeEvent(ctx context.Context, wsID ksid.ID, eventType dto.EventType, node *content.Node, actorID ksid.ID) {
	if s.Broker == nil {
		return
	}
	s.Broker.Publish(wsID, dto.WorkspaceEvent{
		Type:      eventType,
		NodeID:    node.ID,
		ActorID:   actorID,
		Modified:  node.Modified,
		RequestID: reqctx.RequestID(ctx),
	})
}

// PublishRecordEvent publishes a record-level workspace SSE event.
func (s *Services) PublishRecordEvent(ctx context.Context, wsID, nodeID, recordID, actorID ksid.ID) {
	if s.Broker == nil {
		return
	}
	s.Broker.Publish(wsID, dto.WorkspaceEvent{
		Type:      dto.EventRecordsChanged,
		NodeID:    nodeID,
		RecordID:  recordID,
		ActorID:   actorID,
		Modified:  storage.Now(),
		RequestID: reqctx.RequestID(ctx),
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
		if !svc.StartBackground(context.WithoutCancel(ctx), 30*time.Second, func(ctx context.Context) {
			payload, _ := json.Marshal(map[string]string{
				"id":    n.ID.String(),
				"title": n.Title,
				"body":  n.Body,
				"type":  string(n.Type),
			})
			for sub := range svc.PushSubscription.ListByUser(userID) {
				resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
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
		}) {
			slog.WarnContext(ctx, "Web push skipped during shutdown", "user_id", userID)
		}
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

// EffectiveWorkspaceRole applies current workspace membership and the organization owner/admin override.
func (svc *Services) EffectiveWorkspaceRole(user *identity.User, wsID ksid.ID) (role identity.WorkspaceRole, errMsg string, statusCode int) {
	ws, err := svc.Workspace.Get(wsID)
	if err != nil {
		return "", "Workspace not found", http.StatusNotFound
	}

	orgMem, err := svc.OrgMembership.Get(user.ID, ws.OrganizationID)
	if err != nil {
		return "", "Forbidden: not a member of this organization", http.StatusForbidden
	}
	if orgMem.Role == identity.OrgRoleOwner || orgMem.Role == identity.OrgRoleAdmin {
		return identity.WSRoleAdmin, "", 0
	}

	wsMem, err := svc.WSMembership.Get(user.ID, wsID)
	if err != nil {
		return "", "Forbidden: not a member of this workspace", http.StatusForbidden
	}
	return wsMem.Role, "", 0
}

func (s *Services) rollbackUnpublishedWorkspace(wsID ksid.ID) error {
	return errors.Join(s.Workspace.Delete(wsID), s.FileStore.DiscardWorkspace(wsID))
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

// GenerateSignedAssetURL creates a parent-session-bound rooted asset link.
// Without a validated host session it returns no link; user-only contexts cannot delegate access.
func (c *Config) GenerateSignedAssetURL(ctx context.Context, wsID, nodeID ksid.ID, name string) string {
	session := reqctx.Session(ctx)
	if session == nil {
		return ""
	}
	sid := session.ID.String()
	expiry := min(time.Now().Add(AssetURLExpiry).Unix(), session.ExpiresAt.AsTime().Unix())
	path := fmt.Sprintf("%s/%s/%s", wsID, nodeID, name)
	sig := c.generateSignature(path, sid, expiry)
	urlPath := fmt.Sprintf("%s/%s/%s", wsID, nodeID, url.PathEscape(name))
	return fmt.Sprintf("/assets/%s?sig=%s&exp=%d&sid=%s", urlPath, sig, expiry, sid)
}

// generateSignature creates an HMAC-SHA256 signature for asset access.
func (c *Config) generateSignature(path, sid string, expiry int64) string {
	data := fmt.Sprintf("mddb-asset-session-v1:%s:%s:%d", path, sid, expiry)
	mac := hmac.New(sha256.New, c.JWTSecret)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyAssetSignature checks if the provided signature is valid.
func (c *Config) VerifyAssetSignature(path, sid, sig string, expiry int64) bool {
	expected := c.generateSignature(path, sid, expiry)
	return hmac.Equal([]byte(expected), []byte(sig))
}

// AssetURLExpiry is the default duration for which signed asset URLs are valid.
const AssetURLExpiry = 1 * time.Hour

const tokenExpiration = 31 * 24 * time.Hour

// SessionClaims binds a host JWT to its persisted session and account.
type SessionClaims struct {
	jwt.RegisteredClaims
	SessionID string `json:"sid"`
	Email     string `json:"email"`
}

// IssueToken persists a session and returns its signed credential and non-secret ID.
// A nonzero ID on error identifies partial creation that the caller must revoke.
func (c *Config) IssueToken(sessionSvc *identity.SessionService, user *identity.User, clientIP, userAgent, countryCode string) (credential string, id ksid.ID, err error) {
	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(tokenExpiration)
	sessionID := ksid.NewID()
	claims := SessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		SessionID: sessionID.String(),
		Email:     user.Email,
	}

	// Generate the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	tokenString, err := token.SignedString(c.JWTSecret)
	if err != nil {
		return "", 0, err
	}

	// Create session with the pre-generated ID and token hash
	deviceInfo := devicelogin.UserAgent(userAgent)
	if created, err := sessionSvc.CreateWithID(sessionID, user.ID, utils.HashToken(tokenString), deviceInfo, clientIP, countryCode, storage.ToTime(expiresAt), c.Quotas.MaxSessionsPerUser); err != nil {
		if errors.Is(err, identity.ErrSessionQuotaExceeded) {
			return "", 0, dto.QuotaExceeded("sessions per user", c.Quotas.MaxSessionsPerUser)
		}
		if created != nil {
			return tokenString, sessionID, err
		}
		return "", 0, err
	}

	return tokenString, sessionID, nil
}

// VAPIDKeys returns a VAPIDKeys pair from the Config, or nil if unconfigured.
func (c *Config) VAPIDKeys() *vapidKeys {
	if c.VAPID.PublicKey == "" || c.VAPID.PrivateKey == "" {
		return nil
	}
	return &vapidKeys{Public: c.VAPID.PublicKey, Private: c.VAPID.PrivateKey}
}

// SessionCookieName identifies the host-only HttpOnly browser credential.
const SessionCookieName = "__Host-mddb_session"

// WriteSessionCookie serializes an already-issued session as a Secure, HttpOnly
// cookie. Local development uses localhost, which browsers treat as trustworthy.
func (c *Config) WriteSessionCookie(w http.ResponseWriter, token string) error {
	claims := &SessionClaims{}
	if _, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) { return c.JWTSecret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired()); err != nil {
		return err
	}
	if claims.ExpiresAt == nil {
		return errors.New("session expiry missing")
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: claims.ExpiresAt.Time, MaxAge: int(math.Ceil(time.Until(claims.ExpiresAt.Time).Seconds()))})
	w.Header().Set("Cache-Control", "no-store")
	return nil
}

// ClearSessionCookie expires the browser credential after server-side revocation.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.Header().Set("Cache-Control", "no-store")
}
