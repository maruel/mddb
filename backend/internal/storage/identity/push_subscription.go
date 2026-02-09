// Manages push subscription entities for web push notifications.

package identity

import (
	"errors"
	"iter"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

// PushSubscription stores a Web Push subscription for a user.
type PushSubscription struct {
	ID       jsonldb.ID   `json:"id"`
	UserID   jsonldb.ID   `json:"user_id"`
	Endpoint string       `json:"endpoint"`
	P256dh   string       `json:"p256dh"`
	Auth     string       `json:"auth"`
	Created  storage.Time `json:"created"`
}

// Clone returns a deep copy.
func (p *PushSubscription) Clone() *PushSubscription {
	c := *p
	return &c
}

// GetID returns the subscription's ID.
func (p *PushSubscription) GetID() jsonldb.ID {
	return p.ID
}

// Validate checks required fields.
func (p *PushSubscription) Validate() error {
	if p.ID.IsZero() {
		return errPushSubIDRequired
	}
	if p.UserID.IsZero() {
		return errPushSubUserIDRequired
	}
	if p.Endpoint == "" {
		return errPushSubEndpointRequired
	}
	return nil
}

// PushSubscriptionService manages push subscription persistence.
type PushSubscriptionService struct {
	table      *jsonldb.Table[*PushSubscription]
	byUserID   *jsonldb.Index[jsonldb.ID, *PushSubscription]
	byEndpoint *jsonldb.UniqueIndex[string, *PushSubscription]
}

// NewPushSubscriptionService creates a new push subscription service.
func NewPushSubscriptionService(tablePath string) (*PushSubscriptionService, error) {
	table, err := jsonldb.NewTable[*PushSubscription](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(p *PushSubscription) jsonldb.ID { return p.UserID })
	byEndpoint := jsonldb.NewUniqueIndex(table, func(p *PushSubscription) string { return p.Endpoint })
	return &PushSubscriptionService{table: table, byUserID: byUserID, byEndpoint: byEndpoint}, nil
}

// Create creates or replaces a push subscription. If a subscription with the
// same endpoint already exists, it is replaced (upsert).
func (s *PushSubscriptionService) Create(userID jsonldb.ID, endpoint, p256dh, auth string) (*PushSubscription, error) {
	// Upsert: delete existing subscription for the same endpoint.
	if existing := s.byEndpoint.Get(endpoint); existing != nil {
		if _, err := s.table.Delete(existing.ID); err != nil {
			return nil, err
		}
	}
	sub := &PushSubscription{
		ID:       jsonldb.NewID(),
		UserID:   userID,
		Endpoint: endpoint,
		P256dh:   p256dh,
		Auth:     auth,
		Created:  storage.Now(),
	}
	if err := s.table.Append(sub); err != nil {
		return nil, err
	}
	return sub.Clone(), nil
}

// ListByUser returns all push subscriptions for a user.
func (s *PushSubscriptionService) ListByUser(userID jsonldb.ID) iter.Seq[*PushSubscription] {
	return func(yield func(*PushSubscription) bool) {
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
func (s *PushSubscriptionService) Delete(id jsonldb.ID) error {
	_, err := s.table.Delete(id)
	return err
}

var (
	errPushSubIDRequired       = errors.New("push subscription id is required")
	errPushSubUserIDRequired   = errors.New("push subscription user_id is required")
	errPushSubEndpointRequired = errors.New("push subscription endpoint is required")
	errPushSubNotFound         = errors.New("push subscription not found")
)
