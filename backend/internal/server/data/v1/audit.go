// Persisted device-login audit records in device-login-audit.jsonl.

// Package v1 defines version 1 of the files the server writes beside its configuration.
package v1

import (
	"time"

	"github.com/gomodedev/gomode/devicelogin"
)

// DeviceLoginAudit is one device-login-audit.jsonl line: a pairing request and the
// approver decision.
type DeviceLoginAudit struct {
	RequestID   string                `json:"request_id"`
	SessionID   string                `json:"session_id,omitempty"`
	Kind        devicelogin.AuditKind `json:"kind"`
	Subject     string                `json:"subject"`
	Approved    bool                  `json:"approved"`
	IP          string                `json:"ip"`
	UserAgent   string                `json:"user_agent"`
	RequestedAt time.Time             `json:"requested_at"`
	DecidedAt   time.Time             `json:"decided_at"`
}
