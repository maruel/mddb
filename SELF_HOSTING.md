# Self-hosting and deployment instructions for mddb

## Installation / Building

As of January 2026, there is no binary releases yet, so you need the [Go toolchain](https://go.dev/dl).

```
go install github.com/maruel/mddb/backend/cmd/mddb@latest
```

## Configuration

Run `mddb -help` for a full list of configuration options.

## Authentication

### Google OAuth

Google OAuth works even if you only expose the server on localhost!

1. Create a Google Cloud project at https://console.cloud.google.com/
1. Go to API and services at https://console.cloud.google.com/apis/dashboard
1. Configure the OAuth interstitial branding at https://console.cloud.google.com/auth/branding
1. Create a OAuth Google Client ID and Google Client Secret for a web application at https://console.cloud.google.com/auth/clients
1. The callback URL (for tailscale) is `https://<hostname>.<tailnet>.ts.net/api/auth/google/callback`

### GitHub Oauth

GitHub OAuth requires an HTTPS URL, so you need to server over Tailscale or a reverse proxy like Caddy.

1. Go to OAuth Apps at https://github.com/settings/developers
1. Set as the Authorization callback URL `https://<hostname>.<tailnet>.ts.net/api/auth/github/callback`

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
mkdir -p ~/.config/systemd/user
cp contrib/mddb.service ~/.config/systemd/user/

# Edit as needed. In particular, add the tailscale hostname and change the port if it conflicts on your system.
nano ~/.config/systemd/user/mddb.service

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

By default, mddb listens to localhost on port 8080. Use the `-http` flag to change this, e.g., `-http 0.0.0.0:8080` to listen on all interfaces.

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
