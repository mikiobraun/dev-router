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
