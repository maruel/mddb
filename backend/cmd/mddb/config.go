// Loads mddb's config.toml instance configuration.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// defaultConfigDir returns the mddb config directory: $XDG_CONFIG_HOME/mddb,
// with a fallback to ~/.config/mddb.
func defaultConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mddb")
}

// configPath returns the path of the configuration file inside cfgDir.
func configPath(cfgDir string) string {
	return filepath.Join(cfgDir, "config.toml")
}

// tomlConfig mirrors the config.toml file layout. Zero values mean "not set in
// the file"; defaultConfig pre-populates every default so decoding only
// overwrites the keys the file names.
//
// IMPORTANT: When adding or modifying configuration fields, update
// contrib/config.toml accordingly. Document all default values in the example.
type tomlConfig struct {
	Server       tomlServer       `toml:"server"`
	OAuth        tomlOAuth        `toml:"oauth"`
	GitHub       tomlGitHub       `toml:"github"`
	VoiceGateway tomlVoiceGateway `toml:"voice-gateway"`
	Debug        tomlDebug        `toml:"debug"`
}

type tomlServer struct {
	HTTP    string `toml:"http"`
	DataDir string `toml:"data_dir"`
	BaseURL string `toml:"base_url"`
	GeoDB   string `toml:"geo_db"`
}

type tomlOAuth struct {
	Google    tomlOAuthProvider `toml:"google"`
	Microsoft tomlOAuthProvider `toml:"microsoft"`
	GitHub    tomlOAuthProvider `toml:"github"`
}

// tomlOAuthProvider holds one [oauth.<provider>] block.
type tomlOAuthProvider struct {
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
}

type tomlGitHub struct {
	App tomlGitHubApp `toml:"app"`
}

type tomlGitHubApp struct {
	ID             int64  `toml:"id"`
	PrivateKeyFile string `toml:"private_key_file"`
	WebhookSecret  string `toml:"webhook_secret"`
}

type tomlVoiceGateway struct {
	APIKey string `toml:"api_key"`
}

type tomlDebug struct {
	LogLevel string `toml:"log_level"`
}

// defaultConfig returns the built-in configuration, which is what an absent or
// partially written config.toml resolves to.
func defaultConfig() tomlConfig {
	return tomlConfig{
		Server: tomlServer{
			HTTP:    "localhost:8080",
			DataDir: "data",
			BaseURL: "http://localhost",
		},
		Debug: tomlDebug{LogLevel: "info"},
	}
}

// loadTOMLConfig reads cfgDir/config.toml over the defaults. An absent file is
// not an error; a malformed file or an unknown key fails with the file path.
func loadTOMLConfig(cfgDir string) (tomlConfig, error) {
	path := configPath(cfgDir)
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is the operator's config directory
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return tomlConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	tc := defaultConfig()
	dec := toml.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tc); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return tomlConfig{}, fmt.Errorf("parse %s: unknown keys: %s", path, unknownKeys(strict.Errors))
		}
		return tomlConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	slog.Info("loaded config", "path", path)
	return tc, nil
}

// unknownKeys formats the offending dotted key paths reported by strict
// TOML decoding.
func unknownKeys(errs []toml.DecodeError) string {
	keys := make([]string, 0, len(errs))
	for i := range errs {
		if key := errs[i].Key(); len(key) != 0 {
			keys = append(keys, strings.Join(key, "."))
			continue
		}
		keys = append(keys, errs[i].Error())
	}
	return strings.Join(keys, ", ")
}

// oauthCredentials holds one provider's client ID and secret.
type oauthCredentials struct {
	ClientID     string
	ClientSecret string
}

type oauthCredentialsSet struct {
	Google    oauthCredentials
	Microsoft oauthCredentials
	GitHub    oauthCredentials
}

type githubAppSettings struct {
	ID             int64
	PrivateKeyFile string // absolute, or empty when no app is configured
	WebhookSecret  string
}

// resolvedConfig is the startup configuration after defaults, path resolution,
// and validation.
type resolvedConfig struct {
	ConfigDir   string
	HTTP        string
	DataDir     string // relative to the working directory, or absolute
	BaseURL     string
	LogLevel    slog.Level
	GeoDB       string // absolute, or empty when IP geolocation is disabled
	TestOAuth   bool   // set only in an e2e build, which fakes the OAuth providers
	OAuth       oauthCredentialsSet
	GitHubApp   githubAppSettings
	VoiceAPIKey string
}

// resolveConfig validates tc and resolves its paths.
//
// data_dir is relative to the working directory, like the flag it replaces.
// geo_db and github.app.private_key_file are configuration-side files, so they
// are relative to cfgDir.
func resolveConfig(tc *tomlConfig, cfgDir string) (*resolvedConfig, error) {
	logLevel, err := parseLogLevel(tc.Debug.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("debug.log_level: %w", err)
	}
	google, err := resolveOAuthProvider("oauth.google", tc.OAuth.Google)
	if err != nil {
		return nil, err
	}
	microsoft, err := resolveOAuthProvider("oauth.microsoft", tc.OAuth.Microsoft)
	if err != nil {
		return nil, err
	}
	github, err := resolveOAuthProvider("oauth.github", tc.OAuth.GitHub)
	if err != nil {
		return nil, err
	}
	testOAuth := e2eBuild
	if testOAuth {
		google = fakeOAuthCredentials(google, "Google", "test-google-client-id", "test-google-client-secret")
		microsoft = fakeOAuthCredentials(microsoft, "Microsoft", "test-ms-client-id", "test-ms-client-secret")
		github = fakeOAuthCredentials(github, "GitHub", "test-github-client-id", "test-github-client-secret")
	}
	ghApp, err := resolveGitHubApp(tc.GitHub.App, cfgDir)
	if err != nil {
		return nil, err
	}
	geoDB := ""
	if tc.Server.GeoDB != "" {
		geoDB = resolveConfigPath(tc.Server.GeoDB, cfgDir)
	}
	return &resolvedConfig{
		ConfigDir:   cfgDir,
		HTTP:        tc.Server.HTTP,
		DataDir:     tc.Server.DataDir,
		BaseURL:     tc.Server.BaseURL,
		LogLevel:    logLevel,
		GeoDB:       geoDB,
		TestOAuth:   testOAuth,
		OAuth:       oauthCredentialsSet{Google: google, Microsoft: microsoft, GitHub: github},
		GitHubApp:   ghApp,
		VoiceAPIKey: tc.VoiceGateway.APIKey,
	}, nil
}

// resolveOAuthProvider validates one provider's credentials. section names the
// configuration path for error messages.
func resolveOAuthProvider(section string, p tomlOAuthProvider) (oauthCredentials, error) {
	if (p.ClientID == "") != (p.ClientSecret == "") {
		return oauthCredentials{}, fmt.Errorf("%s: client_id and client_secret must both be set or both be empty", section)
	}
	return oauthCredentials(p), nil
}

// fakeOAuthCredentials replaces empty credentials with fakes in an e2e build.
func fakeOAuthCredentials(creds oauthCredentials, provider, id, secret string) oauthCredentials {
	if creds.ClientID != "" {
		return creds
	}
	slog.Info("e2e build: using fake " + provider + " OAuth credentials")
	return oauthCredentials{ClientID: id, ClientSecret: secret}
}

// resolveGitHubApp validates the GitHub App block and resolves its private key
// file against cfgDir.
func resolveGitHubApp(app tomlGitHubApp, cfgDir string) (githubAppSettings, error) {
	if (app.ID == 0) != (app.PrivateKeyFile == "") {
		return githubAppSettings{}, errors.New("github.app: id and private_key_file must both be set or both be empty")
	}
	if app.ID == 0 {
		return githubAppSettings{}, nil
	}
	return githubAppSettings{
		ID:             app.ID,
		PrivateKeyFile: resolveConfigPath(app.PrivateKeyFile, cfgDir),
		WebhookSecret:  app.WebhookSecret,
	}, nil
}

// resolveConfigPath resolves a configuration-side file path against cfgDir.
// An empty path stays empty.
func resolveConfigPath(path, cfgDir string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cfgDir, path)
}

// parseLogLevel maps a configured log level to a slog level.
func parseLogLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q", level)
	}
}
