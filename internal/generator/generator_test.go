package generator

import (
	"strings"
	"testing"

	"github.com/mikiobraun/dev-router/internal/config"
	"github.com/mikiobraun/dev-router/internal/scanner"
)

func TestGenerateForwardAuth(t *testing.T) {
	cfg := &config.Config{
		Domain:       "rp5.miki.one",
		CertPath:     "/certs/wild.pem",
		KeyPath:      "/certs/wild-key.pem",
		AuthUpstream: "localhost:6100",
	}
	projects := []scanner.Project{
		{Name: "open", Port: 3000, Enabled: true, Auth: false},
		{Name: "secure", Port: 4000, Enabled: true, Auth: true},
	}
	out := Generate(cfg, projects)

	// The open service has no forward_auth.
	openBlock := blockFor(out, "open.rp5.miki.one")
	if strings.Contains(openBlock, "forward_auth") {
		t.Errorf("open service should not have forward_auth:\n%s", openBlock)
	}

	// The secure service gets a forward_auth block pointing at /verify.
	secBlock := blockFor(out, "secure.rp5.miki.one")
	for _, want := range []string{
		"handle /.well-known/oauth-protected-resource {",
		"forward_auth localhost:6100 {",
		"uri /verify",
		"copy_headers X-Volume-User X-Volume-Scopes",
		"reverse_proxy localhost:4000",
	} {
		if !strings.Contains(secBlock, want) {
			t.Errorf("secure block missing %q:\n%s", want, secBlock)
		}
	}
}

func TestGenerateHandleErrors(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem"}
	out := Generate(cfg, []scanner.Project{{Name: "open", Port: 3000, Enabled: true}})

	block := blockFor(out, "open.rp5.miki.one")
	for _, want := range []string{
		"handle_errors 502 503 {",
		`respond "open is not running (localhost:3000) — error {err.status_code}" 503`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("block missing %q:\n%s", want, block)
		}
	}
}

func TestGenerateAuthWithoutUpstreamWarns(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", AuthUpstream: ""}
	out := Generate(cfg, []scanner.Project{{Name: "secure", Port: 4000, Enabled: true, Auth: true}})
	if !strings.Contains(out, "WARNING") || strings.Contains(out, "forward_auth") {
		t.Errorf("expected a warning and no forward_auth when auth_upstream is unset:\n%s", out)
	}
}

// A service with cors.origins gets a route{} wrapper, a reflected ACAO header
// for each origin, exposed headers, and a preflight handler — composed with the
// forward_auth gate.
func TestGenerateCORSWithAuth(t *testing.T) {
	cfg := &config.Config{
		Domain:       "rp5.miki.one",
		CertPath:     "/c.pem",
		KeyPath:      "/k.pem",
		AuthUpstream: "localhost:6100",
	}
	projects := []scanner.Project{{
		Name: "kbmcp", Port: 8070, Enabled: true, Auth: true,
		CORSOrigins: []string{"https://editor.rp5.miki.one"},
		CORSExpose:  []string{"ETag", "WWW-Authenticate"},
	}}
	block := blockFor(Generate(cfg, projects), "kbmcp.rp5.miki.one")

	for _, want := range []string{
		"route {",
		"@cors_o0 header Origin https://editor.rp5.miki.one",
		`Access-Control-Allow-Origin "https://editor.rp5.miki.one"`,
		`Access-Control-Expose-Headers "ETag, WWW-Authenticate"`,
		"defer",
		"@preflight method OPTIONS",
		"handle @preflight {",
		`Access-Control-Allow-Methods "GET, PUT, POST, DELETE, OPTIONS"`,
		"respond 204",
		// still gated
		"forward_auth localhost:6100 {",
		"reverse_proxy localhost:8070",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("CORS+auth block missing %q:\n%s", want, block)
		}
	}

	// The preflight handler must appear before forward_auth so OPTIONS is
	// answered without hitting /verify.
	if strings.Index(block, "@preflight") > strings.Index(block, "forward_auth") {
		t.Errorf("preflight must precede forward_auth:\n%s", block)
	}
}

// Without cors.origins there is no route wrapper and no CORS headers (unchanged
// output).
func TestGenerateNoCORSByDefault(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem"}
	block := blockFor(Generate(cfg, []scanner.Project{{Name: "open", Port: 3000, Enabled: true}}), "open.rp5.miki.one")
	for _, unwanted := range []string{"route {", "Access-Control-Allow-Origin", "@preflight"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("non-CORS service should not contain %q:\n%s", unwanted, block)
		}
	}
}

// A wildcard origin emits a single static ACAO with no per-origin matcher.
func TestGenerateCORSWildcard(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem"}
	block := blockFor(Generate(cfg, []scanner.Project{{
		Name: "wild", Port: 5000, Enabled: true, CORSOrigins: []string{"*"},
	}}), "wild.rp5.miki.one")
	if !strings.Contains(block, `Access-Control-Allow-Origin "*"`) {
		t.Errorf("wildcard ACAO missing:\n%s", block)
	}
	if strings.Contains(block, "@cors_o0") {
		t.Errorf("wildcard should not emit a per-origin matcher:\n%s", block)
	}
}

// An import-only project (caddy_import, no port) emits just the import line and
// no generated vhost. A project with both a port and a caddy_import emits both.
func TestGenerateCaddyImport(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem"}
	out := Generate(cfg, []scanner.Project{
		{Name: "ory-demo", Enabled: true, CaddyImport: "/home/x/Projects/ory-demo/Caddyfile.snippet"},
		{Name: "both", Port: 9000, Enabled: true, CaddyImport: "/home/x/Projects/both/extra.caddy"},
	})

	// import-only: an import line, but no ory-demo.rp5.miki.one vhost.
	if !strings.Contains(out, "import /home/x/Projects/ory-demo/Caddyfile.snippet") {
		t.Errorf("missing import line for ory-demo:\n%s", out)
	}
	if strings.Contains(out, "ory-demo.rp5.miki.one {") {
		t.Errorf("import-only project should not emit a default vhost:\n%s", out)
	}

	// both: the generated vhost AND its import line.
	if blockFor(out, "both.rp5.miki.one") == "" {
		t.Errorf("port+import project should still emit its vhost:\n%s", out)
	}
	if !strings.Contains(out, "import /home/x/Projects/both/extra.caddy") {
		t.Errorf("port+import project should also emit the import line:\n%s", out)
	}
}

// token injects a Caddy-supplied shared bearer via an {env.<NAME>_TOKEN}
// reference (never the literal), on the gated backend reverse_proxy.
func TestGenerateToken(t *testing.T) {
	cfg := &config.Config{
		Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem",
		AuthUpstream: "localhost:6100",
	}
	block := blockFor(Generate(cfg, []scanner.Project{
		{Name: "kbmcp", Port: 8070, Enabled: true, Auth: true, Token: true},
	}), "kbmcp.rp5.miki.one")

	for _, want := range []string{
		"reverse_proxy localhost:8070 {",
		`header_up Authorization "Bearer {env.KBMCP_TOKEN}"`,
		"forward_auth localhost:6100 {", // still gated
	} {
		if !strings.Contains(block, want) {
			t.Errorf("token block missing %q:\n%s", want, block)
		}
	}
	// The discovery proxy to the auth upstream must NOT carry the token header.
	if strings.Contains(block, "reverse_proxy localhost:6100 {") {
		t.Errorf("token header leaked onto the discovery/auth proxy:\n%s", block)
	}
}

// token_env overrides the env-var name (e.g. to share one secret across services).
func TestGenerateTokenEnvOverride(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem"}
	block := blockFor(Generate(cfg, []scanner.Project{
		{Name: "kbmcp", Port: 8070, Enabled: true, Token: true, TokenEnv: "GATEWAY_TOKEN"},
	}), "kbmcp.rp5.miki.one")
	if !strings.Contains(block, `header_up Authorization "Bearer {env.GATEWAY_TOKEN}"`) {
		t.Errorf("token_env override not honoured:\n%s", block)
	}
}

// A service with a hyphen gets a sanitised default env-var name.
func TestGenerateTokenNameSanitised(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem"}
	block := blockFor(Generate(cfg, []scanner.Project{
		{Name: "volume-auth", Port: 6100, Enabled: true, Token: true},
	}), "volume-auth.rp5.miki.one")
	if !strings.Contains(block, `{env.VOLUME_AUTH_TOKEN}`) {
		t.Errorf("hyphen not sanitised to _:\n%s", block)
	}
}

// Without token there is no header_up and the proxy stays a bare line.
func TestGenerateNoTokenByDefault(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem"}
	block := blockFor(Generate(cfg, []scanner.Project{{Name: "open", Port: 3000, Enabled: true}}), "open.rp5.miki.one")
	if strings.Contains(block, "header_up") || strings.Contains(block, "reverse_proxy localhost:3000 {") {
		t.Errorf("non-token service should have a bare reverse_proxy:\n%s", block)
	}
}

// A project with domain: overrides the generated hostname entirely.
func TestGenerateDomainOverride(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", CertPath: "/c.pem", KeyPath: "/k.pem"}
	out := Generate(cfg, []scanner.Project{
		{Name: "volume-auth", Port: 6100, Domain: "volume-auth.miki.one", Enabled: true},
	})
	if blockFor(out, "volume-auth.miki.one") == "" {
		t.Errorf("expected vhost for the domain override:\n%s", out)
	}
	if blockFor(out, "volume-auth.rp5.miki.one") != "" {
		t.Errorf("should not emit the default subdomain when domain is set:\n%s", out)
	}
}

// When cert_path/key_path are empty, no tls directive is emitted (Let's Encrypt).
func TestGenerateNoTLSWithoutCerts(t *testing.T) {
	cfg := &config.Config{Domain: "miki.one"}
	out := Generate(cfg, []scanner.Project{
		{Name: "myapp", Port: 3000, Enabled: true},
	})
	block := blockFor(out, "myapp.miki.one")
	if block == "" {
		t.Fatalf("expected vhost:\n%s", out)
	}
	if strings.Contains(block, "tls ") {
		t.Errorf("should not emit tls directive without cert paths:\n%s", block)
	}
}

// blockFor returns the Caddy site block starting at the given host header.
func blockFor(caddyfile, host string) string {
	i := strings.Index(caddyfile, host+" {")
	if i < 0 {
		return ""
	}
	end := strings.Index(caddyfile[i:], "\n}\n")
	if end < 0 {
		return caddyfile[i:]
	}
	return caddyfile[i : i+end]
}
