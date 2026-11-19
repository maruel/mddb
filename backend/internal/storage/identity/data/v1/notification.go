// Persisted notification rows, notification types, and channel overrides.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// NotificationType represents a category of notification.
type NotificationType string

const (
	// NotifOrgInvite is sent when a user is invited to an organization.
	NotifOrgInvite NotificationType = "org_invite"
	// NotifWSInvite is sent when a user is invited to a workspace.
	NotifWSInvite NotificationType = "ws_invite"
	// NotifMemberJoined is sent when a new member joins an org/workspace.
	NotifMemberJoined NotificationType = "member_joined"
	// NotifMemberRemoved is sent when a member is removed from an org/workspace.
	NotifMemberRemoved NotificationType = "member_removed"
	// NotifPageMention is sent when someone @-mentions you in a page.
	NotifPageMention NotificationType = "page_mention"
	// NotifPageEdited is sent when a page you follow is edited.
	NotifPageEdited NotificationType = "page_edited"
)

// ChannelSet indicates which delivery channels are enabled for a notification type.
type ChannelSet struct {
	Email bool `json:"email"`
	Web   bool `json:"web"`
}

// NotificationPreferences holds per-type channel overrides.
type NotificationPreferences struct {
	Overrides map[NotificationType]ChannelSet `json:"overrides,omitempty"`
}

// Notification represents an in-app notification for a user.
type Notification struct {
	ID         ksid.ID          `json:"id"`
	UserID     ksid.ID          `json:"user_id"`
	Type       NotificationType `json:"type"`
	Title      string           `json:"title"`
	Body       string           `json:"body,omitempty"`
	ResourceID string           `json:"resource_id,omitempty"`
	ActorID    ksid.ID          `json:"actor_id,omitempty"`
	Read       bool             `json:"read"`
	Created    storagev1.Time   `json:"created"`
}

// Clone returns a deep copy.
func (n *Notification) Clone() *Notification {
	c := *n
	return &c
}

// GetID returns the notification's ID.
func (n *Notification) GetID() ksid.ID {
	return n.ID
}

// Validate checks required fields.
func (n *Notification) Validate() error {
	if n.ID.IsZero() {
		return ErrNotificationIDRequired
	}
	if n.UserID.IsZero() {
		return ErrNotificationUserIDRequired
	}
	if n.Type == "" {
		return ErrNotificationTypeRequired
	}
	if n.Title == "" {
		return ErrNotificationTitleRequired
	}
	return nil
}
