package entity

// OrganizationQuota defines limits for an organization.
type OrganizationQuota struct {
	MaxPages   int   `json:"max_pages" jsonschema:"description=Maximum number of pages allowed"`
	MaxStorage int64 `json:"max_storage" jsonschema:"description=Maximum storage in bytes"`
	MaxUsers   int   `json:"max_users" jsonschema:"description=Maximum number of users allowed"`
}

// UserQuota defines limits for a user.
type UserQuota struct {
	MaxOrgs int `json:"max_orgs" jsonschema:"description=Maximum number of organizations the user can be a member of"`
}

// DefaultUserQuota returns the default quota for new users.
func DefaultUserQuota() UserQuota {
	return UserQuota{
		MaxOrgs: 3,
	}
}
