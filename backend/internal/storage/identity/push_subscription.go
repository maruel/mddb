// Manages push subscription entities for web push notifications.

package identity

import (
	"errors"
	"iter"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// PushSubscriptionService manages push subscription persistence.
type PushSubscriptionService struct {
	table      *jsonldb.Table[*identityv1.PushSubscription]
	byUserID   *jsonldb.Index[ksid.ID, *identityv1.PushSubscription]
	byEndpoint *jsonldb.UniqueIndex[string, *identityv1.PushSubscription]
}

// NewPushSubscriptionService creates a new push subscription service.
func NewPushSubscriptionService(tablePath string) (*PushSubscriptionService, error) {
	table, err := jsonldb.NewTable[*identityv1.PushSubscription](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(p *identityv1.PushSubscription) ksid.ID { return p.UserID })
	byEndpoint := jsonldb.NewUniqueIndex(table, func(p *identityv1.PushSubscription) string { return p.Endpoint })
	return &PushSubscriptionService{table: table, byUserID: byUserID, byEndpoint: byEndpoint}, nil
}

// Create creates or replaces a push subscription. If a subscription with the
// same endpoint already exists, it is replaced (upsert).
func (s *PushSubscriptionService) Create(userID ksid.ID, endpoint, p256dh, auth string) (*identityv1.PushSubscription, error) {
	// Upsert: delete existing subscription for the same endpoint.
	if existing := s.byEndpoint.Get(endpoint); existing != nil {
		if _, err := s.table.Delete(existing.ID); err != nil {
			return nil, err
		}
	}
	sub := &identityv1.PushSubscription{
		ID:       ksid.NewID(),
		UserID:   userID,
		Endpoint: endpoint,
		P256dh:   p256dh,
		Auth:     auth,
		Created:  storagev1.Now(),
	}
	if err := s.table.Append(sub, nil); err != nil {
		return nil, err
	}
	return sub.Clone(), nil
}

// ListByUser returns all push subscriptions for a user.
func (s *PushSubscriptionService) ListByUser(userID ksid.ID) iter.Seq[*identityv1.PushSubscription] {
	return func(yield func(*identityv1.PushSubscription) bool) {
		for sub := range s.byUserID.Iter(userID) {
			if !yield(sub.Clone()) {
				return
			}
		}
	}
}

// DeleteByEndpoint deletes a push subscription by endpoint URL.
func (s *PushSubscriptionService) DeleteByEndpoint(endpoint string) error {
	existing := s.byEndpoint.Get(endpoint)
	if existing == nil {
		return errPushSubNotFound
	}
	_, err := s.table.Delete(existing.ID)
	return err
}

// Delete deletes a push subscription by ID.
func (s *PushSubscriptionService) Delete(id ksid.ID) error {
	_, err := s.table.Delete(id)
	return err
}

var (
	errPushSubNotFound = errors.New("push subscription not found")
)
