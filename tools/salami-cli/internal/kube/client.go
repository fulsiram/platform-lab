package kube

import (
	"fmt"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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
