// Tests for TOML config loading, validation, and path resolution.
package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// uncommentDirective strips the leading '#' from a commented-out TOML directive
// (a line like "#key = value"), leaving prose comments ("# explanation") intact.
// The two are distinguished by the character after the '#': a default directive
// has no space, a prose comment or example does. contrib/config.toml relies on
// this convention so uncommenting its defaults reproduces defaultConfig().
var uncommentDirective = regexp.MustCompile(`(?m)^#([^ ])`)

// TestContribConfigDefaults uncomments every default directive in the shipped
// contrib/config.toml and verifies the result loads into exactly the built-in
// defaults. This guards the real template (not a copy) on three axes at once:
//   - faithfulness: loadTOMLConfig uses DisallowUnknownFields, so an
//     undocumented or misspelled key fails the load;
//   - accuracy: the documented defaults must match defaultConfig();
//   - coverage: every key defaultConfig() knows must appear in the example.
func TestContribConfigDefaults(t *testing.T) {
	t.Parallel()
	const contribPath = "../../../contrib/config.toml"
	src, err := os.ReadFile(contribPath)
	if err != nil {
		t.Fatal(err)
	}
	stripped := uncommentDirective.ReplaceAll(src, []byte("$1"))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), stripped, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadTOMLConfig(dir)
	if err != nil {
		t.Fatalf("%s documents a key mddb rejects, or a malformed default: %v", contribPath, err)
	}
	want := defaultConfig()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("uncommenting %s does not reproduce the defaults:\n got = %+v\nwant = %+v", contribPath, got, want)
	}

	// Every key the decoder understands must be documented in the example.
	var documented map[string]any
	if err := toml.Unmarshal(stripped, &documented); err != nil {
		t.Fatal(err)
	}
	for _, path := range tomlKeyPaths(reflect.ValueOf(want), nil) {
		value := any(documented)
		for _, part := range path {
			table, ok := value.(map[string]any)
			if !ok {
				break
			}
			value, ok = table[part]
			if !ok {
				t.Errorf("%s does not document key %s", contribPath, strings.Join(path, "."))
				break
			}
		}
	}
}

// tomlKeyPaths returns the dotted TOML key path of every leaf field of v.
func tomlKeyPaths(v reflect.Value, prefix []string) [][]string {
	var paths [][]string
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("toml"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		path := append(append([]string{}, prefix...), name)
		if field.Type.Kind() == reflect.Struct {
			paths = append(paths, tomlKeyPaths(v.Field(i), path)...)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func TestLoadTOMLConfig(t *testing.T) {
	t.Parallel()

	writeConfig := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("missing file uses defaults", func(t *testing.T) {
		t.Parallel()
		got, err := loadTOMLConfig(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if want := defaultConfig(); !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("parses every field", func(t *testing.T) {
		t.Parallel()
		dir := writeConfig(t, `
[server]
http = ":9090"
data_dir = "/srv/mddb"
base_url = "https://mddb.example.com"
geo_db = "GeoLite2-Country.mmdb"

[oauth.google]
client_id = "google-id"
client_secret = "google-secret"

[oauth.microsoft]
client_id = "microsoft-id"
client_secret = "microsoft-secret"

[oauth.github]
client_id = "github-id"
client_secret = "github-secret"

[github.app]
id = 42
private_key_file = "github-app.pem"
webhook_secret = "webhook-secret"

[voice-gateway]
api_key = "gemini-key"

[debug]
log_level = "debug"
`)
		got, err := loadTOMLConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := tomlConfig{
			Server: tomlServer{
				HTTP:    ":9090",
				DataDir: "/srv/mddb",
				BaseURL: "https://mddb.example.com",
				GeoDB:   "GeoLite2-Country.mmdb",
			},
			OAuth: tomlOAuth{
				Google:    tomlOAuthProvider{ClientID: "google-id", ClientSecret: "google-secret"},
				Microsoft: tomlOAuthProvider{ClientID: "microsoft-id", ClientSecret: "microsoft-secret"},
				GitHub:    tomlOAuthProvider{ClientID: "github-id", ClientSecret: "github-secret"},
			},
			GitHub: tomlGitHub{App: tomlGitHubApp{
				ID:             42,
				PrivateKeyFile: "github-app.pem",
				WebhookSecret:  "webhook-secret",
			}},
			VoiceGateway: tomlVoiceGateway{APIKey: "gemini-key"},
			Debug:        tomlDebug{LogLevel: "debug"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("partial file keeps defaults", func(t *testing.T) {
		t.Parallel()
		got, err := loadTOMLConfig(writeConfig(t, "[debug]\nlog_level = \"warn\"\n"))
		if err != nil {
			t.Fatal(err)
		}
		want := defaultConfig()
		want.Debug.LogLevel = "warn"
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("malformed file fails with the path", func(t *testing.T) {
		t.Parallel()
		dir := writeConfig(t, "[server]\nhttp = \n")
		_, err := loadTOMLConfig(dir)
		if err == nil {
			t.Fatal("expected an error")
		}
		if want := configPath(dir); !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	})

	t.Run("unknown key fails with the offending path", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name     string
			content  string
			wantPath string
		}{
			{"misspelled server key", "[server]\nhtpp = \":8080\"\n", "server.htpp"},
			{"unknown section", "[server]\n[server.extra]\nkey = 1\n", "server.extra"},
			{"unknown oauth provider", "[oauth.myspace]\nclient_id = \"x\"\n", "oauth.myspace"},
			{"unknown debug key", "[debug]\nno_log_time = true\n", "debug.no_log_time"},
			{"unknown voice key", "[voice-gateway]\napi_key_env = \"GEMINI_API_KEY\"\n", "voice-gateway.api_key_env"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				dir := writeConfig(t, tc.content)
				_, err := loadTOMLConfig(dir)
				if err == nil {
					t.Fatalf("expected an error for %q", tc.content)
				}
				if want := configPath(dir); !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
				if !strings.Contains(err.Error(), tc.wantPath) {
					t.Errorf("error %q does not name %q", err, tc.wantPath)
				}
			})
		}
	})
}

func TestResolveConfig(t *testing.T) {
	cfgDir := t.TempDir()

	t.Run("defaults", func(t *testing.T) {
		tc := defaultConfig()
		got, err := resolveConfig(&tc, cfgDir)
		if err != nil {
			t.Fatal(err)
		}
		if got.LogLevel != slog.LevelInfo {
			t.Errorf("got log level %v, want info", got.LogLevel)
		}
		if got.ConfigDir != cfgDir || got.DataDir != "data" || got.HTTP != "localhost:8080" || got.BaseURL != "http://localhost" {
			t.Errorf("got %+v", got)
		}
		if got.GeoDB != "" || got.VoiceAPIKey != "" {
			t.Errorf("expected geo_db and voice api key to be off: %+v", got)
		}
		if got.GitHubApp != (githubAppSettings{}) {
			t.Errorf("expected no GitHub App: %+v", got)
		}
	})

	t.Run("oauth credentials", func(t *testing.T) {
		tc := defaultConfig()
		tc.OAuth.Google = tomlOAuthProvider{ClientID: "google-id", ClientSecret: "google-secret"}
		tc.OAuth.Microsoft = tomlOAuthProvider{ClientID: "microsoft-id", ClientSecret: "microsoft-secret"}
		tc.OAuth.GitHub = tomlOAuthProvider{ClientID: "github-id", ClientSecret: "github-secret"}
		got, err := resolveConfig(&tc, cfgDir)
		if err != nil {
			t.Fatal(err)
		}
		want := oauthCredentialsSet{
			Google:    oauthCredentials{ClientID: "google-id", ClientSecret: "google-secret"},
			Microsoft: oauthCredentials{ClientID: "microsoft-id", ClientSecret: "microsoft-secret"},
			GitHub:    oauthCredentials{ClientID: "github-id", ClientSecret: "github-secret"},
		}
		if got.OAuth != want {
			t.Errorf("got %+v, want %+v", got.OAuth, want)
		}
	})

	t.Run("incomplete oauth pair fails with the section path", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			mutate  func(*tomlConfig)
			wantErr string
		}{
			{"client id without secret", func(tc *tomlConfig) {
				tc.OAuth.Google = tomlOAuthProvider{ClientID: "google-id"}
			}, "oauth.google"},
			{"secret without client id", func(tc *tomlConfig) {
				tc.OAuth.Microsoft = tomlOAuthProvider{ClientSecret: "microsoft-secret"}
			}, "oauth.microsoft"},
			{"github incomplete", func(tc *tomlConfig) {
				tc.OAuth.GitHub = tomlOAuthProvider{ClientID: "github-id"}
			}, "oauth.github"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := defaultConfig()
				tc.mutate(&cfg)
				_, err := resolveConfig(&cfg, cfgDir)
				if err == nil {
					t.Fatal("expected an error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not name %q", err, tc.wantErr)
				}
			})
		}
	})

	t.Run("unknown log level fails with the path", func(t *testing.T) {
		tc := defaultConfig()
		tc.Debug.LogLevel = "verbose"
		_, err := resolveConfig(&tc, cfgDir)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "debug.log_level") {
			t.Errorf("error %q does not name debug.log_level", err)
		}
	})

	t.Run("paths resolve against their documented base", func(t *testing.T) {
		tc := defaultConfig()
		tc.Server.DataDir = "./content"
		tc.Server.GeoDB = "GeoLite2-Country.mmdb"
		tc.GitHub.App = tomlGitHubApp{ID: 42, PrivateKeyFile: "github-app.pem", WebhookSecret: "secret"}
		got, err := resolveConfig(&tc, cfgDir)
		if err != nil {
			t.Fatal(err)
		}
		// data_dir stays relative to the working directory.
		if got.DataDir != "./content" {
			t.Errorf("got data_dir %q, want ./content", got.DataDir)
		}
		if want := filepath.Join(cfgDir, "GeoLite2-Country.mmdb"); got.GeoDB != want {
			t.Errorf("got geo_db %q, want %q", got.GeoDB, want)
		}
		wantApp := githubAppSettings{ID: 42, PrivateKeyFile: filepath.Join(cfgDir, "github-app.pem"), WebhookSecret: "secret"}
		if got.GitHubApp != wantApp {
			t.Errorf("got GitHub App %+v, want %+v", got.GitHubApp, wantApp)
		}
	})

	t.Run("absolute paths pass through", func(t *testing.T) {
		tc := defaultConfig()
		tc.Server.GeoDB = "/var/lib/mddb/GeoLite2-Country.mmdb"
		tc.GitHub.App = tomlGitHubApp{ID: 42, PrivateKeyFile: "/etc/mddb/github-app.pem"}
		got, err := resolveConfig(&tc, cfgDir)
		if err != nil {
			t.Fatal(err)
		}
		if got.GeoDB != tc.Server.GeoDB || got.GitHubApp.PrivateKeyFile != "/etc/mddb/github-app.pem" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("github app needs both id and private key file", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			app     tomlGitHubApp
			wantErr bool
		}{
			{"id without private key file", tomlGitHubApp{ID: 42}, true},
			{"private key file without id", tomlGitHubApp{PrivateKeyFile: "github-app.pem"}, true},
			{"disabled", tomlGitHubApp{}, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := defaultConfig()
				cfg.GitHub.App = tc.app
				_, err := resolveConfig(&cfg, cfgDir)
				if (err != nil) != tc.wantErr {
					t.Fatalf("got error %v for %+v, wantErr %t", err, tc.app, tc.wantErr)
				}
				if err != nil && !strings.Contains(err.Error(), "github.app") {
					t.Errorf("error %q does not name github.app", err)
				}
			})
		}
	})

	t.Run("voice api key passes through", func(t *testing.T) {
		tc := defaultConfig()
		tc.VoiceGateway.APIKey = "gemini-key"
		got, err := resolveConfig(&tc, cfgDir)
		if err != nil {
			t.Fatal(err)
		}
		if got.VoiceAPIKey != "gemini-key" {
			t.Errorf("got voice api key %q, want gemini-key", got.VoiceAPIKey)
		}
	})
}

func TestDefaultConfigDir(t *testing.T) {
	t.Run("XDG_CONFIG_HOME", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		if got, want := defaultConfigDir(), "/xdg/mddb"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("home fallback", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "/home/example")
		if got, want := defaultConfigDir(), "/home/example/.config/mddb"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
