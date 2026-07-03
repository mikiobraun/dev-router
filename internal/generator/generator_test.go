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

func TestGenerateAuthWithoutUpstreamWarns(t *testing.T) {
	cfg := &config.Config{Domain: "rp5.miki.one", AuthUpstream: ""}
	out := Generate(cfg, []scanner.Project{{Name: "secure", Port: 4000, Enabled: true, Auth: true}})
	if !strings.Contains(out, "WARNING") || strings.Contains(out, "forward_auth") {
		t.Errorf("expected a warning and no forward_auth when auth_upstream is unset:\n%s", out)
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
