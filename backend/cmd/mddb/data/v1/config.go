// Package v1 defines version 1 of the mddb config.toml schema.
//
// Zero values mean "not set in the file"; package main pre-populates every default so
// decoding only overwrites the keys the file names, then validates and converts.
//
// IMPORTANT: When adding or modifying configuration fields, update
// contrib/config.toml accordingly. Document all default values in the example.
package v1

import voicegatewayv1 "github.com/gomodedev/gomode/voicegateway/data/v1"

// File mirrors the config.toml file layout.
type File struct {
	AutoUpdate   string       `toml:"auto_update"`
	Server       Server       `toml:"server"`
	OAuth        OAuth        `toml:"oauth"`
	GitHub       GitHub       `toml:"github"`
	VoiceGateway VoiceGateway `toml:"voice-gateway"`
	Debug        Debug        `toml:"debug"`
}

// Server is the [server] table.
type Server struct {
	HTTP         string   `toml:"http"`
	DataDir      string   `toml:"data_dir"`
	BaseURL      string   `toml:"base_url"`
	GeoDB        string   `toml:"geo_db"`
	AllowOrigins []string `toml:"allow_origins"`

	TrustedProxies []string `toml:"trusted_proxies"`
}

// OAuth is the [oauth] table.
type OAuth struct {
	Google    OAuthProvider `toml:"google"`
	Microsoft OAuthProvider `toml:"microsoft"`
	GitHub    OAuthProvider `toml:"github"`
}

// OAuthProvider holds one [oauth.<provider>] block.
type OAuthProvider struct {
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
}

// GitHub is the [github] table.
type GitHub struct {
	App GitHubApp `toml:"app"`
}

// GitHubApp is the [github.app] table.
type GitHubApp struct {
	ID             int64  `toml:"id"`
	PrivateKeyFile string `toml:"private_key_file"`
	WebhookSecret  string `toml:"webhook_secret"`
}

// VoiceGateway is the [voice-gateway] table. It embeds Go Mode's published WebRTC
// fragment, whose keys sit directly in this table.
type VoiceGateway struct {
	voicegatewayv1.WebRTC

	Mode                 VoiceGatewayMode `toml:"mode"`
	URL                  string           `toml:"url"`
	InstanceID           string           `toml:"instance_id"`
	TokenMode            string           `toml:"token_mode"`
	SigningPrivateKeyPEM string           `toml:"signing_private_key_pem"`
}

// VoiceGatewayMode selects the host transport without consulting provider credentials.
type VoiceGatewayMode string

// Voice gateway modes.
const (
	VoiceGatewayDisabled VoiceGatewayMode = "disabled"
	VoiceGatewayEmbedded VoiceGatewayMode = "embedded"
	VoiceGatewayExternal VoiceGatewayMode = "external"
)

// Debug is the [debug] table.
type Debug struct {
	LogLevel string `toml:"log_level"`
}
