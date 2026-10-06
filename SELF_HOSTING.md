# Self-hosting and deployment instructions for mddb

## Installation / Building

As of January 2026, there is no binary releases yet, so you need the [Go toolchain](https://go.dev/dl).

```
go install github.com/maruel/mddb/backend/cmd/mddb@latest
```

## Configuration

Every startup setting lives in `~/.config/mddb/config.toml`, which mddb reads
once at startup. Pass `-config-dir DIR` to read the file from elsewhere; run
`mddb -help` for the flags. `contrib/config.toml` is the documented example and
lists every key with its default value. A malformed file or an unknown key stops
startup with the offending path.

Relative paths use one of two bases:

- `server.data_dir` resolves against the working directory, so the bundled
  systemd unit's `WorkingDirectory=%h/mddb` puts content in `~/mddb/data`.
- `server.geo_db` and `github.app.private_key_file` are configuration-side files
  and resolve against the config directory.

The file holds OAuth client secrets, the GitHub App webhook secret, and the
voice API key, so keep it readable only by the server account:

```bash
mkdir -p ~/.config/mddb
install -m 600 contrib/config.toml ~/.config/mddb/config.toml
nano ~/.config/mddb/config.toml
```

mddb writes its own `settings.json` in the same directory. It holds the
mutable settings you edit in the web UI (SMTP, quotas, rate limits) plus the
generated JWT secret and Web Push key pair, so mddb creates it mode 0600. The
data directory holds content only.

### Voice overlay

Set `voice-gateway.api_key` in `~/.config/mddb/config.toml` to a Gemini API key
to enable the embedded Gemini Live voice gateway. The discovery manifest
advertises the gateway only after it starts successfully, and the browser shows
the voice bar only when the manifest advertises one. If it stays hidden, check
`curl <base-url>/.well-known/gomode.json` (expect a non-empty
`webShell.voiceGateway.url`) and make sure any reverse proxy forwards
`/.well-known/` to mddb. Signaling uses the
mddb HTTP origin and requires a valid mddb bearer token. WebRTC media also
needs a reachable UDP port; the embedded gateway selects a free port at startup.
Voice startup failures leave the main server running without the overlay. The
gateway allows one active session per user, eight globally, and at most three
new offers per minute per user and twenty globally. When capacity is available,
a new offer replaces an existing session from the same user; sessions expire
after two hours. Transcript activity logs are stored in a private temporary
directory outside the git-backed data directory and removed on normal shutdown.

Enabling voice accepts sending speech off the device. The embedded backend is
Google Gemini Live: while a session is connected, microphone audio, the session
instructions, and the results of client-executed MCP tools are sent to Google,
and assistant audio is returned from Google. Without an API key the discovery
manifest does not advertise a gateway and no audio leaves the device. mddb does
not store voice audio.

Voice reuses the workspace MCP catalog at `/api/v1/gomode/mcp`, scoped to the
user's active workspace. Every member can list and read nodes and node
resources; editors additionally get `node_create`, `node_update`, and
`node_append`, and each edit is committed to the workspace git history.

To validate the live path (requires a voice API key and a reachable WebRTC UDP
port), run `make test-smoke-voice` with `GEMINI_API_KEY` in the environment: it
completes one Gemini voice turn, calls `nodes_list` through the workspace MCP
endpoint, and checks that hang-up releases session capacity.

### GeoLite

Get a .mmdb for free. You need to create af free account at https://www.maxmind.com/en/geolite2/signup

Select "GeoLite Country"

See the documentation at https://dev.maxmind.com/geoip/updating-databases

Set `server.geo_db` to the file path, relative to the config directory or
absolute. Leaving it empty disables IP geolocation.

## Authentication

### Google OAuth

Google OAuth works even if you only expose the server on localhost!

1. Create a Google Cloud project at https://console.cloud.google.com/
1. Go to API and services at https://console.cloud.google.com/apis/dashboard
1. Configure the OAuth interstitial branding at https://console.cloud.google.com/auth/branding
1. Create a OAuth Google Client ID and Google Client Secret for a web application at https://console.cloud.google.com/auth/clients
1. The callback URL (for tailscale) is `https://<hostname>.<tailnet>.ts.net/api/v1/auth/oauth/google/callback`
1. Set `oauth.google.client_id` and `oauth.google.client_secret` in `~/.config/mddb/config.toml`

### GitHub OAuth

GitHub OAuth requires an HTTPS URL, so you need to server over Tailscale or a reverse proxy like Caddy.

1. Go to OAuth Apps at https://github.com/settings/developers
1. Set as the Authorization callback URL `https://<hostname>.<tailnet>.ts.net/api/v1/auth/oauth/github/callback`
1. Set `oauth.github.client_id` and `oauth.github.client_secret` in `~/.config/mddb/config.toml`

### Microsoft OAuth

Microsoft OAuth is Microsoft Entra

1. https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/CreateApplicationBlade/quickStartType~/null/isMSAApp~/false
   1. Accounts in any organizational directory (Any Microsoft Entra ID tenant - Multitenant) and personal Microsoft accounts (e.g. Skype, Xbox)
   1. Redirect URL:
      1. Web
      1. `https://<hostname>.<tailnet>.ts.net/api/v1/auth/oauth/microsoft/callback`
1. Click "Add a certificate or secret"
1. New client secret
1. Duration: 730 days
1. `oauth.microsoft.client_id`: "Application (client) ID" — a UUID on the overview page
1. `oauth.microsoft.client_secret`: Client Secret = Certificates & secrets → Client secrets → the Value column (only
   visible right after creation, not the "Secret ID" column)
   Set both in `~/.config/mddb/config.toml`
1. Set branding
   1. https://<host>/terms
   1. https://<host>/privacy
1. Add yourself as owner
1. Validate a domain name with a `/.well-known/microsoft-identity-association.json` file
1. Token configuration ([Configure optional claims](https://learn.microsoft.com/entra/identity-platform/optional-claims))
   1. Add optional claim
   1. ID
   1. Check `email` and `xms_edov`
   1. Click Add
   1. Check "Turn on the Microsoft Graph email permission" then click Add.

Graph `mail` and `userPrincipalName` are unverified: a tenant admin can set `mail` to any address
([Migrate away from email claims](https://learn.microsoft.com/entra/identity-platform/migrate-off-email-claim-authorization)).
mddb therefore trusts only an ID token `email` whose `xms_edov` is `true`, meaning that the domain owner verified it,
as Microsoft personal accounts and tenant-verified domains are
([Optional claims reference](https://learn.microsoft.com/entra/identity-platform/optional-claims-reference)). Such an
email creates an account or signs in to the account that holds it. Without it, a Microsoft identity that no account
holds is refused with `EMAIL_NOT_VERIFIED`; the user signs in another way and links Microsoft from the profile
settings. Accounts that already have Microsoft linked sign in regardless.

## GitHub App (Live Sync)

A GitHub App enables bidirectional sync between workspaces and GitHub repositories. Edits auto-push to GitHub,
and GitHub pushes trigger automatic pulls via webhooks.

It requires your webserver to be accessible from the internet over HTTPS.

### Creating the GitHub App

1. Go to https://github.com/settings/apps/new
1. App name: e.g. "mddb-sync"
1. Homepage URL: your mddb instance URL
1. Webhook URL: `https://<host>/api/v1/webhooks/github`
1. `github.app.webhook_secret`: Webhook secret: generate a random string (e.g. `openssl rand -hex 32`)
1. Repository permissions:
   - Contents: Read & write
   - Metadata: Read-only
1. Subscribe to events: Push
1. Where can this GitHub App be installed?: "Only on this account" (or "Any account" for multi-org)
1. Click "Create GitHub App"
1. `github.app.id`: The App ID shown on the app settings page
1. On the App settings page, scroll to "Private keys"
1. `github.app.private_key_file`: Click "Generate a private key", save the `.pem` file, and set this to its path
   relative to the config directory (or an absolute path)

### Installing the GitHub App

1. Go to `https://github.com/apps/<app-name>/installations/new`
1. Select the account/organization and grant access to specific repositories
1. In mddb workspace settings, go to the Sync tab and click "GitHub App" to connect

### Outbound email via SMTP

- I personally use Maileroo but you can use any SMTP provider that support TLS.
- Once your mddb server is up and running, navigate to `https://<host>/settings/server` and enter the
  information there. Email will immediately start working (or if there's a bug left, restart the server).

## Running

### Running as a systemd Service

mddb is very resource light! If you plan to run on linux, you can take the absolute cheapest VM to run it.

A hardened systemd user service file is provided in [contrib//mddb.service](contrib//mddb.service):

```bash
# Install
mkdir -p ~/.config/systemd/user ~/.config/mddb
install -m 600 contrib/config.toml ~/.config/mddb/config.toml
cp contrib/mddb.service ~/.config/systemd/user/

# Edit the config: set server.http, server.base_url to the tailscale hostname,
# and the OAuth or GitHub App credentials you use.
nano ~/.config/mddb/config.toml

# Configure data directory (edit paths if needed)
mkdir -p ~/mddb/data

# Enable and start
systemctl --user daemon-reload
systemctl --user enable --now mddb

# View logs
journalctl --user -u mddb -f
```

### Running on macOS

To describe later. Ask "how to run a program via launchd"

### Running on Windows

To describe later. Ask "how to run a service on windows"

## Serving over the web

By default, mddb listens to localhost on port 8080. Set `server.http` in `~/.config/mddb/config.toml` to change this, e.g. `"0.0.0.0:8080"` to listen on all interfaces.

## Serving over Tailscale

Safely expose mddb on your [Tailscale](https://tailscale.com/) network using `tailscale serve`. This provides
secure access from any device on your tailnet without opening ports or configuring firewalls.

```bash
# Expose mddb on your tailnet at https://<hostname>.<tailnet>.ts.net
tailscale serve --bg 8080
```

For public access via Tailscale Funnel (exposes to the internet!):

```bash
# Make mddb publicly accessible at https://<hostname>.<tailnet>.ts.net
tailscale funnel --bg 8080
```

**HTTPS**: Tailscale serve/funnel provides HTTPS automatically via Let's Encrypt TLS certificates.

### Reverse Proxy with Caddy

A sample Caddyfile is provided in [contrib/mddb.caddyfile](contrib/mddb.caddyfile) for running mddb behind
[Caddy](https://caddyserver.com/).

**HTTPS**: Caddy provides HTTPS automatically via Let's Encrypt TLS certificates.

**Client IP**: Set `server.trusted_proxies = ["127.0.0.1/32", "::1/128"]` in `config.toml`. Without it, mddb
ignores `X-Forwarded-For` and `X-Real-IP`, and every client shares Caddy's address for rate limits.
