// Manages notification entities and delivery preferences.

package identity

import (
	"cmp"
	"errors"
	"slices"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/rid"
	"github.com/maruel/mddb/backend/internal/storage"
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

// defaultChannels maps each notification type to its default delivery channels.
var defaultChannels = map[NotificationType]ChannelSet{
	NotifOrgInvite:     {Email: true, Web: true},
	NotifWSInvite:      {Email: true, Web: true},
	NotifMemberJoined:  {Web: true},
	NotifMemberRemoved: {Web: true},
	NotifPageMention:   {Email: true, Web: true},
	NotifPageEdited:    {Web: true},
}

// DefaultChannels returns the default channel set for a notification type.
func DefaultChannels(t NotificationType) ChannelSet {
	if cs, ok := defaultChannels[t]; ok {
		return cs
	}
	return ChannelSet{Web: true}
}

// AllNotificationTypes returns all defined notification types.
func AllNotificationTypes() []NotificationType {
	return []NotificationType{
		NotifOrgInvite,
		NotifWSInvite,
		NotifMemberJoined,
		NotifMemberRemoved,
		NotifPageMention,
		NotifPageEdited,
	}
}

// NotificationPreferences holds per-type channel overrides.
type NotificationPreferences struct {
	Overrides map[NotificationType]ChannelSet `json:"overrides,omitempty"`
}

// EffectiveChannels returns the user's channels for a notification type,
// falling back to defaults when no override exists.
func (p *NotificationPreferences) EffectiveChannels(t NotificationType) ChannelSet {
	if p != nil && p.Overrides != nil {
		if cs, ok := p.Overrides[t]; ok {
			return cs
		}
	}
	return DefaultChannels(t)
}

// Notification represents an in-app notification for a user.
type Notification struct {
	ID         rid.ID           `json:"id"`
	UserID     rid.ID           `json:"user_id"`
	Type       NotificationType `json:"type"`
	Title      string           `json:"title"`
	Body       string           `json:"body,omitempty"`
	ResourceID string           `json:"resource_id,omitempty"`
	ActorID    rid.ID           `json:"actor_id,omitempty"`
	Read       bool             `json:"read"`
	Created    storage.Time     `json:"created"`
}

// Clone returns a deep copy.
func (n *Notification) Clone() *Notification {
	c := *n
	return &c
}

// GetID returns the notification's ID.
func (n *Notification) GetID() rid.ID {
	return n.ID
}

// Validate checks required fields.
func (n *Notification) Validate() error {
	if n.ID.IsZero() {
		return errNotificationIDRequired
	}
	if n.UserID.IsZero() {
		return errNotificationUserIDRequired
	}
	if n.Type == "" {
		return errNotificationTypeRequired
	}
	if n.Title == "" {
		return errNotificationTitleRequired
	}
	return nil
}

// NotificationService manages notification persistence.
type NotificationService struct {
	table    *jsonldb.Table[*Notification]
	byUserID *jsonldb.Index[rid.ID, *Notification]
}

// NewNotificationService creates a new notification service.
func NewNotificationService(tablePath string) (*NotificationService, error) {
	table, err := jsonldb.NewTable[*Notification](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(n *Notification) rid.ID { return n.UserID })
	return &NotificationService{table: table, byUserID: byUserID}, nil
}

// Create creates a new notification.
func (s *NotificationService) Create(userID rid.ID, notifType NotificationType, title, body, resourceID string, actorID rid.ID) (*Notification, error) {
	n := &Notification{
		ID:         rid.NewID(),
		UserID:     userID,
		Type:       notifType,
		Title:      title,
		Body:       body,
		ResourceID: resourceID,
		ActorID:    actorID,
		Created:    storage.Now(),
	}
	if err := s.table.Append(n); err != nil {
		return nil, err
	}
	return n.Clone(), nil
}

// Get retrieves a notification by ID.
func (s *NotificationService) Get(id rid.ID) (*Notification, error) {
	n := s.table.Get(id)
	if n == nil {
		return nil, errNotificationNotFound
	}
	return n.Clone(), nil
}

// ListByUser returns notifications for a user, newest first, with optional limit, offset, and unread filter.
func (s *NotificationService) ListByUser(userID rid.ID, limit, offset int, unreadOnly bool) []*Notification {
	var all []*Notification
	for n := range s.byUserID.Iter(userID) {
		if unreadOnly && n.Read {
			continue
		}
		all = append(all, n)
	}
	// Sort newest-first by ID (IDs are time-sortable).
	slices.SortFunc(all, func(a, b *Notification) int {
		return cmp.Compare(b.ID, a.ID)
	})

	// Apply offset.
	if offset > 0 {
		if offset >= len(all) {
			return nil
		}
		all = all[offset:]
	}

	// Apply limit.
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}

	result := make([]*Notification, len(all))
	for i, n := range all {
		result[i] = n.Clone()
	}
	return result
}

// CountByUser returns the total number of notifications for a user.
func (s *NotificationService) CountByUser(userID rid.ID) int {
	count := 0
	for range s.byUserID.Iter(userID) {
		count++
	}
	return count
}

// CountUnread returns the number of unread notifications for a user.
func (s *NotificationService) CountUnread(userID rid.ID) int {
	count := 0
	for n := range s.byUserID.Iter(userID) {
		if !n.Read {
			count++
		}
	}
	return count
}

// MarkRead marks a single notification as read.
func (s *NotificationService) MarkRead(id, userID rid.ID) error {
	n := s.table.Get(id)
	if n == nil || n.UserID != userID {
		return errNotificationNotFound
	}
	_, err := s.table.Modify(id, func(n *Notification) error {
		n.Read = true
		return nil
	})
	return err
}

// MarkAllRead marks all notifications for a user as read.
func (s *NotificationService) MarkAllRead(userID rid.ID) error {
	var ids []rid.ID
	for n := range s.byUserID.Iter(userID) {
		if !n.Read {
			ids = append(ids, n.ID)
		}
	}
	for _, id := range ids {
		if _, err := s.table.Modify(id, func(n *Notification) error {
			n.Read = true
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// Delete deletes a single notification owned by the given user.
func (s *NotificationService) Delete(id, userID rid.ID) error {
	n := s.table.Get(id)
	if n == nil || n.UserID != userID {
		return errNotificationNotFound
	}
	_, err := s.table.Delete(id)
	return err
}

// DeleteOlderThan deletes notifications created before cutoff. Returns count deleted.
func (s *NotificationService) DeleteOlderThan(cutoff storage.Time) (int, error) {
	var toDelete []rid.ID
	for n := range s.table.Iter(0) {
		if n.Created.Before(cutoff) {
			toDelete = append(toDelete, n.ID)
		}
	}
	count := 0
	for _, id := range toDelete {
		if _, err := s.table.Delete(id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// DeleteExcessPerUser caps notifications per user at maxPerUser, deleting oldest. Returns total deleted.
func (s *NotificationService) DeleteExcessPerUser(maxPerUser int) (int, error) {
	byUser := make(map[rid.ID][]*Notification)
	for n := range s.table.Iter(0) {
		byUser[n.UserID] = append(byUser[n.UserID], n)
	}
	totalDeleted := 0
	for _, notifs := range byUser {
		if len(notifs) <= maxPerUser {
			continue
		}
		// notifs are in ascending ID order (oldest first); delete from the beginning.
		excess := len(notifs) - maxPerUser
		for i := range excess {
			if _, err := s.table.Delete(notifs[i].ID); err != nil {
				return totalDeleted, err
			}
			totalDeleted++
		}
	}
	return totalDeleted, nil
}

var (
	errNotificationIDRequired     = errors.New("notification id is required")
	errNotificationUserIDRequired = errors.New("notification user_id is required")
	errNotificationTypeRequired   = errors.New("notification type is required")
	errNotificationTitleRequired  = errors.New("notification title is required")
	errNotificationNotFound       = errors.New("notification not found")
)
