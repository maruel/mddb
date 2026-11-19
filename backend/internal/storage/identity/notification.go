// Manages notification entities and delivery preferences.

package identity

import (
	"cmp"
	"errors"
	"slices"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// defaultChannels maps each notification type to its default delivery channels.
var defaultChannels = map[identityv1.NotificationType]identityv1.ChannelSet{
	identityv1.NotifOrgInvite:     {Email: true, Web: true},
	identityv1.NotifWSInvite:      {Email: true, Web: true},
	identityv1.NotifMemberJoined:  {Web: true},
	identityv1.NotifMemberRemoved: {Web: true},
	identityv1.NotifPageMention:   {Email: true, Web: true},
	identityv1.NotifPageEdited:    {Web: true},
}

// DefaultChannels returns the default channel set for a notification type.
func DefaultChannels(t identityv1.NotificationType) identityv1.ChannelSet {
	if cs, ok := defaultChannels[t]; ok {
		return cs
	}
	return identityv1.ChannelSet{Web: true}
}

// AllNotificationTypes returns all defined notification types.
func AllNotificationTypes() []identityv1.NotificationType {
	return []identityv1.NotificationType{
		identityv1.NotifOrgInvite,
		identityv1.NotifWSInvite,
		identityv1.NotifMemberJoined,
		identityv1.NotifMemberRemoved,
		identityv1.NotifPageMention,
		identityv1.NotifPageEdited,
	}
}

// EffectiveChannels returns the user's channels for a notification type,
// falling back to defaults when no override exists.
func EffectiveChannels(p *identityv1.NotificationPreferences, t identityv1.NotificationType) identityv1.ChannelSet {
	if p != nil && p.Overrides != nil {
		if cs, ok := p.Overrides[t]; ok {
			return cs
		}
	}
	return DefaultChannels(t)
}

// NotificationService manages notification persistence.
type NotificationService struct {
	table    *jsonldb.Table[*identityv1.Notification]
	byUserID *jsonldb.Index[ksid.ID, *identityv1.Notification]
}

// NewNotificationService creates a new notification service.
func NewNotificationService(tablePath string) (*NotificationService, error) {
	table, err := jsonldb.NewTable[*identityv1.Notification](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(n *identityv1.Notification) ksid.ID { return n.UserID })
	return &NotificationService{table: table, byUserID: byUserID}, nil
}

// Create creates a new notification.
func (s *NotificationService) Create(userID ksid.ID, notifType identityv1.NotificationType, title, body, resourceID string, actorID ksid.ID) (*identityv1.Notification, error) {
	n := &identityv1.Notification{
		ID:         ksid.NewID(),
		UserID:     userID,
		Type:       notifType,
		Title:      title,
		Body:       body,
		ResourceID: resourceID,
		ActorID:    actorID,
		Created:    storagev1.Now(),
	}
	if err := s.table.Append(n); err != nil {
		return nil, err
	}
	return n.Clone(), nil
}

// Get retrieves a notification by ID.
func (s *NotificationService) Get(id ksid.ID) (*identityv1.Notification, error) {
	n := s.table.Get(id)
	if n == nil {
		return nil, errNotificationNotFound
	}
	return n.Clone(), nil
}

// ListByUser returns notifications for a user, newest first, with optional limit, offset, and unread filter.
func (s *NotificationService) ListByUser(userID ksid.ID, limit, offset int, unreadOnly bool) []*identityv1.Notification {
	var all []*identityv1.Notification
	for n := range s.byUserID.Iter(userID) {
		if unreadOnly && n.Read {
			continue
		}
		all = append(all, n)
	}
	// Sort newest-first by ID (IDs are time-sortable).
	slices.SortFunc(all, func(a, b *identityv1.Notification) int {
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

	result := make([]*identityv1.Notification, len(all))
	for i, n := range all {
		result[i] = n.Clone()
	}
	return result
}

// CountByUser returns the total number of notifications for a user.
func (s *NotificationService) CountByUser(userID ksid.ID) int {
	count := 0
	for range s.byUserID.Iter(userID) {
		count++
	}
	return count
}

// CountUnread returns the number of unread notifications for a user.
func (s *NotificationService) CountUnread(userID ksid.ID) int {
	count := 0
	for n := range s.byUserID.Iter(userID) {
		if !n.Read {
			count++
		}
	}
	return count
}

// MarkRead marks a single notification as read.
func (s *NotificationService) MarkRead(id, userID ksid.ID) error {
	n := s.table.Get(id)
	if n == nil || n.UserID != userID {
		return errNotificationNotFound
	}
	_, err := s.table.Modify(id, func(n *identityv1.Notification) error {
		n.Read = true
		return nil
	})
	return err
}

// MarkAllRead marks all notifications for a user as read.
func (s *NotificationService) MarkAllRead(userID ksid.ID) error {
	var ids []ksid.ID
	for n := range s.byUserID.Iter(userID) {
		if !n.Read {
			ids = append(ids, n.ID)
		}
	}
	for _, id := range ids {
		if _, err := s.table.Modify(id, func(n *identityv1.Notification) error {
			n.Read = true
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// Delete deletes a single notification owned by the given user.
func (s *NotificationService) Delete(id, userID ksid.ID) error {
	n := s.table.Get(id)
	if n == nil || n.UserID != userID {
		return errNotificationNotFound
	}
	_, err := s.table.Delete(id)
	return err
}

// DeleteOlderThan deletes notifications created before cutoff. Returns count deleted.
func (s *NotificationService) DeleteOlderThan(cutoff storagev1.Time) (int, error) {
	deleted, err := s.table.DeleteFunc(func(n *identityv1.Notification) bool { return n.Created.Before(cutoff) })
	return len(deleted), err
}

// DeleteExcessPerUser caps notifications per user at maxPerUser, deleting oldest. Returns total deleted.
func (s *NotificationService) DeleteExcessPerUser(maxPerUser int) (int, error) {
	// Rows are in ascending ID order (oldest first): keep each user's newest
	// maxPerUser by counting each user's rows from the end.
	total := make(map[ksid.ID]int)
	for n := range s.table.Iter(0) {
		total[n.UserID]++
	}
	seen := make(map[ksid.ID]int)
	deleted, err := s.table.DeleteFunc(func(n *identityv1.Notification) bool {
		seen[n.UserID]++
		return seen[n.UserID] <= total[n.UserID]-maxPerUser
	})
	return len(deleted), err
}

var (
	errNotificationNotFound = errors.New("notification not found")
)
