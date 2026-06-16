package kube

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type TokenProvider func(context.Context) (string, error)

func RESTConfig(cfg appconfig.Config, idToken string) (*rest.Config, error) {
	caData, err := appconfig.LoadCAData(cfg)
	if err != nil {
		return nil, err
	}
	if idToken == "" {
		return nil, fmt.Errorf("ID token is required")
	}
	return &rest.Config{
		Host:        cfg.APIServer,
		BearerToken: idToken,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caData,
		},
	}, nil
}

func RESTConfigWithTokenProvider(cfg appconfig.Config, provider TokenProvider) (*rest.Config, error) {
	caData, err := appconfig.LoadCAData(cfg)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("token provider is required")
	}
	return &rest.Config{
		Host: cfg.APIServer,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caData,
		},
		WrapTransport: func(base http.RoundTripper) http.RoundTripper {
			return bearerTokenRoundTripper{
				base:     base,
				provider: provider,
			}
		},
	}, nil
}

func DynamicClient(cfg appconfig.Config, idToken string) (dynamic.Interface, error) {
	restCfg, err := RESTConfig(cfg, idToken)
	if err != nil {
		return nil, err
	}
	client, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes dynamic client: %w", err)
	}
	return client, nil
}

func DynamicClientWithTokenProvider(cfg appconfig.Config, provider TokenProvider) (dynamic.Interface, error) {
	restCfg, err := RESTConfigWithTokenProvider(cfg, provider)
	if err != nil {
		return nil, err
	}
	client, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes dynamic client: %w", err)
	}
	return client, nil
}

func KubernetesClient(cfg appconfig.Config, idToken string) (kubernetes.Interface, error) {
	restCfg, err := RESTConfig(cfg, idToken)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	return client, nil
}

func KubernetesClientWithTokenProvider(cfg appconfig.Config, provider TokenProvider) (kubernetes.Interface, error) {
	restCfg, err := RESTConfigWithTokenProvider(cfg, provider)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	return client, nil
}

type bearerTokenRoundTripper struct {
	base     http.RoundTripper
	provider TokenProvider
}

func (rt bearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := rt.provider(req.Context())
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("ID token is required")
	}
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	next := req.Clone(req.Context())
	next.Header = req.Header.Clone()
	next.Header.Set("Authorization", "Bearer "+token)
	return base.RoundTrip(next)
}
