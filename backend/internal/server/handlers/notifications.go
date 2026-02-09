// Handles notification API endpoints.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NotificationHandler handles notification requests.
type NotificationHandler struct {
	Svc *Services
	Cfg *Config
}

// ListNotifications returns paginated notifications for the authenticated user.
func (h *NotificationHandler) ListNotifications(_ context.Context, user *identity.User, req *dto.ListNotificationsRequest) (*dto.ListNotificationsResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	notifs := h.Svc.Notification.ListByUser(user.ID, limit, req.Offset, req.UnreadOnly)
	dtos := make([]dto.NotificationDTO, len(notifs))
	for i, n := range notifs {
		dtos[i] = notificationToDTO(n, h.Svc.User)
	}
	return &dto.ListNotificationsResponse{
		Notifications: dtos,
		Total:         h.Svc.Notification.CountByUser(user.ID),
		UnreadCount:   h.Svc.Notification.CountUnread(user.ID),
	}, nil
}

// GetUnreadCount returns the unread notification count.
func (h *NotificationHandler) GetUnreadCount(_ context.Context, user *identity.User, _ *dto.GetUnreadCountRequest) (*dto.UnreadCountResponse, error) {
	return &dto.UnreadCountResponse{Count: h.Svc.Notification.CountUnread(user.ID)}, nil
}

// MarkNotificationRead marks a single notification as read.
func (h *NotificationHandler) MarkNotificationRead(_ context.Context, user *identity.User, req *dto.MarkNotificationReadRequest) (*dto.MarkNotificationReadResponse, error) {
	if err := h.Svc.Notification.MarkRead(req.ID, user.ID); err != nil {
		return nil, dto.NotFound("notification not found")
	}
	return &dto.OkResponse{Ok: true}, nil
}

// MarkAllNotificationsRead marks all notifications for the user as read.
func (h *NotificationHandler) MarkAllNotificationsRead(_ context.Context, user *identity.User, _ *dto.MarkAllNotificationsReadRequest) (*dto.MarkAllNotificationsReadResponse, error) {
	if err := h.Svc.Notification.MarkAllRead(user.ID); err != nil {
		return nil, dto.InternalWithError("Failed to mark all as read", err)
	}
	return &dto.OkResponse{Ok: true}, nil
}

// DeleteNotification deletes a single notification.
func (h *NotificationHandler) DeleteNotification(_ context.Context, user *identity.User, req *dto.DeleteNotificationRequest) (*dto.DeleteNotificationResponse, error) {
	if err := h.Svc.Notification.Delete(req.ID, user.ID); err != nil {
		return nil, dto.NotFound("notification not found")
	}
	return &dto.OkResponse{Ok: true}, nil
}

// GetNotificationPrefs returns the user's notification channel preferences.
func (h *NotificationHandler) GetNotificationPrefs(_ context.Context, user *identity.User, _ *dto.GetNotificationPrefsRequest) (*dto.NotificationPrefsDTO, error) {
	return notificationPrefsToDTO(&user.Settings.NotificationPrefs), nil
}

// UpdateNotificationPrefs updates the user's notification channel preferences.
func (h *NotificationHandler) UpdateNotificationPrefs(_ context.Context, user *identity.User, req *dto.UpdateNotificationPrefsRequest) (*dto.NotificationPrefsDTO, error) {
	overrides := make(map[identity.NotificationType]identity.ChannelSet, len(req.Overrides))
	for k, v := range req.Overrides {
		overrides[identity.NotificationType(k)] = identity.ChannelSet{Email: v.Email, Web: v.Web}
	}
	_, err := h.Svc.User.Modify(user.ID, func(u *identity.User) error {
		u.Settings.NotificationPrefs = identity.NotificationPreferences{Overrides: overrides}
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update preferences", err)
	}
	updated, err := h.Svc.User.Get(user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return notificationPrefsToDTO(&updated.Settings.NotificationPrefs), nil
}

// GetVAPIDPublicKey returns the server's VAPID public key for push subscription.
func (h *NotificationHandler) GetVAPIDPublicKey(_ context.Context, user *identity.User, _ *dto.GetVAPIDKeyRequest) (*dto.VAPIDKeyResponse, error) {
	return &dto.VAPIDKeyResponse{PublicKey: h.Cfg.VAPID.PublicKey}, nil
}

// SubscribePush saves a push subscription for the authenticated user.
func (h *NotificationHandler) SubscribePush(_ context.Context, user *identity.User, req *dto.PushSubscribeRequest) (*dto.PushSubscribeResponse, error) {
	if _, err := h.Svc.PushSubscription.Create(user.ID, req.Endpoint, req.P256dh, req.Auth); err != nil {
		return nil, dto.InternalWithError("Failed to save push subscription", err)
	}
	return &dto.OkResponse{Ok: true}, nil
}

// UnsubscribePush removes a push subscription.
func (h *NotificationHandler) UnsubscribePush(_ context.Context, user *identity.User, req *dto.PushUnsubscribeRequest) (*dto.PushUnsubscribeResponse, error) {
	if err := h.Svc.PushSubscription.DeleteByEndpoint(req.Endpoint); err != nil {
		return nil, dto.NotFound("subscription not found")
	}
	return &dto.OkResponse{Ok: true}, nil
}

// --- DTO conversion helpers ---

func notificationToDTO(n *identity.Notification, userSvc *identity.UserService) dto.NotificationDTO {
	d := dto.NotificationDTO{
		ID:         n.ID,
		Type:       string(n.Type),
		Title:      n.Title,
		Body:       n.Body,
		ResourceID: n.ResourceID,
		ActorID:    n.ActorID,
		Read:       n.Read,
		CreatedAt:  n.Created,
	}
	if !n.ActorID.IsZero() {
		if actor, err := userSvc.Get(n.ActorID); err == nil {
			d.ActorName = actor.Name
		}
	}
	return d
}

func notificationPrefsToDTO(prefs *identity.NotificationPreferences) *dto.NotificationPrefsDTO {
	defaults := make(map[string]dto.ChannelSetDTO, len(identity.AllNotificationTypes()))
	for _, t := range identity.AllNotificationTypes() {
		cs := identity.DefaultChannels(t)
		defaults[string(t)] = dto.ChannelSetDTO{Email: cs.Email, Web: cs.Web}
	}
	overrides := make(map[string]dto.ChannelSetDTO, len(prefs.Overrides))
	for k, v := range prefs.Overrides {
		overrides[string(k)] = dto.ChannelSetDTO{Email: v.Email, Web: v.Web}
	}
	return &dto.NotificationPrefsDTO{
		Defaults:  defaults,
		Overrides: overrides,
	}
}
