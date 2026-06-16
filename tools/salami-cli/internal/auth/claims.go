package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	Issuer  string    `json:"iss"`
	Subject string    `json:"sub"`
	Email   string    `json:"email"`
	Groups  []string  `json:"groups"`
	Aud     Audiences `json:"aud"`
	Expiry  int64     `json:"exp"`
}

type Audiences []string

func (a *Audiences) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*a = values
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*a = []string{value}
	return nil
}

func ParseUnverifiedClaims(rawIDToken string) (Claims, error) {
	parts := strings.Split(rawIDToken, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New("ID token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("decode ID token claims: %w", err)
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, fmt.Errorf("parse ID token claims: %w", err)
	}
	return claims, nil
}

func ValidateCachedClaims(claims Claims, issuerURL, clientID string, now time.Time) error {
	if claims.Issuer != issuerURL {
		return fmt.Errorf("ID token issuer %q does not match expected issuer %q", claims.Issuer, issuerURL)
	}
	if !claims.Aud.Contains(clientID) {
		return fmt.Errorf("ID token audience %v does not include %q", []string(claims.Aud), clientID)
	}
	if claims.Expiry == 0 || time.Unix(claims.Expiry, 0).Before(now) {
		return errors.New("ID token is expired")
	}
	return ValidateUserClaims(claims)
}

func ValidateUserClaims(claims Claims) error {
	if claims.Subject == "" {
		return errors.New("ID token is missing sub claim")
	}
	if claims.Email == "" {
		return errors.New("ID token is missing email claim required by the Kubernetes username mapping")
	}
	if len(claims.Groups) == 0 {
		return errors.New("ID token is missing groups claim required by tenant RBAC")
	}
	return nil
}

func (a Audiences) Contains(value string) bool {
	for _, candidate := range a {
		if candidate == value {
			return true
		}
	}
	return false
}

func (c Claims) ExpirationTime() time.Time {
	if c.Expiry == 0 {
		return time.Time{}
	}
	return time.Unix(c.Expiry, 0)
}
