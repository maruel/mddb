// Validation errors returned by persisted identity rows.

// Package v1 defines version 1 of the identity tables: the JSONL rows for users, sessions,
// organizations, workspaces, memberships, invitations, notifications, and push subscriptions.
package v1

import (
	"errors"
)

// Validation errors returned by the Validate methods of version 1 rows.
var (
	// ErrEmailEmpty is returned by Validate.
	ErrEmailEmpty = errors.New("email is required")
	// ErrIDRequired is returned by Validate.
	ErrIDRequired = errors.New("id is required")
	// ErrInvalidOrgRole is returned by Validate.
	ErrInvalidOrgRole = errors.New("invalid organization role")
	// ErrInvalidUserQuota is returned by Validate.
	ErrInvalidUserQuota = errors.New("invalid user quota")
	// ErrInvalidWSRole is returned by Validate.
	ErrInvalidWSRole = errors.New("invalid workspace role")
	// ErrInvalidWorkspaceQuota is returned by Validate.
	ErrInvalidWorkspaceQuota = errors.New("invalid workspace quota")
	// ErrNameRequired is returned by Validate.
	ErrNameRequired = errors.New("name is required")
	// ErrNotificationIDRequired is returned by Validate.
	ErrNotificationIDRequired = errors.New("notification id is required")
	// ErrNotificationTitleRequired is returned by Validate.
	ErrNotificationTitleRequired = errors.New("notification title is required")
	// ErrNotificationTypeRequired is returned by Validate.
	ErrNotificationTypeRequired = errors.New("notification type is required")
	// ErrNotificationUserIDRequired is returned by Validate.
	ErrNotificationUserIDRequired = errors.New("notification user_id is required")
	// ErrOrgIDEmpty is returned by Validate.
	ErrOrgIDEmpty = errors.New("organization id cannot be empty")
	// ErrPushSubEndpointRequired is returned by Validate.
	ErrPushSubEndpointRequired = errors.New("push subscription endpoint is required")
	// ErrPushSubIDRequired is returned by Validate.
	ErrPushSubIDRequired = errors.New("push subscription id is required")
	// ErrPushSubUserIDRequired is returned by Validate.
	ErrPushSubUserIDRequired = errors.New("push subscription user_id is required")
	// ErrSessionIDRequired is returned by Validate.
	ErrSessionIDRequired = errors.New("session id is required")
	// ErrSessionTokenHashRequired is returned by Validate.
	ErrSessionTokenHashRequired = errors.New("session token_hash is required")
	// ErrSessionUserIDRequired is returned by Validate.
	ErrSessionUserIDRequired = errors.New("session user_id is required")
	// ErrTokenRequired is returned by Validate.
	ErrTokenRequired = errors.New("token is required")
	// ErrUserIDEmpty is returned by Validate.
	ErrUserIDEmpty = errors.New("user id cannot be empty")
	// ErrUserIDRequired is returned by Validate.
	ErrUserIDRequired = errors.New("id is required")
	// ErrVerificationEmailRequired is returned by Validate.
	ErrVerificationEmailRequired = errors.New("verification email is required")
	// ErrVerificationIDRequired is returned by Validate.
	ErrVerificationIDRequired = errors.New("verification id is required")
	// ErrVerificationTokenRequired is returned by Validate.
	ErrVerificationTokenRequired = errors.New("verification token is required")
	// ErrVerificationUserIDRequired is returned by Validate.
	ErrVerificationUserIDRequired = errors.New("verification user_id is required")
	// ErrWSIDEmpty is returned by Validate.
	ErrWSIDEmpty = errors.New("workspace id cannot be empty")
)
