# dev-router

Automatic subdomain routing for local dev services.

## Overview

Scans project directories for dev config files and generates a Caddy 
reverse proxy configuration, enabling access via `{project}.dev.{domain}`.

## How it works

1. Scans `~/Projects/` (configurable) for Git repos
2. Looks for `dev.yaml` in each repo root
3. Generates Caddyfile mapping `{repo-name}.dev.{domain}` → `localhost:{port}`
4. Uses mkcert wildcard cert for TLS

## Project config: `dev.yaml`
```yaml
port: 3001
```

That's it. Name is inferred from the repo directory name.

Optional fields for later:
- `name`: override the subdomain (default: directory name)
- `enabled`: false to skip (default: true)

### Escape hatch: `caddy_import`

For projects that don't fit the one-service-one-vhost model (multiple hosts,
path-based routing, anything bespoke), `caddy_import` points at a project-owned
raw Caddy snippet:

```yaml
caddy_import: Caddyfile.snippet   # relative to the project dir (or absolute)
```

dev-router emits a top-level `import <abs-path>` into the generated Caddyfile and
**validates nothing** — the project owns its hostnames, ports, and correctness
(Caddy validates syntax at load). Combine with a `port` to augment the default
vhost with extra hosts, or omit `port` for an import-only project that brings all
its own site blocks (no default vhost is generated). It's the eject button, not
the norm — reach for it only when the structured directives can't express it.

## Global config: `~/.config/dev-router/config.yaml`
```yaml
domain: dev.yourdomain.com
projects_dir: ~/Projects
cert_path: ~/.local/share/mkcert/_wildcard.dev.yourdomain.com.pem
key_path: ~/.local/share/mkcert/_wildcard.dev.yourdomain.com-key.pem
caddyfile_path: ~/.config/caddy/Caddyfile.dev
```

## CLI
```bash
dev-router generate    # scan and write Caddyfile
dev-router list        # show discovered services
```

## Future considerations

- `--reload` flag to call `caddy reload` after generate
- File watcher mode for auto-regeneration
- Health check / status command
