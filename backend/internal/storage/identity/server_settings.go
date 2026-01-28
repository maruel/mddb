// Manages server-wide settings stored in a JSONL table.

package identity

import (
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

// ServerSettings stores server-wide configuration.
// There is only one row in this table (ID=1).
type ServerSettings struct {
	ID       jsonldb.ID   `json:"id"`
	Quotas   ServerQuotas `json:"quotas"`
	Created  storage.Time `json:"created"`
	Modified storage.Time `json:"modified"`
}

// ServerQuotas defines server-wide resource limits.
type ServerQuotas struct {
	// MaxRequestBodyBytes limits the size of any single HTTP request body.
	MaxRequestBodyBytes int64 `json:"max_request_body_bytes"`

	// MaxSessionsPerUser limits active sessions per user.
	MaxSessionsPerUser int `json:"max_sessions_per_user"`

	// MaxTablesPerWorkspace limits tables within a single workspace.
	MaxTablesPerWorkspace int `json:"max_tables_per_workspace"`

	// MaxColumnsPerTable limits properties/columns per table.
	MaxColumnsPerTable int `json:"max_columns_per_table"`

	// MaxRowsPerTable limits records/rows per table.
	MaxRowsPerTable int `json:"max_rows_per_table"`

	// MaxOrganizations limits total organizations on the server.
	MaxOrganizations int `json:"max_organizations"`

	// MaxWorkspaces limits total workspaces on the server.
	MaxWorkspaces int `json:"max_workspaces"`

	// MaxUsers limits total users on the server.
	MaxUsers int `json:"max_users"`

	// MaxTotalStorageBytes limits total storage across all workspaces.
	MaxTotalStorageBytes int64 `json:"max_total_storage_bytes"`
}

// DefaultServerQuotas returns the default server-wide quotas.
func DefaultServerQuotas() ServerQuotas {
	return ServerQuotas{
		MaxRequestBodyBytes:   10 * 1024 * 1024,         // 10 MiB
		MaxSessionsPerUser:    10,                       // 10 sessions
		MaxTablesPerWorkspace: 100,                      // 100 tables
		MaxColumnsPerTable:    50,                       // 50 columns
		MaxRowsPerTable:       1000,                     // 1000 rows
		MaxOrganizations:      1000,                     // 1000 organizations
		MaxWorkspaces:         10000,                    // 10000 workspaces
		MaxUsers:              50,                       // 50 users
		MaxTotalStorageBytes:  100 * 1024 * 1024 * 1024, // 100 GiB
	}
}

// Clone returns a deep copy of the ServerSettings.
func (s *ServerSettings) Clone() *ServerSettings {
	c := *s
	return &c
}

// GetID returns the settings ID.
func (s *ServerSettings) GetID() jsonldb.ID {
	return s.ID
}

// Validate checks that the settings are valid.
func (s *ServerSettings) Validate() error {
	if s.ID.IsZero() {
		return errIDRequired
	}
	return nil
}

// ServerSettingsService manages server-wide settings.
type ServerSettingsService struct {
	table *jsonldb.Table[*ServerSettings]
}

// NewServerSettingsService creates a new server settings service.
// Creates default settings if none exist.
func NewServerSettingsService(tablePath string) (*ServerSettingsService, error) {
	table, err := jsonldb.NewTable[*ServerSettings](tablePath)
	if err != nil {
		return nil, err
	}

	svc := &ServerSettingsService{table: table}

	// Ensure default settings exist
	if err := svc.ensureDefaults(); err != nil {
		return nil, err
	}

	return svc, nil
}

// settingsID is the fixed ID for the single settings row.
const settingsID = jsonldb.ID(1)

// ensureDefaults creates default settings if they don't exist.
func (s *ServerSettingsService) ensureDefaults() error {
	if s.table.Get(settingsID) != nil {
		return nil
	}

	now := storage.Now()
	settings := &ServerSettings{
		ID:       settingsID,
		Quotas:   DefaultServerQuotas(),
		Created:  now,
		Modified: now,
	}
	return s.table.Append(settings)
}

// Get returns the current server settings.
func (s *ServerSettingsService) Get() *ServerSettings {
	settings := s.table.Get(settingsID)
	if settings == nil {
		// Should never happen after ensureDefaults, but return defaults as fallback
		return &ServerSettings{
			ID:     settingsID,
			Quotas: DefaultServerQuotas(),
		}
	}
	return settings.Clone()
}

// GetQuotas returns the current server quotas.
func (s *ServerSettingsService) GetQuotas() ServerQuotas {
	return s.Get().Quotas
}

// Update updates the server settings.
func (s *ServerSettingsService) Update(fn func(*ServerSettings) error) (*ServerSettings, error) {
	return s.table.Modify(settingsID, func(settings *ServerSettings) error {
		settings.Modified = storage.Now()
		return fn(settings)
	})
}

// UpdateQuotas updates just the quota settings.
func (s *ServerSettingsService) UpdateQuotas(quotas ServerQuotas) (*ServerSettings, error) {
	return s.Update(func(settings *ServerSettings) error {
		settings.Quotas = quotas
		return nil
	})
}
