package scanner

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Project struct {
	Name    string
	Port    int
	Host    string // upstream host (default "localhost")
	Path    string
	Enabled bool
	Auth    bool
	// Token, when true, makes Caddy inject a shared bearer to the backend
	// (`header_up Authorization "Bearer {env.<name>}"`) so the service can verify
	// the caller is the gateway (defence-in-depth on top of loopback binding).
	// The value lives in Caddy's environment, never here; TokenEnv overrides the
	// default env-var name (<NAME>_TOKEN).
	Token    bool
	TokenEnv string
	// CORSOrigins is the allowlist of Origins the browser may make cross-origin
	// requests from (e.g. the SPA editor's host). Empty means no CORS emitted.
	// A single "*" allows any origin (fine only because auth is by bearer token,
	// not cookies — but prefer explicit origins).
	CORSOrigins []string
	// CORSExpose is the set of response headers the browser JS may read
	// (Access-Control-Expose-Headers), e.g. ETag, WWW-Authenticate.
	CORSExpose []string
	// CORSHeaders is the set of request headers the browser may send
	// (Access-Control-Allow-Headers). Empty means the generator's default list,
	// which covers Authorization, Content-Type and the conditional-request
	// headers; set it when a service needs something else, since a header the
	// preflight doesn't advertise is one the browser refuses to send.
	CORSHeaders []string
	// Domain, when set, overrides the generated hostname entirely — the vhost
	// uses this instead of {name}.{global_domain}. Use it when a project lives
	// on its own domain (e.g. "volume-auth.miki.one") rather than a subdomain of
	// the dev-router base domain.
	Domain string
	// CaddyImport, when set, is the absolute path to a project-owned raw Caddy
	// snippet that dev-router imports verbatim (a `caddy_import` in dev.yaml).
	// It's the escape hatch for projects the structured model can't express:
	// dev-router emits an `import` line and validates nothing — the project owns
	// the hostnames, ports, and correctness. A project with a CaddyImport and no
	// Port gets only the import, no default vhost.
	CaddyImport string
	// RateLimit, when set, emits a caddy-ratelimit `rate_limit` handler for this
	// vhost (requires the github.com/mholt/caddy-ratelimit module compiled into
	// Caddy). Scoped to the given paths if any, else the whole vhost; keyed per
	// client IP unless Key overrides. The vhost is wrapped in a route{} so the
	// plugin directive has a defined order.
	RateLimit *RateLimit
}

// RateLimit is the per-service rate_limit declaration (caddy-ratelimit module).
type RateLimit struct {
	Paths  []string // request paths to limit; empty = the whole vhost
	Events int      // allowed events per window
	Window string   // window duration (Caddy duration string, e.g. "1m")
	Key    string   // rate-limit key; empty = "{remote_host}" (per client IP)
}

type ScanResult struct {
	Projects []Project
	Warnings []string
}

// corsConfig is the per-service CORS declaration in dev.yaml.
type corsConfig struct {
	Origins []string `yaml:"origins"`
	Expose  []string `yaml:"expose"`
	Headers []string `yaml:"headers"`
}

// rateLimitConfig is the per-service rate_limit declaration in dev.yaml.
type rateLimitConfig struct {
	Paths  []string `yaml:"paths"`
	Events int      `yaml:"events"`
	Window string   `yaml:"window"`
	Key    string   `yaml:"key"`
}

type serviceConfig struct {
	Name        string           `yaml:"name"`
	Port        int              `yaml:"port"`
	Host        string           `yaml:"host"`
	Domain      string           `yaml:"domain"`
	Enabled     *bool            `yaml:"enabled"`
	Auth        *bool            `yaml:"auth"`
	Token       *bool            `yaml:"token"`
	TokenEnv    string           `yaml:"token_env"`
	CORS        *corsConfig      `yaml:"cors"`
	CaddyImport string           `yaml:"caddy_import"`
	RateLimit   *rateLimitConfig `yaml:"rate_limit"`
}

type devConfig struct {
	// Single service format
	Port        int              `yaml:"port"`
	Name        string           `yaml:"name"`
	Host        string           `yaml:"host"`
	Domain      string           `yaml:"domain"`
	Enabled     *bool            `yaml:"enabled"`
	Auth        *bool            `yaml:"auth"`
	Token       *bool            `yaml:"token"`
	TokenEnv    string           `yaml:"token_env"`
	CORS        *corsConfig      `yaml:"cors"`
	CaddyImport string           `yaml:"caddy_import"`
	RateLimit   *rateLimitConfig `yaml:"rate_limit"`
	// Multi-service format
	Services []serviceConfig `yaml:"services"`
}

// resolveImport turns a dev.yaml caddy_import value into an absolute path,
// relative to the project directory unless it's already absolute.
func resolveImport(dirPath, val string) string {
	if val == "" {
		return ""
	}
	if filepath.IsAbs(val) {
		return val
	}
	return filepath.Join(dirPath, val)
}

// corsFields safely unpacks a possibly-nil corsConfig.
func corsFields(c *corsConfig) (origins, expose, headers []string) {
	if c == nil {
		return nil, nil, nil
	}
	return c.Origins, c.Expose, c.Headers
}

// rateLimit safely converts a possibly-nil rateLimitConfig.
func rateLimit(c *rateLimitConfig) *RateLimit {
	if c == nil {
		return nil
	}
	return &RateLimit{Paths: c.Paths, Events: c.Events, Window: c.Window, Key: c.Key}
}

func Scan(projectsDir, configFile string) (*ScanResult, error) {
	result := &ScanResult{}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(projectsDir, entry.Name())

		cfgPath := filepath.Join(dirPath, configFile)
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			continue // No config file, skip
		}

		var devCfg devConfig
		if err := yaml.Unmarshal(data, &devCfg); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: malformed YAML: %v", entry.Name(), err))
			continue
		}

		// Multi-service format
		if len(devCfg.Services) > 0 {
			for _, svc := range devCfg.Services {
				enabled := true
				if svc.Enabled != nil {
					enabled = *svc.Enabled
				}
				origins, expose, headers := corsFields(svc.CORS)
				result.Projects = append(result.Projects, Project{
					Name:        svc.Name,
					Port:        svc.Port,
					Host:        svc.Host,
					Domain:      svc.Domain,
					Path:        dirPath,
					Enabled:     enabled,
					Auth:        svc.Auth != nil && *svc.Auth,
					Token:       svc.Token != nil && *svc.Token,
					TokenEnv:    svc.TokenEnv,
					CORSOrigins: origins,
					CORSExpose:  expose,
					CORSHeaders: headers,
					CaddyImport: resolveImport(dirPath, svc.CaddyImport),
					RateLimit:   rateLimit(svc.RateLimit),
				})
			}
			continue
		}

		// Single service format
		name := entry.Name()
		if devCfg.Name != "" {
			name = devCfg.Name
		}

		enabled := true
		if devCfg.Enabled != nil {
			enabled = *devCfg.Enabled
		}

		origins, expose, headers := corsFields(devCfg.CORS)
		result.Projects = append(result.Projects, Project{
			Name:        name,
			Port:        devCfg.Port,
			Host:        devCfg.Host,
			Domain:      devCfg.Domain,
			Path:        dirPath,
			Enabled:     enabled,
			Auth:        devCfg.Auth != nil && *devCfg.Auth,
			Token:       devCfg.Token != nil && *devCfg.Token,
			TokenEnv:    devCfg.TokenEnv,
			CORSOrigins: origins,
			CORSExpose:  expose,
			CORSHeaders: headers,
			CaddyImport: resolveImport(dirPath, devCfg.CaddyImport),
			RateLimit:   rateLimit(devCfg.RateLimit),
		})
	}

	// Check for duplicate ports (ignore import-only services, which have no port).
	portUsers := make(map[int][]string)
	for _, p := range result.Projects {
		if p.Enabled && p.Port > 0 {
			portUsers[p.Port] = append(portUsers[p.Port], p.Name)
		}
	}
	for port, names := range portUsers {
		if len(names) > 1 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("port %d used by multiple services: %v", port, names))
		}
	}

	// A service with neither a port nor a caddy_import produces nothing.
	for _, p := range result.Projects {
		if p.Enabled && p.Port == 0 && p.CaddyImport == "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: no port and no caddy_import; nothing generated", p.Name))
		}
	}

	// A rate_limit missing events/window can't be emitted; flag it rather than
	// silently dropping a security control.
	for _, p := range result.Projects {
		if p.Enabled && p.RateLimit != nil && (p.RateLimit.Events <= 0 || p.RateLimit.Window == "") {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: rate_limit needs events>0 and window; ignoring", p.Name))
		}
	}

	return result, nil
}
