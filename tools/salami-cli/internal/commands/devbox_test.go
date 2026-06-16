package commands

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/devbox"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/sshkeys"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"golang.org/x/crypto/ssh"
)

func TestDevboxCreateCommandIsRegistered(t *testing.T) {
	cmd, err := NewRootCmd()
	if err != nil {
		t.Fatalf("NewRootCmd: %v", err)
	}
	cmd.SetArgs([]string{"devbox", "create", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	help := out.String()
	for _, want := range []string{"Create a Devbox", "--port", "--no-wait", "--timeout"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help %q does not contain %q", help, want)
		}
	}
}

func TestDevboxCommandIncludesGetAndDelete(t *testing.T) {
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
	for _, want := range []string{"get", "delete", "start", "stop", "restart"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help %q does not contain %q", help, want)
		}
	}
}

func TestDevboxStatusPrinterUsesCompactLine(t *testing.T) {
	var out bytes.Buffer
	printer := newDevboxStatusPrinter(&out)

	printer.Update(devbox.Devbox{
		Namespace:         "team-a",
		Name:              "dev-a",
		VMPrintableStatus: "Provisioning",
	}, 2*time.Second)
	printer.Update(devbox.Devbox{
		Namespace:         "team-a",
		Name:              "dev-a",
		VMPrintableStatus: "Running",
		Address:           "10.0.0.10",
	}, 3*time.Second)

	got := out.String()
	for _, want := range []string{"=> Starting devbox team-a/dev-a: Provisioning...", "2s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
	for _, unwanted := range []string{"waiting", "VMStatus=", "Address:", "10.0.0.10", "Running"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("output %q unexpectedly contains %q", got, unwanted)
		}
	}
}

func TestDevboxDeleteStatusPrinterUsesCompactLine(t *testing.T) {
	var out bytes.Buffer
	printer := newDevboxDeleteStatusPrinter(&out)

	printer.Update(devbox.Devbox{
		Namespace: "team-a",
		Name:      "dev-a",
	}, 2*time.Second)

	got := out.String()
	for _, want := range []string{"=> Deleting devbox team-a/dev-a: Terminating...", "2s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
}

func TestDevboxStopStatusPrinterUsesCompactLine(t *testing.T) {
	var out bytes.Buffer
	printer := newDevboxStopStatusPrinter(&out)

	printer.Update(devbox.Devbox{
		Namespace:         "team-a",
		Name:              "dev-a",
		VMPrintableStatus: "Stopping",
	}, 2*time.Second)

	got := out.String()
	for _, want := range []string{"=> Stopping devbox team-a/dev-a: Stopping...", "2s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
}

func TestDevboxStatusPrinterClearsInteractiveLine(t *testing.T) {
	var out bytes.Buffer
	printer := newDevboxStatusPrinter(&out)
	printer.interactive = true

	printer.Update(devbox.Devbox{
		Namespace:         "team-a",
		Name:              "dev-a",
		VMPrintableStatus: "Provisioning",
	}, 2*time.Second)
	printer.Done()

	got := out.String()
	if !strings.HasSuffix(got, "\r") {
		t.Fatalf("output %q does not clear back to the start of the line", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("output %q unexpectedly leaves a newline", got)
	}
	if !strings.Contains(got, strings.Repeat(" ", len("=> Starting devbox team-a/dev-a: Provisioning.  2s"))) {
		t.Fatalf("output %q does not clear the rendered line", got)
	}
}

func TestFormatDevboxStarted(t *testing.T) {
	got := formatDevboxStarted(devbox.Devbox{
		Namespace: "team-a",
		Name:      "dev-a",
	}, 74*time.Second)
	want := "=> Devbox team-a/dev-a started in 1m14s"
	if got != want {
		t.Fatalf("formatDevboxStarted = %q, want %q", got, want)
	}
}

func TestFormatDevboxDeleted(t *testing.T) {
	got := formatDevboxDeleted("team-a", "dev-a", 74*time.Second)
	want := "=> Devbox team-a/dev-a deleted in 1m14s"
	if got != want {
		t.Fatalf("formatDevboxDeleted = %q, want %q", got, want)
	}
}

func TestFormatDevboxStoppedAndRestarted(t *testing.T) {
	db := devbox.Devbox{
		Namespace: "team-a",
		Name:      "dev-a",
	}
	if got, want := formatDevboxStopped(db, 74*time.Second), "=> Devbox team-a/dev-a stopped in 1m14s"; got != want {
		t.Fatalf("formatDevboxStopped = %q, want %q", got, want)
	}
	if got, want := formatDevboxRestarted(db, 74*time.Second), "=> Devbox team-a/dev-a restarted in 1m14s"; got != want {
		t.Fatalf("formatDevboxRestarted = %q, want %q", got, want)
	}
}

func TestEnsureDevboxSSHKeys(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-ssh-keys",
			Namespace: "team-a",
		},
		Data: map[string]string{
			sshkeys.AuthorizedKeysDataKey: testAuthorizedKeyLine(t, 1, "user@example.com") + "\n",
		},
	})

	if err := ensureDevboxSSHKeys(context.Background(), client, "team-a", "user-ssh-keys"); err != nil {
		t.Fatalf("ensureDevboxSSHKeys: %v", err)
	}
}

func TestEnsureDevboxSSHKeysReportsMissingConfigMap(t *testing.T) {
	client := fake.NewSimpleClientset()

	err := ensureDevboxSSHKeys(context.Background(), client, "team-a", "user-ssh-keys")
	if err == nil {
		t.Fatal("expected missing ConfigMap error")
	}
	if !strings.Contains(err.Error(), "salami keys add --file ~/.ssh/id_ed25519.pub") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnsureDevboxSSHKeysReportsEmptyConfigMap(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-ssh-keys",
			Namespace: "team-a",
		},
		Data: map[string]string{
			sshkeys.AuthorizedKeysDataKey: "",
		},
	})

	err := ensureDevboxSSHKeys(context.Background(), client, "team-a", "user-ssh-keys")
	if err == nil {
		t.Fatal("expected empty ConfigMap error")
	}
	if !strings.Contains(err.Error(), "does not contain any SSH keys") {
		t.Fatalf("error = %v", err)
	}
}

func testAuthorizedKeyLine(t *testing.T, seedByte byte, comment string) string {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = seedByte
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))) + " " + comment
}
