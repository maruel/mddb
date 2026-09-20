// Central notification dispatcher called from existing handlers.

package handlers

// vapidKeys holds the VAPID key pair for web push.
type vapidKeys struct {
	Public  string
	Private string
}
