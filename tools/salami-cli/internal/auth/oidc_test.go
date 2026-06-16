package auth

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
)

func TestTokenFallsBackToLoginWhenCachedTokenExpired(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "tokens.json")
	cache := Cache{Path: path}
	expiredToken := unsignedJWT(`{
		"iss": "https://issuer.example",
		"sub": "old-user",
		"email": "old@example.com",
		"groups": ["k8s-akaia"],
		"aud": ["kube-apiserver"],
		"exp": ` + unixString(now.Add(-time.Hour).Unix()) + `
	}`)
	if err := cache.Save(TokenSet{
		IssuerURL: "https://issuer.example",
		ClientID:  "kube-apiserver",
		IDToken:   expiredToken,
		Expiry:    now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("save token: %v", err)
	}

	var stderr bytes.Buffer
	loginCalls := 0
	manager := Manager{
		Config: appconfig.Config{
			IssuerURL:    "https://issuer.example",
			OIDCClientID: "kube-apiserver",
		},
		Cache:  cache,
		Stderr: &stderr,
		Now: func() time.Time {
			return now
		},
		LoginFunc: func(context.Context, LoginOptions) (TokenSet, Claims, error) {
			loginCalls++
			return TokenSet{
					IDToken: "new-id-token",
					Expiry:  now.Add(time.Hour),
				}, Claims{
					Subject: "new-user",
					Email:   "new@example.com",
					Groups:  []string{"k8s-akaia"},
				}, nil
		},
	}

	token, claims, err := manager.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if loginCalls != 1 {
		t.Fatalf("loginCalls = %d, want 1", loginCalls)
	}
	if token.IDToken != "new-id-token" {
		t.Fatalf("IDToken = %q", token.IDToken)
	}
	if claims.Email != "new@example.com" {
		t.Fatalf("Email = %q", claims.Email)
	}
	if !strings.Contains(stderr.String(), "starting OIDC login") {
		t.Fatalf("stderr %q does not mention OIDC login", stderr.String())
	}
}
