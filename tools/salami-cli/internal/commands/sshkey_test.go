package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestSSHKeyCommandIsRegistered(t *testing.T) {
	cmd, err := NewRootCmd()
	if err != nil {
		t.Fatalf("NewRootCmd: %v", err)
	}
	cmd.SetArgs([]string{"keys", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	help := out.String()
	for _, want := range []string{"Manage SSH authorized keys", "list", "add", "remove", "set"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help %q does not contain %q", help, want)
		}
	}
}

func TestSSHKeyAliasesStillWork(t *testing.T) {
	for _, alias := range []string{"ssh-key", "ssh-keys"} {
		cmd, err := NewRootCmd()
		if err != nil {
			t.Fatalf("NewRootCmd: %v", err)
		}
		cmd.SetArgs([]string{alias, "--help"})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute %s: %v", alias, err)
		}
		if !strings.Contains(out.String(), "Manage SSH authorized keys") {
			t.Fatalf("help for %s = %q", alias, out.String())
		}
	}
}

func TestKeysListHasNoDetailsFlag(t *testing.T) {
	cmd, err := NewRootCmd()
	if err != nil {
		t.Fatalf("NewRootCmd: %v", err)
	}
	cmd.SetArgs([]string{"keys", "list", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out.String(), "--details") {
		t.Fatalf("help unexpectedly contains --details: %q", out.String())
	}
}

func TestKeysRemoveDoesNotMentionFingerprint(t *testing.T) {
	cmd, err := NewRootCmd()
	if err != nil {
		t.Fatalf("NewRootCmd: %v", err)
	}
	cmd.SetArgs([]string{"keys", "remove", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, unwanted := range []string{"FINGERPRINT", "fingerprint", "SHA256"} {
		if strings.Contains(out.String(), unwanted) {
			t.Fatalf("help unexpectedly contains %q: %q", unwanted, out.String())
		}
	}
}
