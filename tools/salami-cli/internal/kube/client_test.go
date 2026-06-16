package kube

import (
	"context"
	"net/http"
	"testing"
)

func TestBearerTokenRoundTripperUsesProvider(t *testing.T) {
	calls := 0
	rt := bearerTokenRoundTripper{
		base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if got, want := req.Header.Get("Authorization"), "Bearer fresh-token"; got != want {
				t.Fatalf("Authorization = %q, want %q", got, want)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}),
		provider: func(context.Context) (string, error) {
			calls++
			return "fresh-token", nil
		},
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.example.test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
