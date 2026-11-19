// Persisted Web Push subscription rows.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// PushSubscription stores a Web Push subscription for a user.
type PushSubscription struct {
	ID       ksid.ID        `json:"id"`
	UserID   ksid.ID        `json:"user_id"`
	Endpoint string         `json:"endpoint"`
	P256dh   string         `json:"p256dh"`
	Auth     string         `json:"auth"`
	Created  storagev1.Time `json:"created"`
}

// Clone returns a deep copy.
func (p *PushSubscription) Clone() *PushSubscription {
	c := *p
	return &c
}

// GetID returns the subscription's ID.
func (p *PushSubscription) GetID() ksid.ID {
	return p.ID
}

// Validate checks required fields.
func (p *PushSubscription) Validate() error {
	if p.ID.IsZero() {
		return ErrPushSubIDRequired
	}
	if p.UserID.IsZero() {
		return ErrPushSubUserIDRequired
	}
	if p.Endpoint == "" {
		return ErrPushSubEndpointRequired
	}
	return nil
}
