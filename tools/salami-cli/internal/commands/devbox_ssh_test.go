package commands

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDevboxSSHCommandsAreRegistered(t *testing.T) {
	cmd, err := NewRootCmd()
	if err != nil {
		t.Fatalf("NewRootCmd: %v", err)
	}
	cmd.SetArgs([]string{"devbox", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	help := out.String()
	for _, want := range []string{"ssh", "ssh-config", "forward"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help %q does not contain %q", help, want)
		}
	}
	if strings.Contains(help, "ssh-proxy") {
		t.Fatalf("help unexpectedly contains hidden command: %q", help)
	}
}

func TestDevboxSSHProxyCommandIsHiddenButCallable(t *testing.T) {
	cmd, err := NewRootCmd()
	if err != nil {
		t.Fatalf("NewRootCmd: %v", err)
	}
	proxy, _, err := cmd.Find([]string{"devbox", "ssh-proxy"})
	if err != nil {
		t.Fatalf("Find ssh-proxy: %v", err)
	}
	if proxy == nil || !proxy.Hidden {
		t.Fatalf("ssh-proxy command = %#v", proxy)
	}
}

func TestBuildDevboxProxyCommandQuotesArgs(t *testing.T) {
	got := buildDevboxProxyCommand("/Applications/Salami CLI/salami", []string{"--config", "/tmp/salami config.yaml", "--namespace", "team-a"}, "dev-a", 22)
	want := "'/Applications/Salami CLI/salami' --config '/tmp/salami config.yaml' --namespace team-a devbox ssh-proxy dev-a --port 22"
	if got != want {
		t.Fatalf("proxy command = %q, want %q", got, want)
	}
}

func TestDevboxProxyGlobalArgsIncludesOverrides(t *testing.T) {
	global := &globalOptions{
		ConfigPath: "/tmp/salami.yaml",
	}
	cmd := &cobra.Command{Use: "test"}
	flags := cmd.Flags()
	flags.StringVar(&global.APIServer, "api-server", "", "")
	flags.StringVar(&global.IssuerURL, "issuer-url", "", "")
	flags.StringVar(&global.OIDCClientID, "oidc-client-id", "", "")
	flags.StringVar(&global.CertificateAuthority, "certificate-authority", "", "")
	flags.StringVar(&global.CertificateAuthorityData, "certificate-authority-data", "", "")
	flags.StringSliceVar(&global.OIDCScopes, "oidc-scope", nil, "")
	if err := flags.Parse([]string{
		"--api-server", "https://api.example.com",
		"--oidc-scope", "email",
		"--oidc-scope", "groups",
	}); err != nil {
		t.Fatalf("Parse flags: %v", err)
	}

	got := devboxProxyGlobalArgs(cmd, global, "team-a")
	want := []string{
		"--config", "/tmp/salami.yaml",
		"--api-server", "https://api.example.com",
		"--oidc-scope", "email",
		"--oidc-scope", "groups",
		"--namespace", "team-a",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("global args = %#v, want %#v", got, want)
	}
}

func TestWriteDevboxSSHConfig(t *testing.T) {
	var out bytes.Buffer
	err := writeDevboxSSHConfig(&out, devboxSSHConfigOptions{
		User:     "user",
		Port:     22,
		Identity: "~/.ssh/id_ed25519",
		Host:     "dev-a.team-a.devbox",
	}, "team-a", "dev-a", "salami --namespace team-a devbox ssh-proxy dev-a --port 22")
	if err != nil {
		t.Fatalf("writeDevboxSSHConfig: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Host dev-a.team-a.devbox",
		"  HostName dev-a.team-a.devbox",
		"  User user",
		"  ProxyCommand salami --namespace team-a devbox ssh-proxy dev-a --port 22",
		"  HostKeyAlias salami-devbox/team-a/dev-a",
		"  CheckHostIP no",
		"  IdentityFile ~/.ssh/id_ed25519",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("config %q does not contain %q", got, want)
		}
	}
}

func TestBuildOpenSSHArgs(t *testing.T) {
	got := buildOpenSSHArgs(devboxSSHConfigOptions{
		User:     "dev",
		Identity: "/tmp/id",
		Host:     "dev-a.team-a.devbox",
	}, "team-a", "dev-a", "salami devbox ssh-proxy dev-a", []string{"-L", "8080:localhost:8080"})
	want := []string{
		"-o", "ProxyCommand=salami devbox ssh-proxy dev-a",
		"-o", "HostKeyAlias=salami-devbox/team-a/dev-a",
		"-o", "CheckHostIP=no",
		"-i", "/tmp/id",
		"-L", "8080:localhost:8080",
		"dev@dev-a.team-a.devbox",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ssh args = %#v, want %#v", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"salami":       "salami",
		"/tmp/salami":  "/tmp/salami",
		"two words":    "'two words'",
		"quote'here":   "'quote'\\''here'",
		"":             "''",
		"team-a/dev-a": "team-a/dev-a",
	}
	for input, want := range tests {
		if got := shellQuote(input); got != want {
			t.Fatalf("shellQuote(%q) = %q, want %q", input, got, want)
		}
	}
}
