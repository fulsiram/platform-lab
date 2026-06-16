package auth

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestWriteExecCredential(t *testing.T) {
	expiry := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := WriteExecCredential(&buf, TokenSet{
		IDToken: "id-token",
		Expiry:  expiry,
	})
	if err != nil {
		t.Fatalf("write exec credential: %v", err)
	}

	var got struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Status     struct {
			Token               string `json:"token"`
			ExpirationTimestamp string `json:"expirationTimestamp"`
		} `json:"status"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("parse exec credential: %v", err)
	}
	if got.APIVersion != "client.authentication.k8s.io/v1" {
		t.Fatalf("apiVersion = %q", got.APIVersion)
	}
	if got.Kind != "ExecCredential" {
		t.Fatalf("kind = %q", got.Kind)
	}
	if got.Status.Token != "id-token" {
		t.Fatalf("token = %q", got.Status.Token)
	}
	if got.Status.ExpirationTimestamp != "2026-06-15T12:00:00Z" {
		t.Fatalf("expirationTimestamp = %q", got.Status.ExpirationTimestamp)
	}
}
