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
}

type ScanResult struct {
	Projects []Project
	Warnings []string
}

// corsConfig is the per-service CORS declaration in dev.yaml.
type corsConfig struct {
	Origins []string `yaml:"origins"`
	Expose  []string `yaml:"expose"`
}

type serviceConfig struct {
	Name        string      `yaml:"name"`
	Port        int         `yaml:"port"`
	Domain      string      `yaml:"domain"`
	Enabled     *bool       `yaml:"enabled"`
	Auth        *bool       `yaml:"auth"`
	Token       *bool       `yaml:"token"`
	TokenEnv    string      `yaml:"token_env"`
	CORS        *corsConfig `yaml:"cors"`
	CaddyImport string      `yaml:"caddy_import"`
}

type devConfig struct {
	// Single service format
	Port        int         `yaml:"port"`
	Name        string      `yaml:"name"`
	Domain      string      `yaml:"domain"`
	Enabled     *bool       `yaml:"enabled"`
	Auth        *bool       `yaml:"auth"`
	Token       *bool       `yaml:"token"`
	TokenEnv    string      `yaml:"token_env"`
	CORS        *corsConfig `yaml:"cors"`
	CaddyImport string      `yaml:"caddy_import"`
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
func corsFields(c *corsConfig) (origins, expose []string) {
	if c == nil {
		return nil, nil
	}
	return c.Origins, c.Expose
}

func Scan(projectsDir string) (*ScanResult, error) {
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

		// Check for dev.yaml
		devYamlPath := filepath.Join(dirPath, "dev.yaml")
		data, err := os.ReadFile(devYamlPath)
		if err != nil {
			continue // No dev.yaml, skip
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
				origins, expose := corsFields(svc.CORS)
				result.Projects = append(result.Projects, Project{
					Name:        svc.Name,
					Port:        svc.Port,
					Domain:      svc.Domain,
					Path:        dirPath,
					Enabled:     enabled,
					Auth:        svc.Auth != nil && *svc.Auth,
					Token:       svc.Token != nil && *svc.Token,
					TokenEnv:    svc.TokenEnv,
					CORSOrigins: origins,
					CORSExpose:  expose,
					CaddyImport: resolveImport(dirPath, svc.CaddyImport),
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

		origins, expose := corsFields(devCfg.CORS)
		result.Projects = append(result.Projects, Project{
			Name:        name,
			Port:        devCfg.Port,
			Domain:      devCfg.Domain,
			Path:        dirPath,
			Enabled:     enabled,
			Auth:        devCfg.Auth != nil && *devCfg.Auth,
			Token:       devCfg.Token != nil && *devCfg.Token,
			TokenEnv:    devCfg.TokenEnv,
			CORSOrigins: origins,
			CORSExpose:  expose,
			CaddyImport: resolveImport(dirPath, devCfg.CaddyImport),
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

	return result, nil
}
