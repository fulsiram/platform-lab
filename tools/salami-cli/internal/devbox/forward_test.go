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

func TestResolveForwardTarget(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), forwardDevbox(t, "team-a", "dev-a", "oidc:user@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}
	kubeClient := kubefake.NewSimpleClientset(
		forwardService("team-a", "dev-a-svc", "dev-a", 22),
		forwardPod("team-a", "virt-launcher-dev-a", "dev-a", corev1.PodRunning),
	)

	got, err := ResolveForwardTarget(context.Background(), dynamicClient, kubeClient, "team-a", "dev-a", 22)
	if err != nil {
		t.Fatalf("ResolveForwardTarget: %v", err)
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

func TestResolveForwardTargetAllowsMissingOwnerAnnotation(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), forwardDevbox(t, "team-a", "dev-a", "", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}
	kubeClient := kubefake.NewSimpleClientset(
		forwardService("team-a", "dev-a-svc", "dev-a", 22),
		forwardPod("team-a", "virt-launcher-dev-a", "dev-a", corev1.PodRunning),
	)

	got, err := ResolveForwardTarget(context.Background(), dynamicClient, kubeClient, "team-a", "dev-a", 22)
	if err != nil {
		t.Fatalf("ResolveForwardTarget: %v", err)
	}
	if got.Devbox.Owner != "" {
		t.Fatalf("Owner = %q, want empty", got.Devbox.Owner)
	}
}

func TestResolveForwardTargetRejectsStoppedDevbox(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), forwardDevbox(t, "team-a", "dev-a", "oidc:user@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Stopped"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	_, err := ResolveForwardTarget(context.Background(), dynamicClient, kubefake.NewSimpleClientset(), "team-a", "dev-a", 22)
	if err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveForwardTargetRejectsMissingPort(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), forwardDevbox(t, "team-a", "dev-a", "oidc:user@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	_, err := ResolveForwardTarget(context.Background(), dynamicClient, kubefake.NewSimpleClientset(), "team-a", "dev-a", 2222)
	if err == nil || !strings.Contains(err.Error(), "does not expose port 2222") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveForwardTargetRejectsMissingRunningPod(t *testing.T) {
	dynamicClient := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := dynamicClient.Resource(Resource).Namespace("team-a").Create(context.Background(), forwardDevbox(t, "team-a", "dev-a", "oidc:user@example.com", []int{22}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := dynamicClient.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}
	kubeClient := kubefake.NewSimpleClientset(
		forwardService("team-a", "dev-a-svc", "dev-a", 22),
		forwardPod("team-a", "virt-launcher-dev-a", "dev-a", corev1.PodPending),
	)

	_, err := ResolveForwardTarget(context.Background(), dynamicClient, kubeClient, "team-a", "dev-a", 22)
	if err == nil || !strings.Contains(err.Error(), "no running pod") {
		t.Fatalf("error = %v", err)
	}
}

func forwardDevbox(t *testing.T, namespace string, name string, owner string, ports []int) *unstructured.Unstructured {
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
	if owner != "" {
		obj.SetAnnotations(map[string]string{OwnerAnnotation: owner})
	}
	return obj
}

func forwardService(namespace string, name string, devboxName string, port int32) *corev1.Service {
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

func forwardPod(namespace string, name string, devboxName string, phase corev1.PodPhase) *corev1.Pod {
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
