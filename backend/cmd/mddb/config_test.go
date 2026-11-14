// Tests for TOML config loading, validation, and path resolution.
package main

import (
	"bytes"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maruel/gomode/httpserver/geoip"
	"github.com/maruel/gomode/voicegateway"
	"github.com/maruel/mddb/backend/internal/server"
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
// defaults outside its explicit, disabled voice pipeline examples. This guards
// the shipped template on three axes at once:
//   - faithfulness: loadTOMLConfig uses DisallowUnknownFields, so an
//     undocumented or misspelled key fails the load;
//   - accuracy: host defaults match defaultConfig() and voice pipelines validate;
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
	if got.VoiceGateway.Mode != voiceGatewayDisabled {
		t.Fatal("shipped example enables voice")
	}
	if _, err := got.VoiceGateway.Config.Resolve(); err != nil {
		t.Fatalf("shipped voice pipelines: %v", err)
	}
	got.VoiceGateway.Config = voicegateway.Config{}
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
auto_update = "0 3 * * 6"
[server]
http = ":9090"
data_dir = "/srv/mddb"
base_url = "https://mddb.example.com"
geo_db = "GeoLite2-Country.mmdb"
allow_origins = ["CA", "local", "192.0.2.0/24"]
trusted_proxies = ["127.0.0.1/32", "::1/128"]

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
mode = "embedded"

[debug]
log_level = "debug"
`)
		got, err := loadTOMLConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := tomlConfig{
			AutoUpdate: "0 3 * * 6",
			Server: tomlServer{
				HTTP:         ":9090",
				DataDir:      "/srv/mddb",
				BaseURL:      "https://mddb.example.com",
				GeoDB:        "GeoLite2-Country.mmdb",
				AllowOrigins: []string{"CA", "local", "192.0.2.0/24"},

				TrustedProxies: []string{"127.0.0.1/32", "::1/128"},
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
			VoiceGateway: tomlVoiceGateway{Mode: voiceGatewayEmbedded, TokenMode: server.VoiceTokenModeScoped},
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
	t.Run("automatic updates", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			for _, expr := range []string{"50 4 * * *", "0 3 * * 6", ""} {
				t.Run(expr, func(t *testing.T) {
					dir := t.TempDir()
					if err := os.WriteFile(configPath(dir), []byte("auto_update = "+strconv.Quote(expr)), 0o600); err != nil {
						t.Fatal(err)
					}
					tc, err := loadTOMLConfig(dir)
					if err != nil {
						t.Fatal(err)
					}
					cfg, err := resolveConfig(&tc, dir)
					if err != nil {
						t.Fatal(err)
					}
					if expr == "" {
						if cfg.AutoUpdate != nil {
							t.Fatal("empty schedule enabled updates")
						}
						return
					}
					if cfg.AutoUpdate == nil {
						t.Fatal("schedule disabled updates")
					}
					loc := time.FixedZone("local", -5*60*60)
					now := time.Date(2026, 11, 13, 23, 0, 0, 0, loc)
					want := time.Date(2026, 11, 14, 4, 50, 0, 0, loc)
					if expr == "0 3 * * 6" {
						want = time.Date(2026, 11, 14, 3, 0, 0, 0, loc)
					}
					if got := cfg.AutoUpdate.Next(now); !got.Equal(want) || got.Location() != loc {
						t.Fatalf("next update = %v, want %v", got, want)
					}
				})
			}
		})
		t.Run("error", func(t *testing.T) {
			for _, expr := range []string{"nightly", "60 4 * * *", "0 0 31 2 *"} {
				tc := defaultConfig()
				tc.AutoUpdate = expr
				if _, err := resolveConfig(&tc, cfgDir); err == nil || !strings.Contains(err.Error(), "auto_update:") {
					t.Fatalf("schedule %q: %v", expr, err)
				}
			}
		})
	})

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
		if got.GeoIP.DB != "" || got.VoiceMode != voiceGatewayDisabled {
			t.Errorf("expected geo_db and voice to be off: %+v", got)
		}
		if got.GitHubApp != (githubAppSettings{}) {
			t.Errorf("expected no GitHub App: %+v", got)
		}
		if len(got.TrustedProxies) != 0 {
			t.Errorf("expected no trusted proxies: %v", got.TrustedProxies)
		}
		if got.AutoUpdate == nil || tc.AutoUpdate != "50 4 * * *" {
			t.Fatalf("default update schedule = %q, %v", tc.AutoUpdate, got.AutoUpdate)
		}
	})

	t.Run("IP origins", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			for _, origins := range [][]string{
				{},
				{"local", "tailscale", "anthropic", "192.0.2.0/24", "::1/128"},
				{"github", "openai"},
			} {
				tc := defaultConfig()
				tc.Server.AllowOrigins = origins
				got, err := resolveConfig(&tc, cfgDir)
				if err != nil {
					t.Fatal(err)
				}
				if got.GeoIP.DB != "" || !slices.Equal(got.GeoIP.AllowOrigins, origins) {
					t.Fatalf("IP origins = %+v", got.GeoIP)
				}
			}
			tc := defaultConfig()
			tc.Server.GeoDB = "country.mmdb"
			// Country presence is checked when opening the database, not during parsing.
			tc.Server.AllowOrigins = []string{"ca", "US", "BU", "AN", "EZ", "ZZ"}
			got, err := resolveConfig(&tc, cfgDir)
			if err != nil {
				t.Fatal(err)
			}
			if got.GeoIP.DB != filepath.Join(cfgDir, "country.mmdb") || !slices.Equal(got.GeoIP.AllowOrigins, tc.Server.AllowOrigins) {
				t.Fatalf("country origins = %+v", got.GeoIP)
			}
		})
		t.Run("error", func(t *testing.T) {
			for _, token := range []string{"", "US", "1A", "é", "Atlantis", "192.0.2.0/99"} {
				t.Run(token, func(t *testing.T) {
					tc := defaultConfig()
					tc.Server.AllowOrigins = []string{token}
					if token != "US" {
						tc.Server.GeoDB = "country.mmdb"
					}
					_, err := resolveConfig(&tc, cfgDir)
					if err == nil || !strings.Contains(err.Error(), "server.") {
						t.Fatalf("origin %q: %v", token, err)
					}
				})
			}
			tc := defaultConfig()
			tc.Server.GeoDB = "   "
			if _, err := resolveConfig(&tc, cfgDir); err == nil || !strings.Contains(err.Error(), "server.geo_db") {
				t.Fatalf("whitespace DB: %v", err)
			}
		})
	})

	t.Run("trusted proxies", func(t *testing.T) {
		tc := defaultConfig()
		tc.Server.TrustedProxies = []string{"127.0.0.1/32", "10.1.2.3/8"}
		got, err := resolveConfig(&tc, cfgDir)
		if err != nil {
			t.Fatal(err)
		}
		want := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.0.0.0/8")}
		if !slices.Equal(got.TrustedProxies, want) {
			t.Errorf("got %v, want %v", got.TrustedProxies, want)
		}
	})

	t.Run("invalid trusted proxy fails with the path", func(t *testing.T) {
		for _, raw := range []string{"127.0.0.1", "proxy.example.com/24", ""} {
			tc := defaultConfig()
			tc.Server.TrustedProxies = []string{raw}
			_, err := resolveConfig(&tc, cfgDir)
			if err == nil {
				t.Fatalf("expected an error for %q", raw)
			}
			if !strings.Contains(err.Error(), "server.trusted_proxies") {
				t.Errorf("error %q does not name server.trusted_proxies", err)
			}
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
		if want := filepath.Join(cfgDir, "GeoLite2-Country.mmdb"); got.GeoIP.DB != want {
			t.Errorf("got geo_db %q, want %q", got.GeoIP.DB, want)
		}
		wantApp := githubAppSettings{ID: 42, PrivateKeyFile: filepath.Join(cfgDir, "github-app.pem"), WebhookSecret: "secret"}
		if got.GitHubApp != wantApp {
			t.Errorf("got GitHub App %+v, want %+v", got.GitHubApp, wantApp)
		}
	})

	t.Run("absolute paths pass through", func(t *testing.T) {
		tc := defaultConfig()
		tc.Server.DataDir = "/srv/mddb"
		tc.Server.GeoDB = "/var/lib/mddb/GeoLite2-Country.mmdb"
		tc.GitHub.App = tomlGitHubApp{ID: 42, PrivateKeyFile: "/etc/mddb/github-app.pem"}
		got, err := resolveConfig(&tc, cfgDir)
		if err != nil {
			t.Fatal(err)
		}
		if got.DataDir != "/srv/mddb" || got.GeoIP.DB != tc.Server.GeoDB || got.GitHubApp.PrivateKeyFile != "/etc/mddb/github-app.pem" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("tilde expands to home directory", func(t *testing.T) {
		home := "/home/tester"
		t.Setenv("HOME", home)

		tc := defaultConfig()
		tc.Server.DataDir = "~/content"
		tc.Server.GeoDB = "~/GeoLite2-Country.mmdb"
		tc.GitHub.App = tomlGitHubApp{ID: 42, PrivateKeyFile: "~/github-app.pem"}
		got, err := resolveConfig(&tc, "~/custom-cfg")
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, "custom-cfg"); got.ConfigDir != want {
			t.Errorf("got ConfigDir %q, want %q", got.ConfigDir, want)
		}
		if want := filepath.Join(home, "content"); got.DataDir != want {
			t.Errorf("got data_dir %q, want %q", got.DataDir, want)
		}
		if want := filepath.Join(home, "GeoLite2-Country.mmdb"); got.GeoIP.DB != want {
			t.Errorf("got geo_db %q, want %q", got.GeoIP.DB, want)
		}
		if want := filepath.Join(home, "github-app.pem"); got.GitHubApp.PrivateKeyFile != want {
			t.Errorf("got private_key_file %q, want %q", got.GitHubApp.PrivateKeyFile, want)
		}
	})

	t.Run("tilde alone expands to home directory", func(t *testing.T) {
		home := "/home/tester"
		t.Setenv("HOME", home)

		tc := defaultConfig()
		tc.Server.DataDir = "~"
		tc.Server.GeoDB = "~"
		tc.GitHub.App = tomlGitHubApp{ID: 42, PrivateKeyFile: "~"}
		got, err := resolveConfig(&tc, "~")
		if err != nil {
			t.Fatal(err)
		}
		if got.ConfigDir != home {
			t.Errorf("got ConfigDir %q, want %q", got.ConfigDir, home)
		}
		if got.DataDir != home {
			t.Errorf("got data_dir %q, want %q", got.DataDir, home)
		}
		if got.GeoIP.DB != home {
			t.Errorf("got geo_db %q, want %q", got.GeoIP.DB, home)
		}
		if got.GitHubApp.PrivateKeyFile != home {
			t.Errorf("got private_key_file %q, want %q", got.GitHubApp.PrivateKeyFile, home)
		}
	})

	t.Run("home resolution error fails with the path", func(t *testing.T) {
		t.Setenv("HOME", "")

		for _, tc := range []struct {
			name    string
			mutate  func(*tomlConfig)
			cfgDir  string
			wantErr string
		}{
			{
				name: "data_dir with tilde",
				mutate: func(tc *tomlConfig) {
					tc.Server.DataDir = "~/data"
				},
				cfgDir:  cfgDir,
				wantErr: "server.data_dir",
			},
			{
				name: "geo_db with tilde",
				mutate: func(tc *tomlConfig) {
					tc.Server.GeoDB = "~/geo.mmdb"
				},
				cfgDir:  cfgDir,
				wantErr: "server.geo_db",
			},
			{
				name: "github app private key with tilde",
				mutate: func(tc *tomlConfig) {
					tc.GitHub.App = tomlGitHubApp{ID: 42, PrivateKeyFile: "~/key.pem"}
				},
				cfgDir:  cfgDir,
				wantErr: "github.app.private_key_file",
			},
			{
				name:    "cfgDir with tilde",
				mutate:  func(tc *tomlConfig) {},
				cfgDir:  "~/config",
				wantErr: "config_dir",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := defaultConfig()
				tc.mutate(&cfg)
				_, err := resolveConfig(&cfg, tc.cfgDir)
				if err == nil {
					t.Fatal("expected an error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not name %q", err, tc.wantErr)
				}
			})
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

func TestPathResolutionHelpers(t *testing.T) {
	home := "/home/tester"
	t.Setenv("HOME", home)

	t.Run("expandHome", func(t *testing.T) {
		got, err := expandHome("")
		if err != nil || got != "" {
			t.Fatalf("expandHome(\"\") = (%q, %v), want (\"\", nil)", got, err)
		}
		got, err = expandHome("~")
		if err != nil || got != home {
			t.Fatalf("expandHome(\"~\") = (%q, %v), want (%q, nil)", got, err, home)
		}
		got, err = expandHome("~/abc/def")
		if err != nil || got != filepath.Join(home, "abc", "def") {
			t.Fatalf("expandHome(\"~/abc/def\") = (%q, %v)", got, err)
		}
		got, err = expandHome(`~\abc\def`)
		if err != nil || got != filepath.Join(home, "abc", "def") {
			t.Fatalf("expandHome(`~\\abc\\def`) = (%q, %v)", got, err)
		}
		got, err = expandHome("relative/path")
		if err != nil || got != "relative/path" {
			t.Fatalf("expandHome(\"relative/path\") = (%q, %v)", got, err)
		}
		got, err = expandHome("/absolute/path")
		if err != nil || got != "/absolute/path" {
			t.Fatalf("expandHome(\"/absolute/path\") = (%q, %v)", got, err)
		}
	})

	t.Run("resolveDataDir", func(t *testing.T) {
		got, err := resolveDataDir("")
		if err != nil || got != "" {
			t.Fatalf("resolveDataDir(\"\") = (%q, %v)", got, err)
		}
		got, err = resolveDataDir("data")
		if err != nil || got != "data" {
			t.Fatalf("resolveDataDir(\"data\") = (%q, %v)", got, err)
		}
		got, err = resolveDataDir("/srv/data")
		if err != nil || got != "/srv/data" {
			t.Fatalf("resolveDataDir(\"/srv/data\") = (%q, %v)", got, err)
		}
		got, err = resolveDataDir("~/data")
		if err != nil || got != filepath.Join(home, "data") {
			t.Fatalf("resolveDataDir(\"~/data\") = (%q, %v)", got, err)
		}
		got, err = resolveDataDir("~")
		if err != nil || got != home {
			t.Fatalf("resolveDataDir(\"~\") = (%q, %v)", got, err)
		}
	})

	t.Run("resolveConfigPath", func(t *testing.T) {
		cfgDir := "/etc/mddb"
		got, err := resolveConfigPath("", cfgDir)
		if err != nil || got != "" {
			t.Fatalf("resolveConfigPath(\"\") = (%q, %v)", got, err)
		}
		got, err = resolveConfigPath("geo.mmdb", cfgDir)
		if err != nil || got != filepath.Join(cfgDir, "geo.mmdb") {
			t.Fatalf("resolveConfigPath(\"geo.mmdb\") = (%q, %v)", got, err)
		}
		got, err = resolveConfigPath("/var/geo.mmdb", cfgDir)
		if err != nil || got != "/var/geo.mmdb" {
			t.Fatalf("resolveConfigPath(\"/var/geo.mmdb\") = (%q, %v)", got, err)
		}
		got, err = resolveConfigPath("~/geo.mmdb", cfgDir)
		if err != nil || got != filepath.Join(home, "geo.mmdb") {
			t.Fatalf("resolveConfigPath(\"~/geo.mmdb\") = (%q, %v)", got, err)
		}
		got, err = resolveConfigPath("~", cfgDir)
		if err != nil || got != home {
			t.Fatalf("resolveConfigPath(\"~\") = (%q, %v)", got, err)
		}
	})
}

func TestVoiceGatewayConfiguration(t *testing.T) {
	t.Parallel()
	t.Run("external OAuth signing identity", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`[server]
base_url = "https://mddb.example.com"
[voice-gateway]
mode = "external"
url = "https://voice.example.com"
instance_id = "mddb-main"
token_mode = "oauth"
`), 0o600); err != nil {
			t.Fatal(err)
		}
		tc, err := loadTOMLConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := resolveConfig(&tc, dir)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ExternalVoice == nil || cfg.VoiceMode != voiceGatewayExternal || cfg.ExternalVoice.InstanceID != "mddb-main" {
			t.Fatalf("config: %+v", cfg)
		}
		first, err := os.ReadFile(filepath.Join(dir, "voice-oauth-key.pem")) //nolint:gosec // Test-owned temporary signing key.
		if err != nil {
			t.Fatal(err)
		}
		if _, err := resolveConfig(&tc, dir); err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(dir, "voice-oauth-key.pem")) //nolint:gosec // Test-owned temporary signing key.
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatal("OAuth signing identity changed on restart")
		}
		info, err := os.Stat(filepath.Join(dir, "voice-oauth-key.pem"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("private key permissions: %v", info.Mode())
		}
	})
	t.Run("explicit embedded pipelines", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`[voice-gateway]
mode = "embedded"
webrtc_udp_port = 3478
[[voice-gateway.config.realtime]]
id = "gemini-live"
label = "Gemini Live"
provider = "gemini"
model = "gemini-3.8-live"
[[voice-gateway.config.llm]]
id = "local_text"
label = "Local text"
provider = "llamacpp"
remote = "http://localhost:8080"
model = "gemma"
[[voice-gateway.config.llm]]
id = "local_stack"
label = "Local stack"
provider = "llamacpp"
llamacppsrv = true
model = "gemma-local"
context_size = 32768
[[voice-gateway.config.voice]]
id = "local-stack-preset"
label = "Local stack preset"
asr = "remote"
llm = "local_stack"
tts = "remote"
[[voice-gateway.config.asr]]
id = "remote"
engine = "openai-audio"
remote = "http://localhost:8000"
model = "whisper"
[[voice-gateway.config.asr]]
id = "alternate"
engine = "openai-audio"
remote = "http://localhost:8002"
model = "alternate-asr"
[[voice-gateway.config.tts]]
id = "remote"
engine = "openai-audio"
remote = "http://localhost:8001"
model = "tts"
voice = "voice-name"
[[voice-gateway.config.tts]]
id = "alternate"
engine = "openai-audio"
remote = "http://localhost:8003"
model = "alternate-tts"
voice = "alternate-voice"
`), 0o600); err != nil {
			t.Fatal(err)
		}
		tc, err := loadTOMLConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := resolveConfig(&tc, dir)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ExternalVoice != nil || cfg.VoiceMode != voiceGatewayEmbedded || len(cfg.VoiceConfig.Realtime) != 1 || len(cfg.VoiceConfig.LLM) != 2 || len(cfg.VoiceConfig.Pipelines) != 1 {
			t.Fatalf("config: %+v", cfg)
		}
		if cfg.VoiceUDPPort != 3478 || len(cfg.VoiceConfig.Recognition) != 2 || len(cfg.VoiceConfig.Synthesis) != 2 || cfg.VoiceConfig.Pipelines[0].ASR.Model != "whisper" || cfg.VoiceConfig.Pipelines[0].TTS.Voice != "voice-name" || cfg.VoiceConfig.Recognition[0].Model != "whisper" || cfg.VoiceConfig.Recognition[1].Model != "alternate-asr" || cfg.VoiceConfig.Synthesis[0].Model != "tts" || cfg.VoiceConfig.Synthesis[1].Model != "alternate-tts" {
			t.Fatalf("speech stages or listener settings lost: %+v", cfg)
		}
		if cfg.VoiceConfig.Realtime[0].Provider != "gemini" || cfg.VoiceConfig.LLM[1].Model != "gemma-local" || cfg.VoiceConfig.LLM[0].Remote != "http://localhost:8080" || !cfg.VoiceConfig.Pipelines[0].LLM.LlamaCPPServer {
			t.Fatalf("pipeline settings lost: %+v", cfg.VoiceConfig)
		}
	})
	t.Run("invalid external", func(t *testing.T) {
		for _, tc := range []struct{ name, url, issuer, id, mode, key string }{
			{"missing URL", "", "https://mddb.example.com", "main", "oauth", ""},
			{"insecure gateway", "http://voice.example.com", "https://mddb.example.com", "main", "oauth", ""},
			{"insecure issuer", "https://voice.example.com", "http://mddb.example.com", "main", "oauth", ""},
			{"URL credentials", "https://user:pass@voice.example.com", "https://mddb.example.com", "main", "oauth", ""},
			{"URL query", "https://voice.example.com?key=secret", "https://mddb.example.com", "main", "oauth", ""},
			{"URL path", "https://voice.example.com/path", "https://mddb.example.com", "main", "oauth", ""},
			{"missing ID", "https://voice.example.com", "https://mddb.example.com", "", "oauth", ""},
			{"missing key", "https://voice.example.com", "https://mddb.example.com", "main", "scoped", ""},
			{"unknown token mode", "https://voice.example.com", "https://mddb.example.com", "main", "unknown", ""},
			{"unused key", "https://voice.example.com", "https://mddb.example.com", "main", "oauth", "key.pem"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				c := defaultConfig()
				c.Server.BaseURL = tc.issuer
				c.VoiceGateway.Mode = voiceGatewayExternal
				c.VoiceGateway.URL = tc.url
				c.VoiceGateway.InstanceID = tc.id
				c.VoiceGateway.TokenMode = server.VoiceTokenMode(tc.mode)
				c.VoiceGateway.SigningPrivateKeyPEM = tc.key
				if _, err := resolveConfig(&c, t.TempDir()); err == nil {
					t.Fatal("invalid config accepted")
				}
			})
		}
	})
	t.Run("invalid embedded", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			config voicegateway.Config
		}{
			{"missing pipelines", voicegateway.Config{}},
			{"missing model", voicegateway.Config{LLM: []voicegateway.LLMConfig{{ID: "model", Label: "Model", ModelConfig: voicegateway.ModelConfig{Provider: "gemini"}}}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				c := defaultConfig()
				c.VoiceGateway.Mode = voiceGatewayEmbedded
				c.VoiceGateway.Config = tc.config
				if _, err := resolveConfig(&c, t.TempDir()); err == nil {
					t.Fatal("invalid config accepted")
				}
			})
		}
	})
	t.Run("embedded UDP port range", func(t *testing.T) {
		for _, port := range []int{-1, 65536} {
			c := defaultConfig()
			c.VoiceGateway.Mode = voiceGatewayEmbedded
			c.VoiceGateway.WebRTCUDPPort = port
			c.VoiceGateway.Config.Realtime = []voicegateway.RealtimeConfig{{ID: "live", Label: "Live", ModelConfig: voicegateway.ModelConfig{Provider: "gemini", Model: "gemini-3.8-live"}}}
			if _, err := resolveConfig(&c, t.TempDir()); err == nil {
				t.Fatalf("invalid UDP port %d accepted", port)
			}
		}
	})
	t.Run("unknown mode", func(t *testing.T) {
		c := defaultConfig()
		c.VoiceGateway.Mode = "unknown"
		if _, err := resolveConfig(&c, t.TempDir()); err == nil {
			t.Fatal("unknown mode accepted")
		}
	})
	t.Run("dormant declarations do not enable voice", func(t *testing.T) {
		c := defaultConfig()
		c.VoiceGateway.Config.LLM = []voicegateway.LLMConfig{{ID: "preconfigured", Label: "Preconfigured", ModelConfig: voicegateway.ModelConfig{Provider: "gemini", Model: "gemini-3.8-flash"}}}
		cfg, err := resolveConfig(&c, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceMode != voiceGatewayDisabled || cfg.ExternalVoice != nil {
			t.Fatal("configured pipeline enabled disabled gateway")
		}
	})
}

func TestVoiceGatewayEnvironmentIsolation(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")
	tc := defaultConfig()
	cfg, err := resolveConfig(&tc, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VoiceMode != voiceGatewayDisabled || cfg.ExternalVoice != nil || len(cfg.VoiceConfig.Realtime)+len(cfg.VoiceConfig.LLM)+len(cfg.VoiceConfig.Pipelines) != 0 {
		t.Fatal("provider credentials enabled voice or supplied implicit pipelines")
	}
}

func TestResolvedConfig(t *testing.T) {
	t.Run("openGeoIP", func(t *testing.T) {
		// Disabled lookup and database-only enrichment need no user cache.
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("LocalAppData", "")
		t.Setenv("home", "")
		log := slog.New(slog.DiscardHandler)
		t.Run("disabled", func(t *testing.T) {
			c := resolvedConfig{}
			checker, err := c.openGeoIP(t.Context(), log)
			if err != nil || checker != nil {
				t.Fatalf("disabled: checker=%v err=%v", checker, err)
			}
		})
		t.Run("local filter", func(t *testing.T) {
			cacheDir := t.TempDir()
			for _, key := range []string{"HOME", "LocalAppData", "XDG_CACHE_HOME", "home"} {
				t.Setenv(key, cacheDir)
			}
			c := resolvedConfig{GeoIP: geoip.Config{AllowOrigins: []string{"local", "203.0.113.0/24"}}}
			checker, err := c.openGeoIP(t.Context(), log)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = checker.Close() })
			if origin, allowed := checker.CheckOrigin("127.0.0.1"); origin != "local" || !allowed {
				t.Fatalf("local: origin=%q allowed=%v", origin, allowed)
			}
			if _, allowed := checker.CheckOrigin("8.8.8.8"); allowed {
				t.Fatal("filter allowed an unknown origin")
			}
		})
		t.Run("DB error precedes cache", func(t *testing.T) {
			c := resolvedConfig{GeoIP: geoip.Config{DB: filepath.Join(t.TempDir(), "missing.mmdb")}}
			if _, err := c.openGeoIP(t.Context(), log); err == nil || !strings.Contains(err.Error(), "geo_db") {
				t.Fatalf("DB-only without cache location: %v", err)
			}
		})
	})
}
