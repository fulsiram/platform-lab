package devbox

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestResolveAccess(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), accessDevbox(t, "team-a", "dev-a", "oidc:user@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}
	kubeClient := kubefake.NewSimpleClientset(
		accessService("team-a", "dev-a-svc", "dev-a", 22),
		accessPod("team-a", "virt-launcher-dev-a", "dev-a", corev1.PodRunning),
	)

	got, err := ResolveAccess(context.Background(), dynamicClient, kubeClient, "team-a", "dev-a", "user@example.com", 22)
	if err != nil {
		t.Fatalf("ResolveAccess: %v", err)
	}
	if got.ServiceName != "dev-a-svc" {
		t.Fatalf("ServiceName = %q", got.ServiceName)
	}
	if got.PodName != "virt-launcher-dev-a" {
		t.Fatalf("PodName = %q", got.PodName)
	}
	if got.RemotePort != 22 {
		t.Fatalf("RemotePort = %d", got.RemotePort)
	}
}

func TestResolveAccessRejectsStoppedDevbox(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), accessDevbox(t, "team-a", "dev-a", "oidc:user@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Stopped"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	_, err := ResolveAccess(context.Background(), dynamicClient, kubefake.NewSimpleClientset(), "team-a", "dev-a", "user@example.com", 22)
	if err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveAccessRejectsOwnerMismatch(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), accessDevbox(t, "team-a", "dev-a", "oidc:other@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	_, err := ResolveAccess(context.Background(), dynamicClient, kubefake.NewSimpleClientset(), "team-a", "dev-a", "user@example.com", 22)
	if err == nil || !strings.Contains(err.Error(), "owned by") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveAccessRejectsMissingPort(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), accessDevbox(t, "team-a", "dev-a", "oidc:user@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	_, err := ResolveAccess(context.Background(), dynamicClient, kubefake.NewSimpleClientset(), "team-a", "dev-a", "user@example.com", 2222)
	if err == nil || !strings.Contains(err.Error(), "does not expose port 2222") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveAccessRejectsMissingRunningPod(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), accessDevbox(t, "team-a", "dev-a", "oidc:user@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}
	kubeClient := kubefake.NewSimpleClientset(
		accessService("team-a", "dev-a-svc", "dev-a", 22),
		accessPod("team-a", "virt-launcher-dev-a", "dev-a", corev1.PodPending),
	)

	_, err := ResolveAccess(context.Background(), dynamicClient, kubeClient, "team-a", "dev-a", "user@example.com", 22)
	if err == nil || !strings.Contains(err.Error(), "no running pod") {
		t.Fatalf("error = %v", err)
	}
}

func accessDevbox(t *testing.T, namespace string, name string, owner string, ports []int) *unstructured.Unstructured {
	t.Helper()
	obj, err := BuildObject(CreateOptions{
		Namespace:               namespace,
		Name:                    name,
		AuthorizedKeysConfigMap: "user-ssh-keys",
		ExposedPorts:            ports,
	})
	if err != nil {
		t.Fatalf("BuildObject: %v", err)
	}
	obj.SetAnnotations(map[string]string{OwnerAnnotation: owner})
	return obj
}

func accessService(namespace string, name string, devboxName string, port int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"platform.salami.network/devbox": devboxName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"platform.salami.network/devbox": devboxName,
			},
			Ports: []corev1.ServicePort{{Port: port}},
		},
	}
}

func accessPod(namespace string, name string, devboxName string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"platform.salami.network/devbox": devboxName,
			},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}
