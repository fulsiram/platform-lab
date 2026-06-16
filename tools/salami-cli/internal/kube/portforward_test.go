package kube

import "testing"

func TestPortForwardURL(t *testing.T) {
	got, err := portForwardURL("https://pepperoni.salami.network:6443", "team-a", "virt-launcher-dev-a")
	if err != nil {
		t.Fatalf("portForwardURL: %v", err)
	}
	want := "https://pepperoni.salami.network:6443/api/v1/namespaces/team-a/pods/virt-launcher-dev-a/portforward"
	if got.String() != want {
		t.Fatalf("url = %q, want %q", got.String(), want)
	}
}
