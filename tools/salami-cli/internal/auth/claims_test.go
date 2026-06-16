package auth

import (
	"encoding/base64"
	"strconv"
	"testing"
	"time"
)

func TestParseAndValidateCachedClaims(t *testing.T) {
	expiry := time.Now().Add(time.Hour).Unix()
	token := unsignedJWT(`{
		"iss": "https://issuer.example",
		"sub": "user-1",
		"email": "user@example.com",
		"groups": ["k8s-akaia"],
		"aud": ["kube-apiserver"],
		"exp": ` + unixString(expiry) + `
	}`)

	claims, err := ParseUnverifiedClaims(token)
	if err != nil {
		t.Fatalf("parse claims: %v", err)
	}
	if err := ValidateCachedClaims(claims, "https://issuer.example", "kube-apiserver", time.Now()); err != nil {
		t.Fatalf("validate claims: %v", err)
	}
	if claims.Email != "user@example.com" {
		t.Fatalf("Email = %q", claims.Email)
	}
}

func TestValidateCachedClaimsRejectsMissingGroups(t *testing.T) {
	claims := Claims{
		Issuer:  "https://issuer.example",
		Subject: "user-1",
		Email:   "user@example.com",
		Aud:     Audiences{"kube-apiserver"},
		Expiry:  time.Now().Add(time.Hour).Unix(),
	}
	if err := ValidateCachedClaims(claims, "https://issuer.example", "kube-apiserver", time.Now()); err == nil {
		t.Fatal("expected missing groups error")
	}
}

func TestAudiencesUnmarshalString(t *testing.T) {
	token := unsignedJWT(`{
		"iss": "https://issuer.example",
		"sub": "user-1",
		"email": "user@example.com",
		"groups": ["k8s-akaia"],
		"aud": "kube-apiserver",
		"exp": ` + unixString(time.Now().Add(time.Hour).Unix()) + `
	}`)
	claims, err := ParseUnverifiedClaims(token)
	if err != nil {
		t.Fatalf("parse claims: %v", err)
	}
	if !claims.Aud.Contains("kube-apiserver") {
		t.Fatalf("audience = %#v", claims.Aud)
	}
}

func unsignedJWT(payload string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + "."
}

func unixString(value int64) string {
	return strconv.FormatInt(value, 10)
}
