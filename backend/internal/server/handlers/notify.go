// Central notification dispatcher called from existing handlers.

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

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

// VAPIDKeys returns a VAPIDKeys pair from the Config, or nil if unconfigured.
func (c *Config) VAPIDKeys() *vapidKeys {
	if c.VAPID.PublicKey == "" || c.VAPID.PrivateKey == "" {
		return nil
	}
	return &vapidKeys{Public: c.VAPID.PublicKey, Private: c.VAPID.PrivateKey}
}

// vapidKeys holds the VAPID key pair for web push.
type vapidKeys struct {
	Public  string
	Private string
}
