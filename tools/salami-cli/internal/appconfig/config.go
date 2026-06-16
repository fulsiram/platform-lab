package appconfig

import (
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	DefaultAPIServer    = "https://pepperoni.salami.network:6443"
	DefaultIssuerURL    = "https://auth.salami.network/realms/members"
	DefaultOIDCClientID = "kube-apiserver"
	DefaultNamespace    = "team-akaia"
)

var (
	// DefaultCACertPEM is intentionally a variable so downstream builds can
	// replace the platform CA with ldflags or generated code.
	DefaultCACertPEM = `-----BEGIN CERTIFICATE-----
MIIBeDCCAR2gAwIBAgIBADAKBggqhkjOPQQDAjAjMSEwHwYDVQQDDBhrM3Mtc2Vy
dmVyLWNhQDE3NzUyNjU0OTEwHhcNMjYwNDA0MDExODExWhcNMzYwNDAxMDExODEx
WjAjMSEwHwYDVQQDDBhrM3Mtc2VydmVyLWNhQDE3NzUyNjU0OTEwWTATBgcqhkjO
PQIBBggqhkjOPQMBBwNCAASrlQARBzQBv5Jxhl9GB2hJ4ro7Z8VREsoDJwxiOOCi
57tAk5q5IozQXRTM/zzm6TS8E/lFWqk7Hti/FXbt4krso0IwQDAOBgNVHQ8BAf8E
BAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUs1Q3MJkbHN/N0Ms1bSMz
Jgz7KvcwCgYIKoZIzj0EAwIDSQAwRgIhAK3eeLbY/pAEN7OpiNsBGOld06huIRhV
OgUqll9YXF0zAiEAggu8X+Or+0QGesEAsNlT+IeSufGPiCLNT1e7KimSaSc=
-----END CERTIFICATE-----
`
	DefaultScopes = []string{"openid", "email", "profile", "groups"}
)

type Config struct {
	APIServer                string   `yaml:"apiServer,omitempty"`
	IssuerURL                string   `yaml:"issuerURL,omitempty"`
	OIDCClientID             string   `yaml:"oidcClientID,omitempty"`
	CertificateAuthority     string   `yaml:"certificateAuthority,omitempty"`
	CertificateAuthorityData string   `yaml:"certificateAuthorityData,omitempty"`
	TokenCachePath           string   `yaml:"tokenCachePath,omitempty"`
	Namespace                string   `yaml:"namespace,omitempty"`
	Scopes                   []string `yaml:"scopes,omitempty"`
}

func Defaults() Config {
	return Config{
		APIServer:                DefaultAPIServer,
		IssuerURL:                DefaultIssuerURL,
		OIDCClientID:             DefaultOIDCClientID,
		CertificateAuthorityData: DefaultCACertPEM,
		TokenCachePath:           DefaultTokenCachePath(),
		Namespace:                DefaultNamespace,
		Scopes:                   append([]string(nil), DefaultScopes...),
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	configPath, explicit := ResolveConfigPath(path)
	if configPath != "" {
		fileCfg, found, err := loadFile(configPath)
		if err != nil {
			return Config{}, err
		}
		if !found && explicit {
			return Config{}, fmt.Errorf("config file %q does not exist", configPath)
		}
		if found {
			cfg = Merge(cfg, fileCfg)
		}
	}
	cfg = Merge(cfg, envConfig())
	return Normalize(cfg)
}

func ResolveConfigPath(path string) (string, bool) {
	if path != "" {
		return path, true
	}
	if path = os.Getenv("SALAMI_CONFIG"); path != "" {
		return path, true
	}
	return DefaultConfigPath(), false
}

func DefaultConfigPath() string {
	if dir := os.Getenv("SALAMI_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "config.yaml")
	}
	if runtime.GOOS == "windows" {
		if dir, err := os.UserConfigDir(); err == nil && dir != "" {
			return filepath.Join(dir, "salami", "config.yaml")
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "salami", "config.yaml")
	}
	return filepath.Join(".salami", "config.yaml")
}

func DefaultDataDir() string {
	if dir := os.Getenv("SALAMI_DATA_DIR"); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, "salami")
		}
		if dir, err := os.UserConfigDir(); err == nil && dir != "" {
			return filepath.Join(dir, "salami")
		}
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "salami")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", "salami")
	}
	return ".salami"
}

func DefaultTokenCachePath() string {
	return filepath.Join(DefaultDataDir(), "tokens.json")
}

func Merge(base Config, override Config) Config {
	if override.APIServer != "" {
		base.APIServer = override.APIServer
	}
	if override.IssuerURL != "" {
		base.IssuerURL = override.IssuerURL
	}
	if override.OIDCClientID != "" {
		base.OIDCClientID = override.OIDCClientID
	}
	if override.CertificateAuthority != "" {
		base.CertificateAuthority = override.CertificateAuthority
		base.CertificateAuthorityData = ""
	}
	if override.CertificateAuthorityData != "" {
		base.CertificateAuthorityData = override.CertificateAuthorityData
		base.CertificateAuthority = ""
	}
	if override.TokenCachePath != "" {
		base.TokenCachePath = override.TokenCachePath
	}
	if override.Namespace != "" {
		base.Namespace = override.Namespace
	}
	if len(override.Scopes) > 0 {
		base.Scopes = append([]string(nil), override.Scopes...)
	}
	return base
}

func Normalize(cfg Config) (Config, error) {
	cfg = Merge(Defaults(), cfg)
	cfg.APIServer = strings.TrimSpace(cfg.APIServer)
	cfg.IssuerURL = strings.TrimSpace(cfg.IssuerURL)
	cfg.OIDCClientID = strings.TrimSpace(cfg.OIDCClientID)
	cfg.CertificateAuthority = strings.TrimSpace(cfg.CertificateAuthority)
	cfg.CertificateAuthorityData = strings.TrimSpace(cfg.CertificateAuthorityData)
	cfg.TokenCachePath = strings.TrimSpace(cfg.TokenCachePath)
	cfg.Namespace = strings.TrimSpace(cfg.Namespace)
	if cfg.APIServer == "" {
		return Config{}, errors.New("api server is required")
	}
	if cfg.IssuerURL == "" {
		return Config{}, errors.New("issuer URL is required")
	}
	if cfg.OIDCClientID == "" {
		return Config{}, errors.New("OIDC client ID is required")
	}
	if cfg.Namespace == "" {
		cfg.Namespace = DefaultNamespace
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = append([]string(nil), DefaultScopes...)
	}
	for i := range cfg.Scopes {
		cfg.Scopes[i] = strings.TrimSpace(cfg.Scopes[i])
	}
	return cfg, nil
}

func LoadCAData(cfg Config) ([]byte, error) {
	if cfg.CertificateAuthority != "" {
		ca, err := os.ReadFile(cfg.CertificateAuthority)
		if err != nil {
			return nil, fmt.Errorf("read certificate authority %q: %w", cfg.CertificateAuthority, err)
		}
		return parseCAData(string(ca))
	}
	if cfg.CertificateAuthorityData == "" {
		return nil, nil
	}
	return parseCAData(cfg.CertificateAuthorityData)
}

func parseCAData(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "-----BEGIN ") {
		return requirePEM([]byte(value))
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, fmt.Errorf("certificate authority data must be PEM or base64-encoded PEM: %w", err)
	}
	return requirePEM(decoded)
}

func requirePEM(value []byte) ([]byte, error) {
	if block, _ := pem.Decode(value); block == nil {
		return nil, errors.New("certificate authority data does not contain a PEM block")
	}
	return value, nil
}

func loadFile(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read config file %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config file %q: %w", path, err)
	}
	return cfg, true, nil
}

func envConfig() Config {
	cfg := Config{
		APIServer:                os.Getenv("SALAMI_API_SERVER"),
		IssuerURL:                os.Getenv("SALAMI_ISSUER_URL"),
		OIDCClientID:             os.Getenv("SALAMI_OIDC_CLIENT_ID"),
		CertificateAuthority:     os.Getenv("SALAMI_CA_FILE"),
		CertificateAuthorityData: os.Getenv("SALAMI_CA_DATA"),
		TokenCachePath:           os.Getenv("SALAMI_TOKEN_CACHE"),
		Namespace:                os.Getenv("SALAMI_NAMESPACE"),
	}
	if scopes := os.Getenv("SALAMI_OIDC_SCOPES"); scopes != "" {
		cfg.Scopes = splitScopes(scopes)
	}
	return cfg
}

func splitScopes(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	})
	scopes := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			scopes = append(scopes, part)
		}
	}
	return scopes
}
