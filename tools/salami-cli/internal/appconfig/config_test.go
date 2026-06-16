package appconfig

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPEM = "-----BEGIN CERTIFICATE-----\nAA==\n-----END CERTIFICATE-----\n"

func TestLoadPrecedence(t *testing.T) {
	t.Setenv("SALAMI_CONFIG_DIR", t.TempDir())
	t.Setenv("SALAMI_DATA_DIR", filepath.Join(t.TempDir(), "data"))

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
apiServer: https://config.example:6443
issuerURL: https://issuer.example/realms/test
oidcClientID: config-client
tokenCachePath: /tmp/config-token-cache.json
namespace: config-ns
scopes:
  - openid
  - email
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("SALAMI_API_SERVER", "https://env.example:6443")
	t.Setenv("SALAMI_OIDC_CLIENT_ID", "env-client")
	t.Setenv("SALAMI_NAMESPACE", "env-ns")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.APIServer != "https://env.example:6443" {
		t.Fatalf("APIServer = %q", cfg.APIServer)
	}
	if cfg.IssuerURL != "https://issuer.example/realms/test" {
		t.Fatalf("IssuerURL = %q", cfg.IssuerURL)
	}
	if cfg.OIDCClientID != "env-client" {
		t.Fatalf("OIDCClientID = %q", cfg.OIDCClientID)
	}
	if cfg.Namespace != "env-ns" {
		t.Fatalf("Namespace = %q", cfg.Namespace)
	}
	if got, want := cfg.Scopes, []string{"openid", "email"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Scopes = %#v", got)
	}
}

func TestCAMergeTreatsFileAndDataAsExclusive(t *testing.T) {
	t.Setenv("SALAMI_CONFIG_DIR", t.TempDir())
	t.Setenv("SALAMI_DATA_DIR", filepath.Join(t.TempDir(), "data"))

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
certificateAuthority: /tmp/config-ca.pem
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("SALAMI_CA_DATA", testPEM)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.CertificateAuthority != "" {
		t.Fatalf("CertificateAuthority = %q", cfg.CertificateAuthority)
	}
	if cfg.CertificateAuthorityData == "" {
		t.Fatal("expected certificate authority data")
	}
}

func TestLoadIgnoresMissingDefaultConfig(t *testing.T) {
	t.Setenv("SALAMI_CONFIG_DIR", t.TempDir())
	t.Setenv("SALAMI_DATA_DIR", filepath.Join(t.TempDir(), "data"))

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.APIServer != DefaultAPIServer {
		t.Fatalf("APIServer = %q", cfg.APIServer)
	}
	if cfg.Namespace != DefaultNamespace {
		t.Fatalf("Namespace = %q", cfg.Namespace)
	}
	ca, err := LoadCAData(cfg)
	if err != nil {
		t.Fatalf("load default CA: %v", err)
	}
	if len(ca) == 0 {
		t.Fatal("expected default CA data")
	}
}

func TestSaveAndLoadUserConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Config{Namespace: "team-dev"}

	if err := SaveUserConfig(path, cfg); err != nil {
		t.Fatalf("save user config: %v", err)
	}
	got, gotPath, err := LoadUserConfig(path)
	if err != nil {
		t.Fatalf("load user config: %v", err)
	}
	if gotPath != path {
		t.Fatalf("path = %q", gotPath)
	}
	if got.Namespace != "team-dev" {
		t.Fatalf("Namespace = %q", got.Namespace)
	}
}

func TestLoadUserConfigAllowsMissingExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "config.yaml")

	cfg, gotPath, err := LoadUserConfig(path)
	if err != nil {
		t.Fatalf("load user config: %v", err)
	}
	if gotPath != path {
		t.Fatalf("path = %q", gotPath)
	}
	if cfg.Namespace != "" {
		t.Fatalf("Namespace = %q", cfg.Namespace)
	}
}

func TestLoadErrorsOnMissingExplicitConfig(t *testing.T) {
	t.Setenv("SALAMI_DATA_DIR", filepath.Join(t.TempDir(), "data"))

	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected missing explicit config error")
	}
}

func TestLoadCAData(t *testing.T) {
	cfg := Config{CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testPEM))}
	ca, err := LoadCAData(cfg)
	if err != nil {
		t.Fatalf("load base64 CA data: %v", err)
	}
	if strings.TrimSpace(string(ca)) != strings.TrimSpace(testPEM) {
		t.Fatalf("CA data = %q", string(ca))
	}

	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte(testPEM), 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}
	ca, err = LoadCAData(Config{CertificateAuthority: path})
	if err != nil {
		t.Fatalf("load CA file: %v", err)
	}
	if strings.TrimSpace(string(ca)) != strings.TrimSpace(testPEM) {
		t.Fatalf("CA file data = %q", string(ca))
	}
}

func TestLoadCADataRejectsInvalidData(t *testing.T) {
	_, err := LoadCAData(Config{CertificateAuthorityData: "not-pem"})
	if err == nil {
		t.Fatal("expected invalid CA error")
	}
}
