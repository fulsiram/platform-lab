package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
)

type TokenSet struct {
	IssuerURL    string    `json:"issuerUrl"`
	ClientID     string    `json:"clientId"`
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	IDToken      string    `json:"idToken"`
	TokenType    string    `json:"tokenType,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

type Cache struct {
	Path string
}

func (c Cache) Load() (TokenSet, bool, error) {
	data, err := os.ReadFile(c.Path)
	if errors.Is(err, os.ErrNotExist) {
		return TokenSet{}, false, nil
	}
	if err != nil {
		return TokenSet{}, false, fmt.Errorf("read token cache: %w", err)
	}
	var token TokenSet
	if err := json.Unmarshal(data, &token); err != nil {
		return TokenSet{}, false, fmt.Errorf("parse token cache: %w", err)
	}
	if token.IDToken == "" {
		return TokenSet{}, false, errors.New("token cache does not contain an ID token")
	}
	return token, true, nil
}

func (c Cache) Save(token TokenSet) error {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return fmt.Errorf("create token cache directory: %w", err)
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token cache: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.Path), ".tokens-*.json")
	if err != nil {
		return fmt.Errorf("create temporary token cache: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure temporary token cache: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write token cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close token cache: %w", err)
	}
	if err := os.Rename(tmpName, c.Path); err != nil {
		return fmt.Errorf("replace token cache: %w", err)
	}
	if err := os.Chmod(c.Path, 0o600); err != nil {
		return fmt.Errorf("secure token cache: %w", err)
	}
	return nil
}

func (c Cache) Delete() error {
	err := os.Remove(c.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove token cache: %w", err)
	}
	return nil
}

func (t TokenSet) OAuth2TokenForRefresh() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       time.Now().Add(-time.Hour),
	}
}

func (t TokenSet) Fresh(now time.Time) bool {
	return tokenFresh(t.Expiry, now)
}
