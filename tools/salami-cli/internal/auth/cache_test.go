package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCacheSaveLoadAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	cache := Cache{Path: path}
	token := TokenSet{
		IssuerURL:    "https://issuer.example",
		ClientID:     "kube-apiserver",
		AccessToken:  "access",
		RefreshToken: "refresh",
		IDToken:      "id",
		TokenType:    "Bearer",
		Expiry:       time.Unix(123, 0).UTC(),
	}

	if err := cache.Save(token); err != nil {
		t.Fatalf("save token: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat token cache: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("mode = %o", mode)
		}
	}

	got, found, err := cache.Load()
	if err != nil {
		t.Fatalf("load token: %v", err)
	}
	if !found {
		t.Fatal("expected token cache to exist")
	}
	if got.IDToken != token.IDToken || got.RefreshToken != token.RefreshToken {
		t.Fatalf("loaded token = %#v", got)
	}

	if err := cache.Delete(); err != nil {
		t.Fatalf("delete token: %v", err)
	}
	_, found, err = cache.Load()
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if found {
		t.Fatal("expected token cache to be deleted")
	}
}
