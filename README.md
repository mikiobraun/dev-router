# dev-router

[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/mikiobraun/dev-router)](https://goreportcard.com/report/github.com/mikiobraun/dev-router)

Tired of remembering on which port your project's dev server lives?

Automatic subdomain routing for local dev services.

Scans your projects directory for Git repos with a `dev.yaml` config, then generates a Caddy reverse proxy configuration mapping `{project}.dev.{domain}` to `localhost:{port}`.

Creates dev certificates with mkcert. Works great with tailscale or other VPN solutions.

## Installation

```bash
go build -o dev-router .
cp dev-router ~/.local/bin/
```

## Usage

```bash
dev-router list                 # Show discovered services
dev-router generate             # Generate Caddyfile
dev-router generate --reload    # Generate and reload Caddy
```

## Configuration

### Global config: `~/.config/dev-router/config.yaml`

```yaml
domain: dev.yourdomain.com
projects_dir: ~/Projects
cert_path: /etc/caddy/certs/_wildcard.dev.yourdomain.com.pem
key_path: /etc/caddy/certs/_wildcard.dev.yourdomain.com-key.pem
caddyfile_path: ~/.config/caddy/Caddyfile.dev
auth_upstream: localhost:6100   # optional, see Authentication
```

### Project config: `dev.yaml`

Add to any Git repo root. Single service:

```yaml
port: 3000
name: myapp    # optional, defaults to directory name
enabled: true  # optional, defaults to true
auth: false    # optional, defaults to false; see Authentication
```

Multiple services (e.g., frontend + API):

```yaml
services:
  - name: myapp-api
    port: 3000
  - name: myapp-frontend
    port: 5173
```

Each service gets its own subdomain (`myapp-api.dev.yourdomain.com`, etc.).

## Authentication

Any service can be put behind a shared auth gateway by setting `auth: true` in
its `dev.yaml`:

```yaml
port: 3000
auth: true
```

```yaml
services:
  - name: myapp-api
    port: 3000
    auth: true
  - name: myapp-frontend
    port: 5173
```

This requires `auth_upstream` in the global config — the `host:port` of an auth
service you run yourself. dev-router does not provide one; it only wires up the
Caddy configuration.

`dev-router list` shows an `AUTH` column so you can see at a glance which
services are gated.

### What gets generated

For a service with `auth: true`, the site block splits in two:

```
myapp.dev.yourdomain.com {
	tls ...
	handle /.well-known/oauth-protected-resource {
		reverse_proxy localhost:6100
	}
	handle {
		forward_auth localhost:6100 {
			uri /verify
			copy_headers X-Volume-User X-Volume-Scopes
		}
		reverse_proxy localhost:3000
	}
}
```

Everything except OAuth discovery is gated. Discovery stays public because a
client has to be able to find out what it is authenticating against *before* it
has a token, and it is proxied per-service rather than hitting the auth host
directly so that the resource is identified by the service's own hostname.

If `auth: true` is set but `auth_upstream` is not configured, dev-router does
not fail — it emits a visible `# WARNING` comment into the Caddyfile and serves
the service unprotected, so the mistake is loud rather than silent.

### What the auth service must provide

dev-router hardcodes the contract below. An auth service is compatible if it:

1. **Answers `/verify` on any HTTP method.** Caddy forwards the original
   request's method, path and headers, along with `X-Forwarded-Host`,
   `X-Forwarded-Proto` and `X-Forwarded-Uri`.

2. **Returns `2xx` to allow, anything else to deny.** On a `2xx` it should set
   `X-Volume-User` and `X-Volume-Scopes`; those are the two headers Caddy copies
   onto the upstream request, and they are how the protected service learns who
   is calling. A denial is typically `401` for API clients and a `302` to a login
   page for browsers.

3. **Serves `/.well-known/oauth-protected-resource`**, identifying the resource
   by the incoming `X-Forwarded-Host` rather than by its own hostname — one auth
   service fronts many gated services, and each must describe itself correctly.

The endpoint path and the two header names are fixed in the generator
(`internal/generator/generator.go`); change them there if your auth service uses
different ones.

### Unreachable upstreams

Every generated site block ends with a `handle_errors 502 503` block that
responds with a readable message when an upstream cannot be reached — a stopped
dev server, or an auth service that is not running. Note the second case: if
`auth_upstream` is down, *all* gated services return 503, including their
discovery route. Errors produced by the application itself pass through
untouched.

## CORS

A service called from a browser page on another origin (a SPA talking to its
API) needs a `cors` block. dev-router reflects only allowlisted origins, and
answers the preflight inside the `route` *before* `forward_auth` — otherwise the
unauthenticated `OPTIONS` gets redirected to login and the real request never
happens.

```yaml
name: kbmcp
port: 8070
auth: true
cors:
  origins: [https://editor.yourdomain.com]   # "*" allows any origin
  expose: [ETag, WWW-Authenticate]           # response headers JS may read
  headers: [Authorization, Content-Type, If-Match, If-None-Match]  # optional
```

generates, inside the vhost's `route`:

```
@cors_o0 header Origin https://editor.yourdomain.com
header @cors_o0 {
	Access-Control-Allow-Origin "https://editor.yourdomain.com"
	Access-Control-Expose-Headers "ETag, WWW-Authenticate"
	Vary Origin
	defer
}
@preflight method OPTIONS
handle @preflight {
	header Access-Control-Allow-Methods "GET, PUT, POST, DELETE, OPTIONS"
	header Access-Control-Allow-Headers "Authorization, Content-Type, If-Match, If-None-Match"
	header Access-Control-Max-Age "600"
	respond 204
}
```

`headers` sets `Access-Control-Allow-Headers` — the request headers the browser
is allowed to send. It is optional and **replaces** the default list shown above
(`Authorization, Content-Type, If-Match, If-None-Match`) rather than extending
it, so include the ones you still need. Set it when a client sends anything else
(`X-Request-Id`, a custom auth header): a header the preflight doesn't advertise
is one the browser silently refuses to send, and the failure surfaces in the
console as "Request header field … is not allowed by
Access-Control-Allow-Headers" rather than as an error from your service.

`expose` is the mirror image — response headers the page's JS is allowed to
*read*. `ETag` belongs there for any API doing `If-Match` optimistic concurrency,
since the client can't send back a validator it was never allowed to see.

## Rate limiting

Any service can have a `rate_limit` block, which emits a
[caddy-ratelimit](https://github.com/mholt/caddy-ratelimit) handler. This is most
useful on an internet-facing auth service, to throttle credential-guessing on
`/login` and client-registration floods on `/register`:

```yaml
name: volume-auth
port: 6100
domain: auth.yourdomain.com
rate_limit:
  paths: [/login, /register]   # optional; omit to limit the whole vhost
  events: 20                    # allowed requests per window (required)
  window: 1m                    # window duration (required)
  key: "{remote_host}"          # optional; defaults to per-client IP
```

generates:

```
auth.yourdomain.com {
	route {
		@volume_auth_rl path /login /register
		rate_limit @volume_auth_rl {
			zone volume_auth {
				key {remote_host}
				events 20
				window 1m
			}
		}
		reverse_proxy localhost:6100
	}
	handle_errors 502 503 { ... }
}
```

**Requires the `caddy-ratelimit` module compiled into Caddy** — it is not part of
core Caddy. On Debian: `sudo caddy add-package github.com/mholt/caddy-ratelimit`
then restart Caddy (or use `xcaddy` / a custom build). A `rate_limit` missing
`events` or `window` is skipped with a warning rather than emitted.

## Setup

1. **Wildcard DNS**: Point `*.dev.yourdomain.com` to your dev server

2. **Wildcard cert**: Generate with mkcert and install for Caddy:
   ```bash
   mkcert "*.dev.yourdomain.com"
   sudo mkdir -p /etc/caddy/certs
   sudo cp _wildcard.dev.yourdomain.com*.pem /etc/caddy/certs/
   sudo chown caddy:caddy /etc/caddy/certs/*
   sudo chmod 600 /etc/caddy/certs/*-key.pem
   ```

3. **Caddy import**: Add to `/etc/caddy/Caddyfile`:
   ```
   import /home/you/.config/caddy/Caddyfile.dev
   ```

4. **Run**: `dev-router generate --reload`

## License

MIT
