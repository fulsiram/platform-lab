package sshkeys

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"golang.org/x/crypto/ssh"
)

const AuthorizedKeysDataKey = "authorized_keys"

var repeatedDash = regexp.MustCompile(`-+`)

type AuthorizedKey struct {
	Line        string
	Type        string
	Comment     string
	Fingerprint string
}

func DefaultConfigMapName(email string) (string, error) {
	local, _, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || local == "" {
		return "", fmt.Errorf("email %q does not contain a local part", email)
	}
	local = strings.ToLower(local)
	var b strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(repeatedDash.ReplaceAllString(b.String(), "-"), "-")
	const suffix = "-ssh-keys"
	if len(name) > 63-len(suffix) {
		name = strings.TrimRight(name[:63-len(suffix)], "-")
	}
	if name == "" {
		return "", fmt.Errorf("email %q does not produce a valid ConfigMap name", email)
	}
	return name + suffix, nil
}

func ReadKeyInput(keyArg string, filePath string) (string, error) {
	if keyArg != "" && filePath != "" {
		return "", errors.New("provide either a key argument or --file, not both")
	}
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read key file %q: %w", filePath, err)
		}
		return string(data), nil
	}
	if keyArg == "" {
		return "", errors.New("provide a public key argument or --file")
	}
	return keyArg, nil
}

func ParseAuthorizedKeys(contents string) ([]AuthorizedKey, error) {
	lines := strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n")
	keys := make([]AuthorizedKey, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("parse authorized_keys line %d: %w", i+1, err)
		}
		keys = append(keys, AuthorizedKey{
			Line:        line,
			Type:        key.Type(),
			Comment:     comment,
			Fingerprint: ssh.FingerprintSHA256(key),
		})
	}
	return keys, nil
}

func AddKey(contents string, keyLine string) (string, AuthorizedKey, bool, error) {
	keys, err := ParseAuthorizedKeys(contents)
	if err != nil {
		return "", AuthorizedKey{}, false, err
	}
	newKeys, err := ParseAuthorizedKeys(keyLine)
	if err != nil {
		return "", AuthorizedKey{}, false, err
	}
	if len(newKeys) != 1 {
		return "", AuthorizedKey{}, false, fmt.Errorf("expected exactly one public key, got %d", len(newKeys))
	}
	key := newKeys[0]
	for _, existing := range keys {
		if existing.Fingerprint == key.Fingerprint {
			return Normalize(keys), key, false, nil
		}
	}
	keys = append(keys, key)
	return Normalize(keys), key, true, nil
}

func RemoveKey(contents string, keyLine string) (string, AuthorizedKey, bool, error) {
	keys, err := ParseAuthorizedKeys(contents)
	if err != nil {
		return "", AuthorizedKey{}, false, err
	}
	removeKeys, err := ParseAuthorizedKeys(keyLine)
	if err != nil {
		return "", AuthorizedKey{}, false, err
	}
	if len(removeKeys) != 1 {
		return "", AuthorizedKey{}, false, fmt.Errorf("expected exactly one public key, got %d", len(removeKeys))
	}
	target := removeKeys[0]
	kept := make([]AuthorizedKey, 0, len(keys))
	var removed AuthorizedKey
	found := false
	for _, key := range keys {
		if key.Fingerprint == target.Fingerprint {
			removed = key
			found = true
			continue
		}
		kept = append(kept, key)
	}
	if !found {
		return "", AuthorizedKey{}, false, nil
	}
	return Normalize(kept), removed, true, nil
}

func Normalize(keys []AuthorizedKey) string {
	if len(keys) == 0 {
		return ""
	}
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key.Line)
	}
	return strings.Join(lines, "\n") + "\n"
}

func ValidateAuthorizedKeys(contents string) (string, []AuthorizedKey, error) {
	keys, err := ParseAuthorizedKeys(contents)
	if err != nil {
		return "", nil, err
	}
	return Normalize(keys), keys, nil
}

func GetContents(ctx context.Context, client kubernetes.Interface, namespace string, name string) (string, bool, error) {
	cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get ConfigMap %s/%s: %w", namespace, name, err)
	}
	return cm.Data[AuthorizedKeysDataKey], true, nil
}

func UpsertContents(ctx context.Context, client kubernetes.Interface, namespace string, name string, contents string) error {
	configMaps := client.CoreV1().ConfigMaps(namespace)
	cm, err := configMaps.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = configMaps.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string]string{
				AuthorizedKeysDataKey: contents,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ConfigMap %s/%s: %w", namespace, name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get ConfigMap %s/%s: %w", namespace, name, err)
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[AuthorizedKeysDataKey] = contents
	if _, err := configMaps.Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update ConfigMap %s/%s: %w", namespace, name, err)
	}
	return nil
}

func WriteAuthorizedKeys(w io.Writer, contents string) error {
	_, err := io.WriteString(w, contents)
	return err
}
