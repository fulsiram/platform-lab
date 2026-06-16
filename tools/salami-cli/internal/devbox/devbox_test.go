package devbox

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestFromUnstructured(t *testing.T) {
	item := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "platform.salami.network/v1",
			"kind":       "Devbox",
			"metadata": map[string]any{
				"name":      "dev-a",
				"namespace": "team-a",
				"annotations": map[string]any{
					OwnerAnnotation: "oidc:user@example.com",
				},
			},
			"spec": map[string]any{
				"powerState":   "Stopped",
				"exposedPorts": []any{int64(22), int64(8080)},
				"diskGenerations": map[string]any{
					"root":     int64(3),
					"nixStore": int64(5),
				},
				"authorizedKeysConfigMapRef": map[string]any{
					"name": "user-ssh-keys",
				},
			},
			"status": map[string]any{
				"address":          "10.0.0.10",
				"yggdrasilAddress": "200::1",
			},
		},
	}

	got := FromUnstructured(item)
	if got.Namespace != "team-a" {
		t.Fatalf("Namespace = %q", got.Namespace)
	}
	if got.Name != "dev-a" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.Owner != "oidc:user@example.com" {
		t.Fatalf("Owner = %q", got.Owner)
	}
	if got.PowerState != "Stopped" {
		t.Fatalf("PowerState = %q", got.PowerState)
	}
	if got.DiskGenerations.Root != 3 || got.DiskGenerations.NixStore != 5 {
		t.Fatalf("DiskGenerations = %#v", got.DiskGenerations)
	}
	if got.AuthorizedKeysConfigMap != "user-ssh-keys" {
		t.Fatalf("AuthorizedKeysConfigMap = %q", got.AuthorizedKeysConfigMap)
	}
	if want := []int{22, 8080}; !reflect.DeepEqual(got.ExposedPorts, want) {
		t.Fatalf("ExposedPorts = %#v, want %#v", got.ExposedPorts, want)
	}
	if got.Address != "10.0.0.10" {
		t.Fatalf("Address = %q", got.Address)
	}
	if got.VMPrintableStatus != "" {
		t.Fatalf("VMPrintableStatus = %q", got.VMPrintableStatus)
	}
	if got.YggdrasilAddress != "200::1" {
		t.Fatalf("YggdrasilAddress = %q", got.YggdrasilAddress)
	}
}

func TestFromUnstructuredDefaultsPowerState(t *testing.T) {
	item := &unstructured.Unstructured{}
	item.SetName("dev-a")

	got := FromUnstructured(item)
	if got.PowerState != "Running" {
		t.Fatalf("PowerState = %q", got.PowerState)
	}
}

func TestDiskGenerationsFromUnstructuredDefaultsMissingValues(t *testing.T) {
	got := DiskGenerationsFromUnstructured(&unstructured.Unstructured{})
	if got.Root != 1 || got.NixStore != 1 {
		t.Fatalf("DiskGenerations = %#v, want root=1 nixStore=1", got)
	}
}

func TestListDevboxes(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		testListKinds(),
	)
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), newDevbox("team-a", "dev-a"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create dev-a: %v", err)
	}
	if _, err := client.Resource(Resource).Namespace("team-b").Create(context.Background(), newDevbox("team-b", "dev-b"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create dev-b: %v", err)
	}
	if _, err := client.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm dev-a: %v", err)
	}
	if _, err := client.Resource(VirtualMachineResource).Namespace("team-b").Create(context.Background(), newVM("team-b", "dev-b", "Stopped"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm dev-b: %v", err)
	}

	got, err := List(context.Background(), client, "team-a", false)
	if err != nil {
		t.Fatalf("list devboxes: %v", err)
	}
	if len(got) != 1 || got[0].Name != "dev-a" {
		t.Fatalf("devboxes = %#v", got)
	}
	if got[0].VMPrintableStatus != "Running" {
		t.Fatalf("VMPrintableStatus = %q", got[0].VMPrintableStatus)
	}

	got, err = List(context.Background(), client, "", true)
	if err != nil {
		t.Fatalf("list all devboxes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("devboxes = %#v", got)
	}
}

func TestResetDiskGenerations(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	devbox := newDevbox("team-a", "dev-a")
	if err := unstructured.SetNestedField(devbox.Object, int64(3), "spec", "diskGenerations", "root"); err != nil {
		t.Fatalf("set root generation: %v", err)
	}
	if err := unstructured.SetNestedField(devbox.Object, int64(5), "spec", "diskGenerations", "nixStore"); err != nil {
		t.Fatalf("set nixStore generation: %v", err)
	}
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), devbox, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}

	got, err := ResetDiskGenerations(context.Background(), client, "team-a", "dev-a", ResetDiskOptions{Root: true})
	if err != nil {
		t.Fatalf("ResetDiskGenerations root: %v", err)
	}
	if got.DiskGenerations.Root != 4 || got.DiskGenerations.NixStore != 5 {
		t.Fatalf("root reset generations = %#v", got.DiskGenerations)
	}

	got, err = ResetDiskGenerations(context.Background(), client, "team-a", "dev-a", ResetDiskOptions{NixStore: true})
	if err != nil {
		t.Fatalf("ResetDiskGenerations nixStore: %v", err)
	}
	if got.DiskGenerations.Root != 4 || got.DiskGenerations.NixStore != 6 {
		t.Fatalf("nixStore reset generations = %#v", got.DiskGenerations)
	}

	got, err = ResetDiskGenerations(context.Background(), client, "team-a", "dev-a", ResetDiskOptions{Root: true, NixStore: true})
	if err != nil {
		t.Fatalf("ResetDiskGenerations both: %v", err)
	}
	if got.DiskGenerations.Root != 5 || got.DiskGenerations.NixStore != 7 {
		t.Fatalf("both reset generations = %#v", got.DiskGenerations)
	}
}

func TestResetDiskGenerationsDefaultsMissingValues(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), newDevbox("team-a", "dev-a"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}

	got, err := ResetDiskGenerations(context.Background(), client, "team-a", "dev-a", ResetDiskOptions{Root: true})
	if err != nil {
		t.Fatalf("ResetDiskGenerations: %v", err)
	}
	if got.DiskGenerations.Root != 2 || got.DiskGenerations.NixStore != 1 {
		t.Fatalf("DiskGenerations = %#v, want root=2 nixStore=1", got.DiskGenerations)
	}
	obj, err := client.Resource(Resource).Namespace("team-a").Get(context.Background(), "dev-a", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get devbox: %v", err)
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", "diskGenerations", "nixStore"); !found {
		t.Fatalf("reset patch did not write nixStore generation: %#v", obj.Object)
	}
}

func TestResetDiskGenerationsRequiresSelectedDisk(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	_, err := ResetDiskGenerations(context.Background(), client, "team-a", "dev-a", ResetDiskOptions{})
	if err == nil {
		t.Fatal("expected selected disk error")
	}
	if !strings.Contains(err.Error(), "at least one disk") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizePorts(t *testing.T) {
	got, err := NormalizePorts([]int{22, 2222, 22})
	if err != nil {
		t.Fatalf("NormalizePorts: %v", err)
	}
	if want := []int{22, 2222}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ports = %#v, want %#v", got, want)
	}

	got, err = NormalizePorts(nil)
	if err != nil {
		t.Fatalf("NormalizePorts default: %v", err)
	}
	if want := []int{22}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default ports = %#v, want %#v", got, want)
	}

	if _, err := NormalizePorts([]int{0}); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestBuildObject(t *testing.T) {
	obj, err := BuildObject(CreateOptions{
		Namespace:               "team-a",
		Name:                    "dev-a",
		AuthorizedKeysConfigMap: "user-ssh-keys",
		ExposedPorts:            []int{22, 2222},
	})
	if err != nil {
		t.Fatalf("BuildObject: %v", err)
	}
	if obj.GetNamespace() != "team-a" {
		t.Fatalf("namespace = %q", obj.GetNamespace())
	}
	if obj.GetName() != "dev-a" {
		t.Fatalf("name = %q", obj.GetName())
	}
	if obj.GroupVersionKind().Kind != "Devbox" {
		t.Fatalf("kind = %q", obj.GroupVersionKind().Kind)
	}
	powerState, _, _ := unstructured.NestedString(obj.Object, "spec", "powerState")
	if powerState != "Running" {
		t.Fatalf("powerState = %q", powerState)
	}
	configMap, _, _ := unstructured.NestedString(obj.Object, "spec", "authorizedKeysConfigMapRef", "name")
	if configMap != "user-ssh-keys" {
		t.Fatalf("authorizedKeysConfigMapRef.name = %q", configMap)
	}
	ports, _, _ := unstructured.NestedSlice(obj.Object, "spec", "exposedPorts")
	if want := []any{int64(22), int64(2222)}; !reflect.DeepEqual(ports, want) {
		t.Fatalf("exposedPorts = %#v, want %#v", ports, want)
	}
}

func TestCreateDevbox(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())

	got, err := Create(context.Background(), client, CreateOptions{
		Namespace:               "team-a",
		Name:                    "dev-a",
		AuthorizedKeysConfigMap: "user-ssh-keys",
		ExposedPorts:            []int{22},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Namespace != "team-a" || got.Name != "dev-a" {
		t.Fatalf("created devbox = %#v", got)
	}

	obj, err := client.Resource(Resource).Namespace("team-a").Get(context.Background(), "dev-a", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get created object: %v", err)
	}
	configMap, _, _ := unstructured.NestedString(obj.Object, "spec", "authorizedKeysConfigMapRef", "name")
	if configMap != "user-ssh-keys" {
		t.Fatalf("authorizedKeysConfigMapRef.name = %q", configMap)
	}
}

func TestDeleteDevbox(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), newDevbox("team-a", "dev-a"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}

	if err := Delete(context.Background(), client, "team-a", "dev-a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := client.Resource(Resource).Namespace("team-a").Get(context.Background(), "dev-a", metav1.GetOptions{}); err == nil {
		t.Fatal("expected devbox to be deleted")
	}

	if err := Delete(context.Background(), client, "team-a", "dev-a"); err == nil {
		t.Fatal("expected missing devbox error")
	}
}

func TestSetPowerState(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), newDevbox("team-a", "dev-a"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}

	got, err := SetPowerState(context.Background(), client, "team-a", "dev-a", "Stopped")
	if err != nil {
		t.Fatalf("SetPowerState: %v", err)
	}
	if got.PowerState != "Stopped" {
		t.Fatalf("PowerState = %q", got.PowerState)
	}
	obj, err := client.Resource(Resource).Namespace("team-a").Get(context.Background(), "dev-a", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get devbox: %v", err)
	}
	powerState, _, _ := unstructured.NestedString(obj.Object, "spec", "powerState")
	if powerState != "Stopped" {
		t.Fatalf("stored powerState = %q", powerState)
	}
	if _, err := SetPowerState(context.Background(), client, "team-a", "dev-a", "Suspended"); err == nil {
		t.Fatal("expected invalid power state error")
	}
}

func TestWaitForRunning(t *testing.T) {
	obj := newDevbox("team-a", "dev-a")
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		testListKinds(),
	)
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := client.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Running"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	var updates int
	got, err := WaitForRunning(context.Background(), client, "team-a", "dev-a", WaitOptions{
		Timeout:      time.Second,
		PollInterval: time.Millisecond,
		OnUpdate: func(Devbox, time.Duration) {
			updates++
		},
	})
	if err != nil {
		t.Fatalf("WaitForRunning: %v", err)
	}
	if got.VMPrintableStatus != "Running" {
		t.Fatalf("VMPrintableStatus = %q", got.VMPrintableStatus)
	}
	if updates == 0 {
		t.Fatal("expected status updates")
	}
}

func TestWaitForVMStatus(t *testing.T) {
	obj := newDevbox("team-a", "dev-a")
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		testListKinds(),
	)
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := client.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Stopped"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	got, err := WaitForVMStatus(context.Background(), client, "team-a", "dev-a", "Stopped", WaitOptions{
		Timeout:      time.Second,
		PollInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("WaitForVMStatus: %v", err)
	}
	if got.VMPrintableStatus != "Stopped" {
		t.Fatalf("VMPrintableStatus = %q", got.VMPrintableStatus)
	}
}

func TestWaitForRunningTimesOut(t *testing.T) {
	obj := newDevbox("team-a", "dev-a")
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		testListKinds(),
	)
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}
	if _, err := client.Resource(VirtualMachineResource).Namespace("team-a").Create(context.Background(), newVM("team-a", "dev-a", "Provisioning"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create vm: %v", err)
	}

	got, err := WaitForRunning(context.Background(), client, "team-a", "dev-a", WaitOptions{
		Timeout:      5 * time.Millisecond,
		PollInterval: time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out waiting") {
		t.Fatalf("error = %v", err)
	}
	if got.VMPrintableStatus != "Provisioning" {
		t.Fatalf("last status = %#v", got)
	}
}

func TestWaitForDeleted(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())

	if err := WaitForDeleted(context.Background(), client, "team-a", "dev-a", WaitOptions{
		Timeout:      time.Second,
		PollInterval: time.Millisecond,
	}); err != nil {
		t.Fatalf("WaitForDeleted: %v", err)
	}
}

func TestWaitForDeletedTimesOut(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := client.Resource(Resource).Namespace("team-a").Create(context.Background(), newDevbox("team-a", "dev-a"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create devbox: %v", err)
	}

	err := WaitForDeleted(context.Background(), client, "team-a", "dev-a", WaitOptions{
		Timeout:      5 * time.Millisecond,
		PollInterval: time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out waiting") {
		t.Fatalf("error = %v", err)
	}
}

func TestFilterByOwner(t *testing.T) {
	devboxes := []Devbox{
		{Name: "mine", Owner: "oidc:user@example.com"},
		{Name: "theirs", Owner: "oidc:other@example.com"},
		{Name: "unowned"},
	}

	got := FilterByOwner(devboxes, "oidc:user@example.com")
	if len(got) != 1 {
		t.Fatalf("filtered devboxes = %#v", got)
	}
	if got[0].Name != "mine" {
		t.Fatalf("Name = %q", got[0].Name)
	}
}

func TestWriteTableOmitsOwnerByDefault(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTable(&buf, []Devbox{
		{
			Namespace:         "team-a",
			Name:              "dev-a",
			Owner:             "oidc:user@example.com",
			PowerState:        "Running",
			VMPrintableStatus: "Running",
			Address:           "10.0.0.10",
		},
	}, false, false)
	if err != nil {
		t.Fatalf("write table: %v", err)
	}
	output := buf.String()
	for _, want := range []string{"NAME", "dev-a", "Running", "10.0.0.10"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
	for _, unwanted := range []string{"OWNER", "oidc:user@example.com"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("output %q unexpectedly contains %q", output, unwanted)
		}
	}
}

func TestWriteTableIncludesOwnerWhenRequested(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTable(&buf, []Devbox{
		{
			Namespace:         "team-a",
			Name:              "dev-a",
			Owner:             "oidc:user@example.com",
			PowerState:        "Running",
			VMPrintableStatus: "Running",
			Address:           "10.0.0.10",
		},
	}, true, true)
	if err != nil {
		t.Fatalf("write table: %v", err)
	}
	output := buf.String()
	for _, want := range []string{"NAMESPACE", "team-a", "dev-a", "oidc:user@example.com", "10.0.0.10"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestWriteSummaryUsesDevboxPorts(t *testing.T) {
	var buf bytes.Buffer
	err := WriteSummary(&buf, Devbox{
		Namespace:               "team-a",
		Name:                    "dev-a",
		AuthorizedKeysConfigMap: "user-ssh-keys",
		ExposedPorts:            []int{22, 8080},
		VMPrintableStatus:       "Running",
	}, nil)
	if err != nil {
		t.Fatalf("write summary: %v", err)
	}
	output := buf.String()
	for _, want := range []string{"SSHKeys:", "user-ssh-keys", "Ports:", "22,8080"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func newDevbox(namespace, name string) *unstructured.Unstructured {
	item := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "platform.salami.network/v1",
			"kind":       "Devbox",
			"spec": map[string]any{
				"powerState":   "Running",
				"exposedPorts": []any{int64(22)},
			},
		},
	}
	item.SetGroupVersionKind(Resource.GroupVersion().WithKind("Devbox"))
	item.SetNamespace(namespace)
	item.SetName(name)
	item.SetCreationTimestamp(metav1.Now())
	return item
}

func newVM(namespace, name string, printableStatus string) *unstructured.Unstructured {
	item := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]any{
				"labels": map[string]any{
					DevboxLabel: name,
				},
			},
			"status": map[string]any{
				"printableStatus": printableStatus,
			},
		},
	}
	item.SetGroupVersionKind(VirtualMachineResource.GroupVersion().WithKind("VirtualMachine"))
	item.SetNamespace(namespace)
	item.SetName(name)
	return item
}

func testListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		Resource:               "DevboxList",
		VirtualMachineResource: "VirtualMachineList",
	}
}
