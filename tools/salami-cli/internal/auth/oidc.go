package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
	"golang.org/x/oauth2"
)

const minTokenLifetime = time.Minute

type Manager struct {
	Config      appconfig.Config
	Cache       Cache
	OpenBrowser func(string) error
	LoginFunc   func(context.Context, LoginOptions) (TokenSet, Claims, error)
	Stderr      io.Writer
	Now         func() time.Time
}

type LoginOptions struct {
	ListenAddress string
	NoBrowser     bool
	Timeout       time.Duration
}

type Status struct {
	Authenticated bool
	Claims        Claims
	TokenPath     string
	NeedsRefresh  bool
}

func NewManager(cfg appconfig.Config, stderr io.Writer) Manager {
	return Manager{
		Config:      cfg,
		Cache:       Cache{Path: cfg.TokenCachePath},
		OpenBrowser: OpenBrowser,
		Stderr:      stderr,
		Now:         time.Now,
	}
}

func (m Manager) Login(ctx context.Context, opts LoginOptions) (TokenSet, Claims, error) {
	if opts.ListenAddress == "" {
		opts.ListenAddress = "127.0.0.1:0"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	listener, err := net.Listen("tcp", opts.ListenAddress)
	if err != nil {
		return TokenSet{}, Claims{}, fmt.Errorf("listen for OIDC callback: %w", err)
	}
	defer listener.Close()

	redirectURL := "http://" + listener.Addr().String() + "/callback"
	provider, oauthConfig, err := oauthConfig(ctx, m.Config, redirectURL)
	if err != nil {
		return TokenSet{}, Claims{}, err
	}

	state, err := randomURLString(32)
	if err != nil {
		return TokenSet{}, Claims{}, err
	}
	verifier, err := randomURLString(64)
	if err != nil {
		return TokenSet{}, Claims{}, err
	}
	authURL := oauthConfig.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge", pkceChallenge(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	results := make(chan callbackResult, 1)
	server := callbackServer(state, results)
	defer server.Shutdown(context.Background())
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case results <- callbackResult{err: fmt.Errorf("serve OIDC callback: %w", err)}:
			default:
			}
		}
	}()

	fmt.Fprintf(m.stderr(), "Open this URL to log in:\n%s\n", authURL)
	if !opts.NoBrowser {
		if err := m.openBrowser(authURL); err != nil {
			fmt.Fprintf(m.stderr(), "Could not open a browser automatically: %v\n", err)
		}
	}

	var result callbackResult
	select {
	case <-ctx.Done():
		return TokenSet{}, Claims{}, fmt.Errorf("waiting for OIDC callback: %w", ctx.Err())
	case result = <-results:
		if result.err != nil {
			return TokenSet{}, Claims{}, result.err
		}
	}

	token, err := oauthConfig.Exchange(
		ctx,
		result.code,
		oauth2.SetAuthURLParam("code_verifier", verifier),
	)
	if err != nil {
		return TokenSet{}, Claims{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	tokenSet, claims, err := m.tokenSetFromOAuthToken(ctx, provider, token)
	if err != nil {
		return TokenSet{}, Claims{}, err
	}
	if err := m.Cache.Save(tokenSet); err != nil {
		return TokenSet{}, Claims{}, err
	}
	return tokenSet, claims, nil
}

func (m Manager) Token(ctx context.Context) (TokenSet, Claims, error) {
	token, found, err := m.Cache.Load()
	if err != nil {
		return TokenSet{}, Claims{}, err
	}
	if !found {
		return TokenSet{}, Claims{}, errors.New("not logged in; run `salami auth login` first")
	}

	claims, err := ParseUnverifiedClaims(token.IDToken)
	if err == nil && tokenFresh(claims.ExpirationTime(), m.now()) {
		if err := ValidateCachedClaims(claims, m.Config.IssuerURL, m.Config.OIDCClientID, m.now()); err != nil {
			return TokenSet{}, Claims{}, err
		}
		token.Expiry = claims.ExpirationTime()
		return token, claims, nil
	}
	if token.RefreshToken == "" {
		if err != nil {
			return m.loginAfterTokenFailure(ctx, fmt.Errorf("cached ID token is invalid: %w", err))
		}
		return m.loginAfterTokenFailure(ctx, errors.New("cached ID token is expired and no refresh token is available"))
	}

	tokenSet, claims, err := m.refresh(ctx, token)
	if err == nil {
		return tokenSet, claims, nil
	}
	return m.loginAfterTokenFailure(ctx, fmt.Errorf("refresh OIDC token: %w", err))
}

func (m Manager) refresh(ctx context.Context, token TokenSet) (TokenSet, Claims, error) {
	provider, oauthConfig, err := oauthConfig(ctx, m.Config, "http://127.0.0.1/callback")
	if err != nil {
		return TokenSet{}, Claims{}, err
	}
	refreshed, err := oauthConfig.TokenSource(ctx, token.OAuth2TokenForRefresh()).Token()
	if err != nil {
		return TokenSet{}, Claims{}, err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = token.RefreshToken
	}
	tokenSet, claims, err := m.tokenSetFromOAuthToken(ctx, provider, refreshed)
	if err != nil {
		return TokenSet{}, Claims{}, err
	}
	if err := m.Cache.Save(tokenSet); err != nil {
		return TokenSet{}, Claims{}, err
	}
	return tokenSet, claims, nil
}

func (m Manager) loginAfterTokenFailure(ctx context.Context, reason error) (TokenSet, Claims, error) {
	fmt.Fprintf(m.stderr(), "%v; starting OIDC login\n", reason)
	return m.login(ctx, LoginOptions{})
}

func (m Manager) Status() (Status, error) {
	token, found, err := m.Cache.Load()
	if err != nil {
		return Status{}, err
	}
	status := Status{Authenticated: found, TokenPath: m.Cache.Path}
	if !found {
		return status, nil
	}
	claims, err := ParseUnverifiedClaims(token.IDToken)
	if err != nil {
		return Status{}, err
	}
	status.Claims = claims
	status.NeedsRefresh = !tokenFresh(claims.ExpirationTime(), m.now())
	return status, nil
}

func (m Manager) Logout() error {
	return m.Cache.Delete()
}

func (m Manager) tokenSetFromOAuthToken(ctx context.Context, provider *oidc.Provider, token *oauth2.Token) (TokenSet, Claims, error) {
	rawIDToken, _ := token.Extra("id_token").(string)
	if rawIDToken == "" {
		return TokenSet{}, Claims{}, errors.New("OIDC provider did not return an ID token")
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: m.Config.OIDCClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		return TokenSet{}, Claims{}, fmt.Errorf("verify ID token: %w", err)
	}
	var claims Claims
	if err := idToken.Claims(&claims); err != nil {
		return TokenSet{}, Claims{}, fmt.Errorf("parse ID token claims: %w", err)
	}
	if err := ValidateUserClaims(claims); err != nil {
		return TokenSet{}, Claims{}, err
	}
	return TokenSet{
		IssuerURL:    m.Config.IssuerURL,
		ClientID:     m.Config.OIDCClientID,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		IDToken:      rawIDToken,
		TokenType:    token.TokenType,
		Expiry:       idToken.Expiry,
	}, claims, nil
}

func oauthConfig(ctx context.Context, cfg appconfig.Config, redirectURL string) (*oidc.Provider, *oauth2.Config, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return provider, &oauth2.Config{
		ClientID:    cfg.OIDCClientID,
		Endpoint:    provider.Endpoint(),
		RedirectURL: redirectURL,
		Scopes:      cfg.Scopes,
	}, nil
}

type callbackResult struct {
	code string
	err  error
}

func callbackServer(state string, results chan<- callbackResult) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "OIDC state mismatch", http.StatusBadRequest)
			sendCallbackResult(results, callbackResult{err: errors.New("OIDC state mismatch")})
			return
		}
		if providerErr := r.URL.Query().Get("error"); providerErr != "" {
			description := r.URL.Query().Get("error_description")
			http.Error(w, "OIDC login failed", http.StatusBadRequest)
			if description != "" {
				providerErr += ": " + description
			}
			sendCallbackResult(results, callbackResult{err: fmt.Errorf("OIDC login failed: %s", providerErr)})
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "OIDC callback missing code", http.StatusBadRequest)
			sendCallbackResult(results, callbackResult{err: errors.New("OIDC callback missing code")})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "Login complete. You can close this window.\n")
		sendCallbackResult(results, callbackResult{code: code})
	})
	return &http.Server{Handler: mux}
}

func sendCallbackResult(results chan<- callbackResult, result callbackResult) {
	select {
	case results <- result:
	default:
	}
}

func randomURLString(byteCount int) (string, error) {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func tokenFresh(expiry time.Time, now time.Time) bool {
	return !expiry.IsZero() && expiry.After(now.Add(minTokenLifetime))
}

func (m Manager) stderr() io.Writer {
	if m.Stderr != nil {
		return m.Stderr
	}
	return io.Discard
}

func (m Manager) openBrowser(url string) error {
	if m.OpenBrowser != nil {
		return m.OpenBrowser(url)
	}
	return nil
}

func (m Manager) login(ctx context.Context, opts LoginOptions) (TokenSet, Claims, error) {
	if m.LoginFunc != nil {
		return m.LoginFunc(ctx, opts)
	}
	return m.Login(ctx, opts)
}

func (m Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}
