package auth

import (
	"context"
	"sync"
	"time"
)

type IDTokenProvider struct {
	manager Manager
	mu      sync.Mutex
	token   TokenSet
}

func NewIDTokenProvider(manager Manager) *IDTokenProvider {
	return &IDTokenProvider{manager: manager}
}

func (p *IDTokenProvider) IDToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token.IDToken != "" && p.token.Fresh(p.now()) {
		return p.token.IDToken, nil
	}

	token, _, err := p.manager.Token(ctx)
	if err != nil {
		return "", err
	}
	p.token = token
	return token.IDToken, nil
}

func (p *IDTokenProvider) now() time.Time {
	if p.manager.Now != nil {
		return p.manager.Now()
	}
	return time.Now()
}
