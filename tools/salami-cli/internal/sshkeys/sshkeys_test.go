package sshkeys

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"golang.org/x/crypto/ssh"
)

func TestDefaultConfigMapName(t *testing.T) {
	tests := map[string]string{
		"fulsiram@example.com":      "fulsiram-ssh-keys",
		"First.Last@example.com":    "first-last-ssh-keys",
		"user_name+lab@example.com": "user-name-lab-ssh-keys",
		"---user---@example.com":    "user-ssh-keys",
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz@example.com": "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzab-ssh-keys",
	}

	for email, want := range tests {
		got, err := DefaultConfigMapName(email)
		if err != nil {
			t.Fatalf("DefaultConfigMapName(%q): %v", email, err)
		}
		if got != want {
			t.Fatalf("DefaultConfigMapName(%q) = %q, want %q", email, got, want)
		}
		if len(got) > 63 {
			t.Fatalf("DefaultConfigMapName(%q) length = %d", email, len(got))
		}
	}
}

func TestDefaultConfigMapNameRejectsInvalidEmail(t *testing.T) {
	for _, email := range []string{"", "not-email", "@example.com", "!!!@example.com"} {
		if _, err := DefaultConfigMapName(email); err == nil {
			t.Fatalf("expected error for %q", email)
		}
	}
}

func TestParseAuthorizedKeys(t *testing.T) {
	line := testAuthorizedKeyLine(t, 1, "user@example.com")
	keys, err := ParseAuthorizedKeys("\n" + line + "\n")
	if err != nil {
		t.Fatalf("ParseAuthorizedKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("len(keys) = %d", len(keys))
	}
	if keys[0].Type != "ssh-ed25519" {
		t.Fatalf("Type = %q", keys[0].Type)
	}
	if keys[0].Comment != "user@example.com" {
		t.Fatalf("Comment = %q", keys[0].Comment)
	}
	if !strings.HasPrefix(keys[0].Fingerprint, "SHA256:") {
		t.Fatalf("Fingerprint = %q", keys[0].Fingerprint)
	}
	if keys[0].Line != line {
		t.Fatalf("Line = %q, want %q", keys[0].Line, line)
	}
}

func TestParseAuthorizedKeysRejectsInvalidLine(t *testing.T) {
	_, err := ParseAuthorizedKeys("not a key\n")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestAddKeyDedupesByFingerprint(t *testing.T) {
	line := testAuthorizedKeyLine(t, 1, "user@example.com")
	next, key, added, err := AddKey("", line)
	if err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	if !added {
		t.Fatal("expected key to be added")
	}
	if !strings.Contains(next, line+"\n") {
		t.Fatalf("contents = %q", next)
	}

	next, duplicate, added, err := AddKey(next, line)
	if err != nil {
		t.Fatalf("AddKey duplicate: %v", err)
	}
	if added {
		t.Fatal("expected duplicate key not to be added")
	}
	if duplicate.Fingerprint != key.Fingerprint {
		t.Fatalf("duplicate fingerprint = %q, want %q", duplicate.Fingerprint, key.Fingerprint)
	}
	if strings.Count(next, "ssh-ed25519") != 1 {
		t.Fatalf("duplicate contents = %q", next)
	}
}

func TestAddKeyRequiresOneKey(t *testing.T) {
	line1 := testAuthorizedKeyLine(t, 1, "one")
	line2 := testAuthorizedKeyLine(t, 2, "two")
	_, _, _, err := AddKey("", line1+"\n"+line2+"\n")
	if err == nil {
		t.Fatal("expected multi-key add error")
	}
}

func TestRemoveKeyByPublicKey(t *testing.T) {
	line1 := testAuthorizedKeyLine(t, 1, "one")
	line2 := testAuthorizedKeyLine(t, 2, "two")
	keys, err := ParseAuthorizedKeys(line1 + "\n" + line2 + "\n")
	if err != nil {
		t.Fatalf("ParseAuthorizedKeys: %v", err)
	}

	next, removed, ok, err := RemoveKey(line1+"\n"+line2+"\n", line1)
	if err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if !ok {
		t.Fatal("expected key to be removed")
	}
	if removed.Fingerprint != keys[0].Fingerprint {
		t.Fatalf("removed fingerprint = %q", removed.Fingerprint)
	}
	if strings.Contains(next, line1) {
		t.Fatalf("contents still contain removed key: %q", next)
	}
	if !strings.Contains(next, line2) {
		t.Fatalf("contents do not contain kept key: %q", next)
	}
}

func TestRemoveKeyMatchesKeyMaterialNotComment(t *testing.T) {
	line := testAuthorizedKeyLine(t, 1, "stored-comment")
	keyWithoutComment := strings.TrimSuffix(strings.TrimSuffix(line, "stored-comment"), " ")
	sameKeyDifferentComment := keyWithoutComment + " removal-comment"

	next, removed, ok, err := RemoveKey(line+"\n", sameKeyDifferentComment)
	if err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if !ok {
		t.Fatal("expected key to be removed")
	}
	if removed.Line != line {
		t.Fatalf("removed line = %q", removed.Line)
	}
	if next != "" {
		t.Fatalf("contents = %q", next)
	}
}

func TestRemoveKeyRequiresOneKey(t *testing.T) {
	line1 := testAuthorizedKeyLine(t, 1, "one")
	line2 := testAuthorizedKeyLine(t, 2, "two")
	_, _, _, err := RemoveKey(line1+"\n"+line2+"\n", line1+"\n"+line2+"\n")
	if err == nil {
		t.Fatal("expected multi-key remove error")
	}
}

func TestValidateAuthorizedKeysNormalizes(t *testing.T) {
	line := testAuthorizedKeyLine(t, 1, "user@example.com")
	normalized, keys, err := ValidateAuthorizedKeys("\n" + line + "\n\n")
	if err != nil {
		t.Fatalf("ValidateAuthorizedKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("len(keys) = %d", len(keys))
	}
	if normalized != line+"\n" {
		t.Fatalf("normalized = %q", normalized)
	}
}

func TestGetAndUpsertContents(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	contents, found, err := GetContents(ctx, client, "team-a", "user-ssh-keys")
	if err != nil {
		t.Fatalf("GetContents missing: %v", err)
	}
	if found {
		t.Fatalf("found = true, contents = %q", contents)
	}

	if err := UpsertContents(ctx, client, "team-a", "user-ssh-keys", "one\n"); err != nil {
		t.Fatalf("UpsertContents create: %v", err)
	}
	cm, err := client.CoreV1().ConfigMaps("team-a").Get(ctx, "user-ssh-keys", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get created configmap: %v", err)
	}
	if cm.Data[AuthorizedKeysDataKey] != "one\n" {
		t.Fatalf("authorized_keys = %q", cm.Data[AuthorizedKeysDataKey])
	}
	if len(cm.Annotations) != 0 {
		t.Fatalf("annotations = %#v", cm.Annotations)
	}

	if err := UpsertContents(ctx, client, "team-a", "user-ssh-keys", "two\n"); err != nil {
		t.Fatalf("UpsertContents update: %v", err)
	}
	contents, found, err = GetContents(ctx, client, "team-a", "user-ssh-keys")
	if err != nil {
		t.Fatalf("GetContents existing: %v", err)
	}
	if !found {
		t.Fatal("expected ConfigMap to exist")
	}
	if contents != "two\n" {
		t.Fatalf("contents = %q", contents)
	}
}

func TestUpsertContentsPreservesExistingMetadata(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-ssh-keys",
			Namespace: "team-a",
			Labels: map[string]string{
				"existing": "true",
			},
		},
	})

	if err := UpsertContents(ctx, client, "team-a", "user-ssh-keys", "keys\n"); err != nil {
		t.Fatalf("UpsertContents update: %v", err)
	}
	cm, err := client.CoreV1().ConfigMaps("team-a").Get(ctx, "user-ssh-keys", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get updated configmap: %v", err)
	}
	if cm.Labels["existing"] != "true" {
		t.Fatalf("labels = %#v", cm.Labels)
	}
	if cm.Data[AuthorizedKeysDataKey] != "keys\n" {
		t.Fatalf("authorized_keys = %q", cm.Data[AuthorizedKeysDataKey])
	}
}

func TestWriteAuthorizedKeys(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteAuthorizedKeys(&buf, "ssh-ed25519 AAAA user\n"); err != nil {
		t.Fatalf("WriteAuthorizedKeys: %v", err)
	}
	if got := buf.String(); got != "ssh-ed25519 AAAA user\n" {
		t.Fatalf("output = %q", got)
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
