package devbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const (
	OwnerAnnotation = "platform.salami.network/owner"
	DevboxLabel     = "platform.salami.network/devbox"
)

var Resource = schema.GroupVersionResource{
	Group:    "platform.salami.network",
	Version:  "v1",
	Resource: "devboxes",
}

var VirtualMachineResource = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Version:  "v1",
	Resource: "virtualmachines",
}

type Devbox struct {
	Namespace               string
	Name                    string
	Owner                   string
	PowerState              string
	AuthorizedKeysConfigMap string
	ExposedPorts            []int
	Address                 string
	VMPrintableStatus       string
	YggdrasilAddress        string
	CreationTimestamp       time.Time
}

type CreateOptions struct {
	Namespace               string
	Name                    string
	AuthorizedKeysConfigMap string
	ExposedPorts            []int
}

type WaitOptions struct {
	Timeout      time.Duration
	PollInterval time.Duration
	OnUpdate     func(Devbox, time.Duration)
}

func List(ctx context.Context, client dynamic.Interface, namespace string, allNamespaces bool) ([]Devbox, error) {
	resource := client.Resource(Resource)
	var list *unstructured.UnstructuredList
	var err error
	if allNamespaces {
		list, err = resource.List(ctx, metav1.ListOptions{})
	} else {
		if namespace == "" {
			return nil, fmt.Errorf("namespace is required")
		}
		list, err = resource.Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list devboxes: %w", err)
	}
	devboxes := make([]Devbox, 0, len(list.Items))
	for i := range list.Items {
		devboxes = append(devboxes, FromUnstructured(&list.Items[i]))
	}
	if err := AttachVMStatuses(ctx, client, devboxes, namespace, allNamespaces); err != nil {
		return nil, err
	}
	return devboxes, nil
}

func BuildObject(opts CreateOptions) (*unstructured.Unstructured, error) {
	if opts.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if opts.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if opts.AuthorizedKeysConfigMap == "" {
		return nil, fmt.Errorf("authorized keys ConfigMap is required")
	}
	ports, err := NormalizePorts(opts.ExposedPorts)
	if err != nil {
		return nil, err
	}
	exposedPorts := make([]any, 0, len(ports))
	for _, port := range ports {
		exposedPorts = append(exposedPorts, int64(port))
	}
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "platform.salami.network/v1",
			"kind":       "Devbox",
			"metadata": map[string]any{
				"name":      opts.Name,
				"namespace": opts.Namespace,
			},
			"spec": map[string]any{
				"powerState":   "Running",
				"exposedPorts": exposedPorts,
				"authorizedKeysConfigMapRef": map[string]any{
					"name": opts.AuthorizedKeysConfigMap,
				},
			},
		},
	}
	obj.SetGroupVersionKind(Resource.GroupVersion().WithKind("Devbox"))
	return obj, nil
}

func Create(ctx context.Context, client dynamic.Interface, opts CreateOptions) (Devbox, error) {
	obj, err := BuildObject(opts)
	if err != nil {
		return Devbox{}, err
	}
	created, err := client.Resource(Resource).Namespace(opts.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return Devbox{}, fmt.Errorf("devbox %s/%s already exists", opts.Namespace, opts.Name)
	}
	if err != nil {
		return Devbox{}, fmt.Errorf("create devbox %s/%s: %w", opts.Namespace, opts.Name, err)
	}
	return FromUnstructured(created), nil
}

func Get(ctx context.Context, client dynamic.Interface, namespace string, name string) (Devbox, error) {
	if namespace == "" {
		return Devbox{}, fmt.Errorf("namespace is required")
	}
	if name == "" {
		return Devbox{}, fmt.Errorf("name is required")
	}
	obj, err := client.Resource(Resource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Devbox{}, fmt.Errorf("get devbox %s/%s: %w", namespace, name, err)
	}
	devbox := FromUnstructured(obj)
	vmStatus, err := VMPrintableStatus(ctx, client, namespace, name)
	if err != nil {
		return Devbox{}, err
	}
	devbox.VMPrintableStatus = vmStatus
	return devbox, nil
}

func Delete(ctx context.Context, client dynamic.Interface, namespace string, name string) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if name == "" {
		return fmt.Errorf("name is required")
	}
	err := client.Resource(Resource).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("devbox %s/%s does not exist", namespace, name)
	}
	if err != nil {
		return fmt.Errorf("delete devbox %s/%s: %w", namespace, name, err)
	}
	return nil
}

func SetPowerState(ctx context.Context, client dynamic.Interface, namespace string, name string, powerState string) (Devbox, error) {
	if namespace == "" {
		return Devbox{}, fmt.Errorf("namespace is required")
	}
	if name == "" {
		return Devbox{}, fmt.Errorf("name is required")
	}
	if powerState != "Running" && powerState != "Stopped" {
		return Devbox{}, fmt.Errorf("invalid power state %q", powerState)
	}
	patch := []byte(fmt.Sprintf(`{"spec":{"powerState":%q}}`, powerState))
	updated, err := client.Resource(Resource).Namespace(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	if apierrors.IsNotFound(err) {
		return Devbox{}, fmt.Errorf("devbox %s/%s does not exist", namespace, name)
	}
	if err != nil {
		return Devbox{}, fmt.Errorf("patch devbox %s/%s power state: %w", namespace, name, err)
	}
	return FromUnstructured(updated), nil
}

func WaitForRunning(ctx context.Context, client dynamic.Interface, namespace string, name string, opts WaitOptions) (Devbox, error) {
	return WaitForVMStatus(ctx, client, namespace, name, "Running", opts)
}

func WaitForVMStatus(ctx context.Context, client dynamic.Interface, namespace string, name string, targetStatus string, opts WaitOptions) (Devbox, error) {
	if targetStatus == "" {
		return Devbox{}, fmt.Errorf("target VM status is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	started := time.Now()
	var last Devbox
	for {
		current, err := Get(ctx, client, namespace, name)
		if apierrors.IsNotFound(err) {
			return Devbox{}, fmt.Errorf("devbox %s/%s no longer exists", namespace, name)
		}
		if err != nil {
			return Devbox{}, err
		}
		last = current
		if opts.OnUpdate != nil {
			opts.OnUpdate(current, time.Since(started))
		}
		if current.VMPrintableStatus == targetStatus {
			return current, nil
		}

		timer := time.NewTimer(opts.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return last, fmt.Errorf("timed out waiting for devbox %s/%s to become %s; last VMStatus=%s", namespace, name, targetStatus, valueOrDash(last.VMPrintableStatus))
			}
			return last, ctx.Err()
		case <-timer.C:
		}
	}
}

func WaitForDeleted(ctx context.Context, client dynamic.Interface, namespace string, name string, opts WaitOptions) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	started := time.Now()
	var last Devbox
	for {
		obj, err := client.Resource(Resource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get devbox %s/%s: %w", namespace, name, err)
		}
		last = FromUnstructured(obj)
		vmStatus, err := VMPrintableStatus(ctx, client, namespace, name)
		if err != nil {
			return err
		}
		last.VMPrintableStatus = vmStatus
		if opts.OnUpdate != nil {
			opts.OnUpdate(last, time.Since(started))
		}

		timer := time.NewTimer(opts.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("timed out waiting for devbox %s/%s to be deleted; last VMStatus=%s", namespace, name, valueOrDash(last.VMPrintableStatus))
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func AttachVMStatuses(ctx context.Context, client dynamic.Interface, devboxes []Devbox, namespace string, allNamespaces bool) error {
	if len(devboxes) == 0 {
		return nil
	}
	resource := client.Resource(VirtualMachineResource)
	var list *unstructured.UnstructuredList
	var err error
	if allNamespaces {
		list, err = resource.List(ctx, metav1.ListOptions{LabelSelector: DevboxLabel})
	} else {
		if namespace == "" {
			return fmt.Errorf("namespace is required")
		}
		list, err = resource.Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: DevboxLabel})
	}
	if err != nil {
		return fmt.Errorf("list virtualmachines for devbox statuses: %w", err)
	}

	statuses := map[string]string{}
	for i := range list.Items {
		vm := &list.Items[i]
		devboxName := vm.GetLabels()[DevboxLabel]
		if devboxName == "" {
			continue
		}
		key := namespacedName(vm.GetNamespace(), devboxName)
		status := VMPrintableStatusFromUnstructured(vm)
		if vm.GetName() == devboxName || statuses[key] == "" {
			statuses[key] = status
		}
	}
	for i := range devboxes {
		devboxes[i].VMPrintableStatus = statuses[namespacedName(devboxes[i].Namespace, devboxes[i].Name)]
	}
	return nil
}

func VMPrintableStatus(ctx context.Context, client dynamic.Interface, namespace string, name string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	vm, err := client.Resource(VirtualMachineResource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get virtualmachine %s/%s: %w", namespace, name, err)
	}
	return VMPrintableStatusFromUnstructured(vm), nil
}

func VMPrintableStatusFromUnstructured(item *unstructured.Unstructured) string {
	status, _, _ := unstructured.NestedString(item.Object, "status", "printableStatus")
	return status
}

func FromUnstructured(item *unstructured.Unstructured) Devbox {
	powerState, _, _ := unstructured.NestedString(item.Object, "spec", "powerState")
	if powerState == "" {
		powerState = "Running"
	}
	keysConfigMap, _, _ := unstructured.NestedString(item.Object, "spec", "authorizedKeysConfigMapRef", "name")
	address, _, _ := unstructured.NestedString(item.Object, "status", "address")
	yggAddress, _, _ := unstructured.NestedString(item.Object, "status", "yggdrasilAddress")
	return Devbox{
		Namespace:               item.GetNamespace(),
		Name:                    item.GetName(),
		Owner:                   item.GetAnnotations()[OwnerAnnotation],
		PowerState:              powerState,
		AuthorizedKeysConfigMap: keysConfigMap,
		ExposedPorts:            exposedPortsFromUnstructured(item),
		Address:                 address,
		YggdrasilAddress:        yggAddress,
		CreationTimestamp:       item.GetCreationTimestamp().Time,
	}
}

func NormalizePorts(ports []int) ([]int, error) {
	if len(ports) == 0 {
		ports = []int{22}
	}
	seen := map[int]bool{}
	normalized := make([]int, 0, len(ports))
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("port %d is outside the valid range 1-65535", port)
		}
		if seen[port] {
			continue
		}
		seen[port] = true
		normalized = append(normalized, port)
	}
	return normalized, nil
}

func FilterByOwner(devboxes []Devbox, owner string) []Devbox {
	filtered := make([]Devbox, 0, len(devboxes))
	for _, devbox := range devboxes {
		if devbox.Owner == owner {
			filtered = append(filtered, devbox)
		}
	}
	return filtered
}

func WriteTable(w io.Writer, devboxes []Devbox, includeNamespace bool, includeOwner bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	headers := make([]string, 0, 6)
	if includeNamespace {
		headers = append(headers, "NAMESPACE")
	}
	headers = append(headers, "NAME")
	if includeOwner {
		headers = append(headers, "OWNER")
	}
	headers = append(headers, "POWER", "VMSTATUS", "ADDRESS")
	if _, err := fmt.Fprintln(tw, joinColumns(headers)); err != nil {
		return err
	}
	for _, devbox := range devboxes {
		values := make([]string, 0, len(headers))
		if includeNamespace {
			values = append(values, valueOrDash(devbox.Namespace))
		}
		values = append(values, valueOrDash(devbox.Name))
		if includeOwner {
			values = append(values, valueOrDash(devbox.Owner))
		}
		values = append(values, valueOrDash(devbox.PowerState), valueOrDash(devbox.VMPrintableStatus), valueOrDash(devbox.Address))
		if _, err := fmt.Fprintln(tw, joinColumns(values)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func WriteSummary(w io.Writer, devbox Devbox, ports []int) error {
	if len(ports) == 0 {
		ports = devbox.ExposedPorts
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "Name:\t%s\n", valueOrDash(devbox.Name)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "Namespace:\t%s\n", valueOrDash(devbox.Namespace)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "VMStatus:\t%s\n", valueOrDash(devbox.VMPrintableStatus)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "SSHKeys:\t%s\n", valueOrDash(devbox.AuthorizedKeysConfigMap)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "Address:\t%s\n", valueOrDash(devbox.Address)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "Yggdrasil:\t%s\n", valueOrDash(devbox.YggdrasilAddress)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "Ports:\t%s\n", formatPorts(ports)); err != nil {
		return err
	}
	return tw.Flush()
}

func OwnerFromEmail(email string) string {
	return "oidc:" + email
}

func joinColumns(values []string) string {
	line := ""
	for i, value := range values {
		if i > 0 {
			line += "\t"
		}
		line += value
	}
	return line
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func namespacedName(namespace string, name string) string {
	return namespace + "/" + name
}

func exposedPortsFromUnstructured(item *unstructured.Unstructured) []int {
	values, found, _ := unstructured.NestedSlice(item.Object, "spec", "exposedPorts")
	if !found {
		return nil
	}
	ports := make([]int, 0, len(values))
	for _, value := range values {
		switch port := value.(type) {
		case int:
			ports = append(ports, port)
		case int32:
			ports = append(ports, int(port))
		case int64:
			ports = append(ports, int(port))
		case float64:
			if port == float64(int(port)) {
				ports = append(ports, int(port))
			}
		}
	}
	return ports
}

func formatPorts(ports []int) string {
	if len(ports) == 0 {
		return "-"
	}
	out := ""
	for i, port := range ports {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprint(port)
	}
	return out
}
