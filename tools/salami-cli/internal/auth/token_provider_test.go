package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
)

func TestIDTokenProviderCachesFreshToken(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	cache := Cache{Path: filepath.Join(t.TempDir(), "tokens.json")}
	firstToken := tokenProviderTestJWT(t, now.Add(time.Hour))
	secondToken := tokenProviderTestJWT(t, now.Add(2*time.Hour))
	tokenProviderSaveTestToken(t, cache, firstToken, now.Add(time.Hour))
	manager := tokenProviderTestManager(now, cache)
	provider := NewIDTokenProvider(manager)

	first, err := provider.IDToken(context.Background())
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	tokenProviderSaveTestToken(t, cache, secondToken, now.Add(2*time.Hour))
	second, err := provider.IDToken(context.Background())
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if first != second {
		t.Fatalf("tokens differ: %q != %q", first, second)
	}
}

func TestIDTokenProviderReloadsNearExpiry(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	cache := Cache{Path: filepath.Join(t.TempDir(), "tokens.json")}
	firstToken := tokenProviderTestJWT(t, now.Add(time.Hour))
	secondToken := tokenProviderTestJWT(t, now.Add(2*time.Hour))
	tokenProviderSaveTestToken(t, cache, firstToken, now.Add(time.Hour))
	manager := tokenProviderTestManager(now, cache)
	provider := NewIDTokenProvider(manager)

	first, err := provider.IDToken(context.Background())
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	tokenProviderSaveTestToken(t, cache, secondToken, now.Add(2*time.Hour))
	provider.token.Expiry = now.Add(30 * time.Second)
	second, err := provider.IDToken(context.Background())
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if first == second {
		t.Fatalf("expected provider to reload near-expiry token")
	}
}

func tokenProviderTestManager(now time.Time, cache Cache) Manager {
	return Manager{
		Config: appconfig.Config{
			IssuerURL:    "https://issuer.example",
			OIDCClientID: "kube-apiserver",
		},
		Cache: cache,
		Now: func() time.Time {
			return now
		},
	}
}

func tokenProviderSaveTestToken(t *testing.T, cache Cache, idToken string, expiry time.Time) {
	t.Helper()
	if err := cache.Save(TokenSet{
		IssuerURL: "https://issuer.example",
		ClientID:  "kube-apiserver",
		IDToken:   idToken,
		Expiry:    expiry,
	}); err != nil {
		t.Fatalf("save token: %v", err)
	}
}

func tokenProviderTestJWT(t *testing.T, expiry time.Time) string {
	t.Helper()
	return unsignedJWT(`{
		"iss": "https://issuer.example",
		"sub": "user-1",
		"email": "user@example.com",
		"groups": ["k8s-akaia"],
		"aud": ["kube-apiserver"],
		"exp": ` + unixString(expiry.Unix()) + `
	}`)
}
