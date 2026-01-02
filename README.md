# dev-router

[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/mikiobraun/dev-router)](https://goreportcard.com/report/github.com/mikiobraun/dev-router)

Automatic subdomain routing for local dev services.

Scans your projects directory for Git repos with a `dev.yaml` config, then generates a Caddy reverse proxy configuration mapping `{project}.dev.{domain}` to `localhost:{port}`.

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
```

### Project config: `dev.yaml`

Add to any Git repo root:

```yaml
port: 3000
```

Optional fields:
- `name`: override subdomain (default: directory name)
- `enabled`: set to `false` to skip

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
