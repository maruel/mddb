// Tests for TOML config loading, validation, and path resolution.
package main

import (
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
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

func TestLoadTOMLConfigTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgSubdir := filepath.Join(home, "custom")
	if err := os.MkdirAll(cfgSubdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgSubdir, "config.toml"), []byte("[server]\nhttp = \":9999\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadTOMLConfig("~/custom")
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.HTTP != ":9999" {
		t.Errorf("got http %q, want :9999", got.Server.HTTP)
	}
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
		if len(got.TrustedProxies) != 0 {
			t.Errorf("expected no trusted proxies: %v", got.TrustedProxies)
		}
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
		tc.Server.DataDir = "/srv/mddb"
		tc.Server.GeoDB = "/var/lib/mddb/GeoLite2-Country.mmdb"
		tc.GitHub.App = tomlGitHubApp{ID: 42, PrivateKeyFile: "/etc/mddb/github-app.pem"}
		got, err := resolveConfig(&tc, cfgDir)
		if err != nil {
			t.Fatal(err)
		}
		if got.DataDir != "/srv/mddb" || got.GeoDB != tc.Server.GeoDB || got.GitHubApp.PrivateKeyFile != "/etc/mddb/github-app.pem" {
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
		if want := filepath.Join(home, "GeoLite2-Country.mmdb"); got.GeoDB != want {
			t.Errorf("got geo_db %q, want %q", got.GeoDB, want)
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
		if got.GeoDB != home {
			t.Errorf("got geo_db %q, want %q", got.GeoDB, home)
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
