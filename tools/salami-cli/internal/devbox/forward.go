package devbox

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type ForwardTarget struct {
	Devbox      Devbox
	Namespace   string
	Name        string
	RemotePort  int
	ServiceName string
	PodName     string
}

func ResolveForwardTarget(ctx context.Context, dynamicClient dynamic.Interface, kubeClient kubernetes.Interface, namespace string, name string, remotePort int) (ForwardTarget, error) {
	if remotePort < 1 || remotePort > 65535 {
		return ForwardTarget{}, fmt.Errorf("port %d is outside the valid range 1-65535", remotePort)
	}
	current, err := Get(ctx, dynamicClient, namespace, name)
	if err != nil {
		return ForwardTarget{}, err
	}
	if current.VMPrintableStatus != "Running" {
		return ForwardTarget{}, fmt.Errorf("devbox %s/%s is not running; current VMStatus=%s", namespace, name, valueOrDash(current.VMPrintableStatus))
	}
	if !hasPort(current.ExposedPorts, remotePort) {
		return ForwardTarget{}, fmt.Errorf("devbox %s/%s does not expose port %d", namespace, name, remotePort)
	}

	service, err := resolveService(ctx, kubeClient, namespace, name, remotePort)
	if err != nil {
		return ForwardTarget{}, err
	}
	pod, err := resolveServicePod(ctx, kubeClient, service)
	if err != nil {
		return ForwardTarget{}, err
	}

	return ForwardTarget{
		Devbox:      current,
		Namespace:   namespace,
		Name:        name,
		RemotePort:  remotePort,
		ServiceName: service.Name,
		PodName:     pod.Name,
	}, nil
}

func resolveService(ctx context.Context, client kubernetes.Interface, namespace string, devboxName string, remotePort int) (corev1.Service, error) {
	selector := labels.Set{DevboxLabel: devboxName}.String()
	services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return corev1.Service{}, fmt.Errorf("list services for devbox %s/%s: %w", namespace, devboxName, err)
	}
	var fallback *corev1.Service
	preferredName := devboxName + "-svc"
	for i := range services.Items {
		service := &services.Items[i]
		if !serviceExposesPort(service, remotePort) {
			continue
		}
		if service.Name == preferredName {
			return *service, nil
		}
		if fallback == nil {
			fallback = service
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	return corev1.Service{}, fmt.Errorf("no service for devbox %s/%s exposes port %d", namespace, devboxName, remotePort)
}

func resolveServicePod(ctx context.Context, client kubernetes.Interface, service corev1.Service) (corev1.Pod, error) {
	if len(service.Spec.Selector) == 0 {
		return corev1.Pod{}, fmt.Errorf("service %s/%s has no selector", service.Namespace, service.Name)
	}
	pods, err := client.CoreV1().Pods(service.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(),
	})
	if err != nil {
		return corev1.Pod{}, fmt.Errorf("list pods for service %s/%s: %w", service.Namespace, service.Name, err)
	}
	for i := range pods.Items {
		pod := pods.Items[i]
		if pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning {
			return pod, nil
		}
	}
	return corev1.Pod{}, fmt.Errorf("no running pod found for service %s/%s", service.Namespace, service.Name)
}

func serviceExposesPort(service *corev1.Service, remotePort int) bool {
	for _, port := range service.Spec.Ports {
		if int(port.Port) == remotePort {
			return true
		}
	}
	return false
}

func hasPort(ports []int, port int) bool {
	for _, candidate := range ports {
		if candidate == port {
			return true
		}
	}
	return false
}
