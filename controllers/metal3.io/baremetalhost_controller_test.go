package controllers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	promutil "github.com/prometheus/client_golang/prometheus/testutil"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	"github.com/metal3-io/baremetal-operator/pkg/utils"
)

const (
	namespace         string = "test-namespace"
	defaultSecretName string = "bmc-creds-valid" // #nosec
	statusAnnotation  string = `{"operationalStatus":"OK","lastUpdated":"2020-04-15T15:00:50Z","hardwareProfile":"StatusProfile","hardware":{"systemVendor":{"manufacturer":"QEMU","productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6","ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true},{"name":"eth1","model":"0x1af4  0x0001","mac":"00:b7:8b:bb:3d:f8","ip":"192.168.111.20","speedGbps":0,"vlanId":0,"pxe":false}],"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,"flags":["aes","apic","arat","avx","clflush","cmov","constant_tsc","cx16","cx8","de","eagerfpu","ept","erms","f16c","flexpriority","fpu","fsgsbase","fxsr","hypervisor","lahf_lm","lm","mca","mce","mmx","msr","mtrr","nopl","nx","pae","pat","pclmulqdq","pge","pni","popcnt","pse","pse36","rdrand","rdtscp","rep_good","sep","smep","sse","sse2","sse4_1","sse4_2","ssse3","syscall","tpr_shadow","tsc","tsc_adjust","tsc_deadline_timer","vme","vmx","vnmi","vpid","x2apic","xsave","xsaveopt","xtopology"],"count":4},"hostname":"node-0"},"provisioning":{"state":"provisioned","ID":"8a0ede17-7b87-44ac-9293-5b7d50b94b08","image":{"url":"bar","checksum":""}},"goodCredentials":{"credentials":{"name":"node-0-bmc-secret","namespace":"metal3"},"credentialsVersion":"879"},"triedCredentials":{"credentials":{"name":"node-0-bmc-secret","namespace":"metal3"},"credentialsVersion":"879"},"errorMessage":"","poweredOn":true,"operationHistory":{"register":{"start":"2020-04-15T12:06:26Z","end":"2020-04-15T12:07:12Z"},"inspect":{"start":"2020-04-15T12:07:12Z","end":"2020-04-15T12:09:29Z"},"provision":{"start":null,"end":null},"deprovision":{"start":null,"end":null}}}`
	hwdAnnotation     string = `{"systemVendor":{"manufacturer":"QEMU","productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6","ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true}],"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,"flags":["foo"],"count":4},"hostname":"hwdAnnotation-0"}`
)

func newSecret(name string, data map[string]string) *corev1.Secret {
	secretData := make(map[string][]byte)
	for k, v := range data {
		secretData[k] = []byte(base64.StdEncoding.EncodeToString([]byte(v)))
	}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: secretData,
	}

	return secret
}

func newBMCCredsSecret(name, username, password string) *corev1.Secret {
	return newSecret(name, map[string]string{"username": username, "password": password})
}

func newHost(name string, spec *metal3api.BareMetalHostSpec) *metal3api.BareMetalHost {
	return &metal3api.BareMetalHost{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BareMetalHost",
			APIVersion: "metal3.io/v1alpha1",
		}, ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: *spec,
	}
}

func newHostFirmwareSettings(host *metal3api.BareMetalHost, conditions []metav1.Condition) *metal3api.HostFirmwareSettings {
	hfs := &metal3api.HostFirmwareSettings{
		ObjectMeta: metav1.ObjectMeta{
			Name:      host.Name,
			Namespace: host.Namespace,
		},
		Spec: metal3api.HostFirmwareSettingsSpec{
			Settings: metal3api.DesiredSettingsMap{
				"ProcVirtualization": intstr.FromString("Disabled"),
				"SecureBoot":         intstr.FromString("Enabled"),
			},
		},
		Status: metal3api.HostFirmwareSettingsStatus{
			Settings: metal3api.SettingsMap{
				"ProcVirtualization": "Disabled",
				"SecureBoot":         "Enabled",
			},
		},
	}

	hfs.Status.Conditions = conditions

	return hfs
}

func newDefaultNamedHost(t *testing.T, name string) *metal3api.BareMetalHost {
	t.Helper()
	spec := &metal3api.BareMetalHostSpec{
		BMC: metal3api.BMCDetails{
			Address:         "ipmi://192.168.122.1:6233",
			CredentialsName: defaultSecretName,
		},
		HardwareProfile: "libvirt",
		RootDeviceHints: &metal3api.RootDeviceHints{
			DeviceName:         "userd_devicename",
			HCTL:               "1:2:3:4",
			Model:              "userd_model",
			Vendor:             "userd_vendor",
			SerialNumber:       "userd_serial",
			MinSizeGigabytes:   40,
			WWN:                "userd_wwn",
			WWNWithExtension:   "userd_with_extension",
			WWNVendorExtension: "userd_vendor_extension",
		},
	}
	t.Logf("newNamedHost(%s)", name)
	host := newHost(name, spec)
	host.Status.HardwareProfile = "libvirt"
	return host
}

func newDefaultHost(t *testing.T) *metal3api.BareMetalHost {
	t.Helper()
	return newDefaultNamedHost(t, t.Name())
}

func newTestReconcilerWithFixture(fix *fixture.Fixture, initObjs ...runtime.Object) *BareMetalHostReconciler {
	clientBuilder := fakeclient.NewClientBuilder().WithRuntimeObjects(initObjs...)
	for _, v := range initObjs {
		clientBuilder = clientBuilder.WithStatusSubresource(v.(client.Object))
	}
	c := clientBuilder.Build()
	// Add a default secret that can be used by most hosts.
	bmcSecret := newBMCCredsSecret(defaultSecretName, "User", "Pass")
	c.Create(context.TODO(), bmcSecret)

	return &BareMetalHostReconciler{
		Client:             c,
		ProvisionerFactory: fix,
		Log:                ctrl.Log.WithName("controllers").WithName("BareMetalHost"),
		APIReader:          c,
	}
}

func newTestReconciler(initObjs ...runtime.Object) *BareMetalHostReconciler {
	fix := fixture.Fixture{}
	return newTestReconcilerWithFixture(&fix, initObjs...)
}

type DoneFunc func(host *metal3api.BareMetalHost, result reconcile.Result) bool

func newRequest(host *metal3api.BareMetalHost) ctrl.Request {
	namespacedName := types.NamespacedName{
		Namespace: host.ObjectMeta.Namespace,
		Name:      host.ObjectMeta.Name,
	}
	return ctrl.Request{NamespacedName: namespacedName}
}

func tryReconcile(t *testing.T, r *BareMetalHostReconciler, host *metal3api.BareMetalHost, isDone DoneFunc) {
	t.Helper()
	request := newRequest(host)

	for i := 0; ; i++ {
		t.Logf("tryReconcile: top of loop %d", i)
		if i >= 25 {
			t.Fatal("Exceeded 25 iterations")
		}

		result, err := r.Reconcile(context.Background(), request)

		if err != nil {
			t.Fatal(err)
		}

		// The FakeClient keeps a copy of the object we update, so we
		// need to replace the one we have with the updated data in
		// order to test it. In case it was not found, let's set it to nil
		updatedHost := &metal3api.BareMetalHost{}
		if err = r.Get(context.TODO(), request.NamespacedName, updatedHost); k8serrors.IsNotFound(err) {
			host = nil
		} else {
			updatedHost.DeepCopyInto(host)
		}

		if isDone(host, result) {
			t.Logf("tryReconcile: loop done %d", i)
			break
		}

		t.Logf("tryReconcile: loop bottom %d result=%v", i, result)
		if !result.Requeue && result.RequeueAfter == 0 {
			t.Fatalf("Ended reconcile at iteration %d without test condition being true", i)
		}
	}
}

func waitForStatus(t *testing.T, r *BareMetalHostReconciler, host *metal3api.BareMetalHost, desiredStatus metal3api.OperationalStatus) {
	t.Helper()
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			state := host.OperationalStatus()
			t.Logf("Waiting for status %s: %s", desiredStatus, state)
			return state == desiredStatus
		},
	)
}

func waitForError(t *testing.T, r *BareMetalHostReconciler, host *metal3api.BareMetalHost) {
	t.Helper()
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Waiting for error: %q", host.Status.ErrorMessage)
			return host.Status.ErrorMessage != ""
		},
	)
}

func waitForNoError(t *testing.T, r *BareMetalHostReconciler, host *metal3api.BareMetalHost) {
	t.Helper()
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Waiting for no error message: %q", host.Status.ErrorMessage)
			return host.Status.ErrorMessage == ""
		},
	)
}

func waitForProvisioningState(t *testing.T, r *BareMetalHostReconciler, host *metal3api.BareMetalHost, desiredState metal3api.ProvisioningState) {
	t.Helper()
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Waiting for state %q have state %q", desiredState, host.Status.Provisioning.State)
			return host.Status.Provisioning.State == desiredState
		},
	)
}

// TestHardwareDetails_EmptyStatus ensures that hardware details in
// the status field are populated when the hardwaredetails annotation
// is present and no existing HarwareDetails are present.
func TestHardwareDetails_EmptyStatus(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.HardwareDetailsAnnotation: hwdAnnotation,
	}

	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			_, found := host.Annotations[metal3api.HardwareDetailsAnnotation]
			if host.Status.HardwareDetails != nil && host.Status.HardwareDetails.Hostname == "hwdAnnotation-0" && !found {
				return true
			}
			return false
		},
	)
}

// TestHardwareDetails_StatusPresent ensures that hardware details in
// the hardwaredetails annotation is ignored with existing Status.
func TestHardwareDetails_StatusPresent(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.HardwareDetailsAnnotation: hwdAnnotation,
	}
	time := metav1.Now()
	host.Status.LastUpdated = &time
	hwd := metal3api.HardwareDetails{}
	hwd.Hostname = "existinghost"
	host.Status.HardwareDetails = &hwd

	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			_, found := host.Annotations[metal3api.HardwareDetailsAnnotation]
			if host.Status.HardwareDetails != nil && host.Status.HardwareDetails.Hostname == "existinghost" && !found {
				return true
			}
			return false
		},
	)
}

// TestHardwareDetails_StatusPresentInspectDisabled ensures that
// hardware details in the hardwaredetails annotation are consumed
// even when existing status exists, when inspection is disabled.
func TestHardwareDetails_StatusPresentInspectDisabled(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.InspectAnnotationPrefix:   "disabled",
		metal3api.HardwareDetailsAnnotation: hwdAnnotation,
	}
	time := metav1.Now()
	host.Status.LastUpdated = &time
	hwd := metal3api.HardwareDetails{}
	hwd.Hostname = "existinghost"
	host.Status.HardwareDetails = &hwd

	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			_, found := host.Annotations[metal3api.HardwareDetailsAnnotation]
			if host.Status.HardwareDetails != nil && host.Status.HardwareDetails.Hostname == "hwdAnnotation-0" && !found {
				return true
			}
			return false
		},
	)
}

// TestHardwareDetails_Invalid
// Tests scenario where the hardwaredetails value is invalid.
func TestHardwareDetails_Invalid(t *testing.T) {
	host := newDefaultHost(t)
	badAnnotation := fmt.Sprintf("{\"hardware\": %s}", hwdAnnotation)
	host.Annotations = map[string]string{
		metal3api.InspectAnnotationPrefix:   "disabled",
		metal3api.HardwareDetailsAnnotation: badAnnotation,
	}
	time := metav1.Now()
	host.Status.LastUpdated = &time
	hwd := metal3api.HardwareDetails{}
	hwd.Hostname = "existinghost"
	host.Status.HardwareDetails = &hwd

	r := newTestReconciler(host)
	request := newRequest(host)
	_, err := r.Reconcile(context.Background(), request)
	expectedErr := "json: unknown field"
	assert.Contains(t, err.Error(), expectedErr)
}

// TestStatusAnnotation_EmptyStatus ensures that status is manually populated
// when status annotation is present and status field is empty.
func TestStatusAnnotation_EmptyStatus(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.StatusAnnotation: statusAnnotation,
	}
	host.Spec.Online = true
	host.Spec.Image = &metal3api.Image{URL: "foo", Checksum: "123"}

	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			if host.Status.HardwareProfile == "StatusProfile" && host.Status.Provisioning.Image.URL == "bar" {
				return true
			}
			return false
		},
	)
}

// TestStatusAnnotation_StatusPresent tests that if status is present
// status annotation is ignored and deleted.
func TestStatusAnnotation_StatusPresent(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.StatusAnnotation: statusAnnotation,
	}

	host.Spec.Online = true
	time := metav1.Now()
	host.Status.LastUpdated = &time
	host.Status.Provisioning.Image = metal3api.Image{URL: "foo", Checksum: "123"}
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			_, found := host.Annotations[metal3api.StatusAnnotation]
			if host.Status.HardwareProfile != "StatusProfile" && host.Status.Provisioning.Image.URL == "foo" && !found {
				return true
			}
			return false
		},
	)
}

// TestStatusAnnotation_Partial ensures that if the status annotation
// does not include the LastUpdated value reconciliation does not go
// into an infinite loop.
func TestStatusAnnotation_Partial(t *testing.T) {
	// Build a version of the annotation text that does not include
	// a LastUpdated value.
	unpackedStatus, err := unmarshalStatusAnnotation([]byte(statusAnnotation))
	if err != nil {
		t.Fatal(err)
		return
	}
	unpackedStatus.LastUpdated = nil
	packedStatus, err := json.Marshal(unpackedStatus)
	if err != nil {
		t.Fatal(err)
		return
	}

	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.StatusAnnotation: string(packedStatus),
	}
	host.Spec.Online = true
	host.Spec.Image = &metal3api.Image{URL: "foo", Checksum: "123"}

	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			if host.Status.HardwareProfile == "StatusProfile" && host.Status.Provisioning.Image.URL == "bar" {
				return true
			}
			return false
		},
	)
}

// TestHardwareDataExist ensures hardwareData takes precedence over
// statusAnnotation when updating during BareMetalHost status.
func TestHardwareDataExist(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.StatusAnnotation: statusAnnotation,
	}
	host.Spec.Online = true

	hardwareData := &metal3api.HardwareData{
		ObjectMeta: metav1.ObjectMeta{
			Name:      host.Name,
			Namespace: host.Namespace,
		},
		Spec: metal3api.HardwareDataSpec{
			HardwareDetails: &metal3api.HardwareDetails{
				CPU: metal3api.CPU{
					Model: "fake-model",
				},
				Hostname: host.Name,
			},
		},
	}

	r := newTestReconciler(host, hardwareData)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			_, found := host.Annotations[metal3api.StatusAnnotation]
			if found && host.Status.HardwareDetails.CPU.Model != "fake-model" {
				return false
			}
			return true
		},
	)
}

// TestPause ensures that the requeue happens when the pause annotation is there.
func TestPause(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.PausedAnnotation: "true",
	}
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			// Because the host is created with the annotation, we
			// expect it to never have any reconciling done at all, so
			// it has no finalizer.
			t.Logf("requeue: %v  finalizers: %v", result.Requeue, host.Finalizers)
			if !result.Requeue && len(host.Finalizers) == 0 {
				return true
			}
			return false
		},
	)
}

// TestInspectDisabled ensures that Inspection is skipped when disabled.
func TestInspectDisabled(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.InspectAnnotationPrefix: "disabled",
	}
	r := newTestReconciler(host)
	waitForProvisioningState(t, r, host, metal3api.StatePreparing)
	assert.Nil(t, host.Status.HardwareDetails)
}

// TestInspectEnabled ensures that Inspection is completed when not disabled.
func TestInspectEnabled(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)
	waitForProvisioningState(t, r, host, metal3api.StatePreparing)
	assert.NotNil(t, host.Status.HardwareDetails)
}

// TestAddFinalizers ensures that the finalizers for the host are
// updated as part of reconciling it.
func TestAddFinalizers(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("host finalizers: %v", host.ObjectMeta.Finalizers)
			if !utils.StringInList(host.ObjectMeta.Finalizers, metal3api.BareMetalHostFinalizer) {
				return false
			}

			hostSecret := getHostSecret(t, r, host)
			t.Logf("BMC secret finalizers: %v", hostSecret.ObjectMeta.Finalizers)

			return utils.StringInList(hostSecret.ObjectMeta.Finalizers, secretutils.SecretsFinalizer)
		},
	)
}

// TestDoNotAddSecretFinalizersDuringDelete verifies that during a host deletion,
// in case the removal of a secret finalizer triggers an immediate reconcile loop,
// then the secret finalizer is not added again.
func TestDoNotAddSecretFinalizersDuringDelete(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)

	// Let the host reach the available state before deleting it
	waitForProvisioningState(t, r, host, metal3api.StateAvailable)
	err := doDeleteHost(host, r)
	assert.NoError(t, err)
	waitForProvisioningState(t, r, host, metal3api.StateDeleting)

	// The next reconcile loop will start the delete process,
	// and as a first step the Ironic node will be removed
	request := newRequest(host)
	_, err = r.Reconcile(context.Background(), request)
	assert.NoError(t, err)

	// The next reconcile loop remove the finalizers from
	// both the host and the secret.
	// The fake client will immediately remove the host
	// from its cache, so let's keep the latest updated
	// host
	r.Get(context.TODO(), request.NamespacedName, host)
	_, err = r.Reconcile(context.Background(), request)
	assert.NoError(t, err)

	// To simulate an immediate reconciliation loop due the
	// secret update (and a slow host deletion), let's push
	// back the host in the client cache.
	host.ResourceVersion = ""
	r.Client.Create(context.TODO(), host)
	previousSecret := getHostSecret(t, r, host)
	_, err = r.Reconcile(context.Background(), request)
	assert.NoError(t, err)

	// Secret must remain unchanged
	actualSecret := getHostSecret(t, r, host)
	assert.Empty(t, actualSecret.Finalizers)
	assert.Equal(t, previousSecret.ResourceVersion, actualSecret.ResourceVersion)
}

// TestSetLastUpdated ensures that the lastUpdated timestamp in the
// status is set to a non-zero value during reconciliation.
func TestSetLastUpdated(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("LastUpdated: %v", host.Status.LastUpdated)
			return !host.Status.LastUpdated.IsZero()
		},
	)
}

func TestInspectionDisabledAnnotation(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = make(map[string]string)

	assert.False(t, inspectionDisabled(host))

	host.Annotations[metal3api.InspectAnnotationPrefix] = "disabled"
	assert.True(t, inspectionDisabled(host))
}

func makeReconcileInfo(host *metal3api.BareMetalHost) *reconcileInfo {
	return &reconcileInfo{
		log:  logf.Log.WithName("controllers").WithName("BareMetalHost").WithName("baremetal_controller"),
		host: host,
	}
}

func TestHasRebootAnnotation(t *testing.T) {
	testCases := []struct {
		Scenario       string
		Annotations    map[string]string
		expectForce    bool
		expectedReboot bool
		expectedMode   metal3api.RebootMode
	}{
		{
			Scenario: "No annotations",
		},
		{
			Scenario: "Simple with empty value",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix: "",
			},
			expectedReboot: true,
		},
		{
			Scenario: "Suffixed with empty value",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix + "/foo": "",
			},
			expectedReboot: true,
		},
		{
			Scenario: "Two suffixed with empty value",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix + "/foo": "",
				metal3api.RebootAnnotationPrefix + "/bar": "",
			},
			expectedReboot: true,
		},
		{
			Scenario: "Suffixed with soft reboot",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix + "/foo": "{\"mode\": \"soft\"}",
			},
			expectedReboot: true,
			expectedMode:   metal3api.RebootModeSoft,
		},
		{
			Scenario: "Suffixed with hard reboot",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix + "/foo": "{\"mode\": \"hard\"}",
			},
			expectedReboot: true,
			expectedMode:   metal3api.RebootModeHard,
		},
		{
			Scenario: "Suffixed with bad JSON",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix + "/foo": "{\"bad\"= \"json\"]",
			},
			expectedReboot: true,
		},
		{
			Scenario: "Two suffixed with different values",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix + "/foo": "{\"mode\": \"hard\"}",
				metal3api.RebootAnnotationPrefix + "/bar": "{\"mode\": \"soft\"}",
			},
			expectedReboot: true,
			expectedMode:   metal3api.RebootModeHard,
		},
		{
			Scenario: "Suffixed with force",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix + "/foo": "{\"force\": true}",
			},
			expectForce:    true,
			expectedReboot: true,
		},
		{
			Scenario: "Expect force",
			Annotations: map[string]string{
				metal3api.RebootAnnotationPrefix + "/foo": "{\"mode\": \"hard\"}",
			},
			expectForce:    true,
			expectedReboot: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			host := newDefaultHost(t)
			info := makeReconcileInfo(host)
			host.Annotations = tc.Annotations

			if tc.expectedMode == "" {
				tc.expectedMode = metal3api.RebootModeSoft
			}

			hasReboot, rebootMode := hasRebootAnnotation(info, tc.expectForce)
			assert.Equal(t, tc.expectedReboot, hasReboot)
			assert.Equal(t, tc.expectedMode, rebootMode)
		})
	}
}

// TestRebootWithSuffixlessAnnotation tests full reboot cycle with suffixless
// annotation which doesn't wait for annotation removal before power on.
func TestRebootWithSuffixlessAnnotation(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = make(map[string]string)
	host.Annotations[metal3api.RebootAnnotationPrefix] = ""
	host.Status.PoweredOn = true
	host.Status.Provisioning.State = metal3api.StateProvisioned
	host.Spec.Online = true
	host.Spec.Image = &metal3api.Image{URL: "foo", Checksum: "123"}
	host.Spec.Image.URL = "foo"
	host.Status.Provisioning.Image.URL = "foo"

	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return !host.Status.PoweredOn
		},
	)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			if _, exists := host.Annotations[metal3api.RebootAnnotationPrefix]; exists {
				return false
			}

			return true
		},
	)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return host.Status.PoweredOn
		},
	)

	// make sure we don't go into another reboot
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return host.Status.PoweredOn
		},
	)
}

// TestRebootWithSuffixedAnnotation tests a full reboot cycle, with suffixed annotation
// to verify that controller holds power off until annotation removal.
func TestRebootWithSuffixedAnnotation(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = make(map[string]string)
	annotation := metal3api.RebootAnnotationPrefix + "/foo"
	host.Annotations[annotation] = ""
	host.Status.PoweredOn = true
	host.Status.Provisioning.State = metal3api.StateProvisioned
	host.Spec.Online = true
	host.Spec.Image = &metal3api.Image{URL: "foo", Checksum: "123"}
	host.Spec.Image.URL = "foo"
	host.Status.Provisioning.Image.URL = "foo"

	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return !host.Status.PoweredOn
		},
	)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			// we expect that the machine will be powered off until we remove annotation
			return !host.Status.PoweredOn
		},
	)

	delete(host.Annotations, annotation)
	r.Update(context.TODO(), host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return host.Status.PoweredOn
		},
	)

	// make sure we don't go into another reboot
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return host.Status.PoweredOn
		},
	)
}

func getHostSecret(t *testing.T, r *BareMetalHostReconciler, host *metal3api.BareMetalHost) (secret *corev1.Secret) {
	t.Helper()
	secret = &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: host.Namespace,
		Name:      host.Spec.BMC.CredentialsName,
	}
	err := r.Get(context.TODO(), secretName, secret)
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func TestSecretUpdateOwnerRefAndEnvironmentLabelOnStartup(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)

	secret := getHostSecret(t, r, host)
	assert.Empty(t, secret.OwnerReferences)
	assert.Empty(t, secret.Labels)

	waitForProvisioningState(t, r, host, metal3api.StateInspecting)

	secret = getHostSecret(t, r, host)
	assert.Equal(t, host.Name, secret.OwnerReferences[0].Name)
	assert.Equal(t, "BareMetalHost", secret.OwnerReferences[0].Kind)
	assert.Nil(t, secret.OwnerReferences[0].Controller)
	assert.Nil(t, secret.OwnerReferences[0].BlockOwnerDeletion)
	assert.Equal(t, "baremetal", secret.Labels["environment.metal3.io"])
}

// TestUpdateCredentialsSecretSuccessFields ensures that the
// GoodCredentials fields are updated in the status block of a host
// when the secret used exists and has all of the right fields.
func TestUpdateCredentialsSecretSuccessFields(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("ref: %v ver: %s", host.Status.GoodCredentials.Reference,
				host.Status.GoodCredentials.Version)
			return host.Status.GoodCredentials.Version != ""
		},
	)
}

// TestUpdateGoodCredentialsOnNewSecret ensures that the
// GoodCredentials fields are updated when the secret for a host is
// changed to another secret that is also good.
func TestUpdateGoodCredentialsOnNewSecret(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("ref: %v ver: %s", host.Status.GoodCredentials.Reference,
				host.Status.GoodCredentials.Version)
			return host.Status.GoodCredentials.Version != ""
		},
	)

	// Define a second valid secret and update the host to use it.
	secret2 := newBMCCredsSecret("bmc-creds-valid2", "User", "Pass")
	err := r.Create(context.TODO(), secret2)
	if err != nil {
		t.Fatal(err)
	}

	host.Spec.BMC.CredentialsName = "bmc-creds-valid2"
	err = r.Update(context.TODO(), host)
	if err != nil {
		t.Fatal(err)
	}

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("ref: %v ver: %s", host.Status.GoodCredentials.Reference,
				host.Status.GoodCredentials.Version)
			if host.Status.GoodCredentials.Reference != nil && host.Status.GoodCredentials.Reference.Name == "bmc-creds-valid2" {
				return true
			}
			return false
		},
	)
}

// TestUpdateGoodCredentialsOnBadSecret ensures that the
// GoodCredentials fields are *not* updated when the secret is changed
// to one that is missing data.
func TestUpdateGoodCredentialsOnBadSecret(t *testing.T) {
	host := newDefaultHost(t)
	badSecret := newBMCCredsSecret("bmc-creds-no-user", "", "Pass")
	r := newTestReconciler(host, badSecret)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("ref: %v ver: %s", host.Status.GoodCredentials.Reference,
				host.Status.GoodCredentials.Version)
			return host.Status.GoodCredentials.Version != ""
		},
	)

	host.Spec.BMC.CredentialsName = "bmc-creds-no-user"
	err := r.Update(context.TODO(), host)
	if err != nil {
		t.Fatal(err)
	}

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("ref: %v ver: %s", host.Status.GoodCredentials.Reference,
				host.Status.GoodCredentials.Version)
			if host.Spec.BMC.CredentialsName != "bmc-creds-no-user" {
				return false
			}
			if host.Status.GoodCredentials.Reference != nil && host.Status.GoodCredentials.Reference.Name == "bmc-creds-valid" {
				return true
			}
			return false
		},
	)
}

// TestDiscoveredHost ensures that a host without a BMC IP and
// credentials is placed into the "discovered" state.
func TestDiscoveredHost(t *testing.T) {
	noAddressOrSecret := newHost("missing-bmc-address",
		&metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address:         "",
				CredentialsName: "",
			},
		})
	r := newTestReconciler(noAddressOrSecret)
	waitForStatus(t, r, noAddressOrSecret, metal3api.OperationalStatusDiscovered)
	if noAddressOrSecret.Status.ErrorType != "" {
		t.Errorf("Unexpected error type %s", noAddressOrSecret.Status.ErrorType)
	}
	if noAddressOrSecret.Status.ErrorMessage != "" {
		t.Errorf("Unexpected error message %s", noAddressOrSecret.Status.ErrorMessage)
	}
}

// TestMissingBMCParameters ensures that a host that is missing some
// of the required BMC settings is put into an error state.
func TestMissingBMCParameters(t *testing.T) {
	testCases := []struct {
		Scenario string
		Secret   *corev1.Secret
		Host     *metal3api.BareMetalHost
	}{
		{
			Scenario: "secret without username",
			Secret:   newBMCCredsSecret("bmc-creds-no-user", "", "Pass"),
			Host: newHost("missing-bmc-username",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: "bmc-creds-no-user",
					},
				}),
		},

		{
			Scenario: "secret without password",
			Secret:   newBMCCredsSecret("bmc-creds-no-pass", "User", ""),
			Host: newHost("missing-bmc-password",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: "bmc-creds-no-pass",
					},
				}),
		},

		{
			Scenario: "missing address",
			Secret:   newBMCCredsSecret("bmc-creds-ok", "User", "Pass"),
			Host: newHost("missing-bmc-address",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "",
						CredentialsName: "bmc-creds-ok",
					},
				}),
		},

		{
			Scenario: "missing secret",
			Secret:   newBMCCredsSecret("bmc-creds-ok", "User", "Pass"),
			Host: newHost("missing-bmc-credentials-ref",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: "",
					},
				}),
		},

		{
			Scenario: "no such secret",
			Secret:   newBMCCredsSecret("bmc-creds-ok", "User", "Pass"),
			Host: newHost("non-existent-bmc-secret-ref",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: "this-secret-does-not-exist",
					},
				}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			r := newTestReconciler(tc.Secret, tc.Host)
			waitForError(t, r, tc.Host)
			if tc.Host.Status.OperationalStatus != metal3api.OperationalStatusError {
				t.Errorf("Unexpected operational status %s", tc.Host.Status.OperationalStatus)
			}
			if tc.Host.Status.ErrorType != metal3api.RegistrationError {
				t.Errorf("Unexpected error type %s", tc.Host.Status.ErrorType)
			}
		})
	}
}

// TestFixSecret ensures that when the secret for a host is updated to
// be correct the status of the host moves out of the error state.
func TestFixSecret(t *testing.T) {
	secret := newBMCCredsSecret("bmc-creds-no-user", "", "Pass")
	host := newHost("fix-secret",
		&metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address:         "ipmi://192.168.122.1:6233",
				CredentialsName: "bmc-creds-no-user",
			},
		})
	r := newTestReconciler(host, secret)
	waitForError(t, r, host)

	secret = &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: namespace,
		Name:      "bmc-creds-no-user",
	}
	err := r.Get(context.TODO(), secretName, secret)
	if err != nil {
		t.Fatal(err)
	}
	secret.Data["username"] = []byte(base64.StdEncoding.EncodeToString([]byte("username")))
	err = r.Update(context.TODO(), secret)
	if err != nil {
		t.Fatal(err)
	}
	waitForNoError(t, r, host)
}

// TestBreakThenFixSecret ensures that when the secret for a known host
// is updated to be broken, and then correct, the status of the host
// moves out of the error state.
func TestBreakThenFixSecret(t *testing.T) {
	// Create the host without any errors and wait for it to be
	// registered and get a provisioning ID.
	secret := newBMCCredsSecret("bmc-creds-toggle-user", "User", "Pass")
	host := newHost("break-then-fix-secret",
		&metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address:         "ipmi://192.168.122.1:6233",
				CredentialsName: "bmc-creds-toggle-user",
			},
		})
	r := newTestReconciler(host, secret)
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			id := host.Status.Provisioning.ID
			t.Logf("Waiting for provisioning ID to be set: %q", id)
			return id != ""
		},
	)

	// Modify the secret to be bad by removing the username. Wait for
	// the host to show the error.
	secret = &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: namespace,
		Name:      "bmc-creds-toggle-user",
	}
	err := r.Get(context.TODO(), secretName, secret)
	if err != nil {
		t.Fatal(err)
	}
	oldUsername := secret.Data["username"]
	secret.Data["username"] = []byte{}
	err = r.Update(context.TODO(), secret)
	if err != nil {
		t.Fatal(err)
	}
	waitForError(t, r, host)

	// Modify the secret to be correct again. Wait for the error to be
	// cleared from the host.
	secret = &corev1.Secret{}
	err = r.Get(context.TODO(), secretName, secret)
	if err != nil {
		t.Fatal(err)
	}
	secret.Data["username"] = oldUsername
	err = r.Update(context.TODO(), secret)
	if err != nil {
		t.Fatal(err)
	}
	waitForNoError(t, r, host)
}

// TestSetHardwareProfile ensures that the host has a label with
// the hardware profile name.
func TestSetHardwareProfile(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("profile: %v", host.Status.HardwareProfile)
			return host.Status.HardwareProfile != ""
		},
	)
}

// TestCreateHardwareDetails ensures that the HardwareDetails portion
// of the status block is filled in for new hosts.
func TestCreateHardwareDetails(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("new host details: %v", host.Status.HardwareDetails)
			return host.Status.HardwareDetails != nil
		},
	)
}

// TestNeedsProvisioning verifies the logic for deciding when a host
// needs to be provisioned.
func TestNeedsProvisioning(t *testing.T) {
	host := newDefaultHost(t)

	if host.NeedsProvisioning() {
		t.Fatal("host without spec image should not need provisioning")
	}

	host.Spec.Image = &metal3api.Image{
		URL:      "https://example.com/image-name",
		Checksum: "12345",
	}

	if host.NeedsProvisioning() {
		t.Fatal("host with spec image but not online should not need provisioning")
	}

	host.Spec.Online = true

	if !host.NeedsProvisioning() {
		t.Fatal("host with spec image and online without provisioning image should need provisioning")
	}

	host.Status.Provisioning.Image = *host.Spec.Image

	if host.NeedsProvisioning() {
		t.Fatal("host with spec image matching status image should not need provisioning")
	}
}

// TestNeedsProvisioning verifies the logic for deciding when a host
// needs to be provisioned when custom deploy is used.
func TestNeedsProvisioningCustomDeploy(t *testing.T) {
	cases := []struct {
		name string

		customDeploy        string
		currentCustomDeploy string
		online              bool
		image               *metal3api.Image

		needsProvisioning bool
	}{
		{
			name:              "empty host",
			needsProvisioning: false,
		},
		{
			name:              "with custom deploy but not online",
			customDeploy:      "install_everything",
			needsProvisioning: false,
		},
		{
			name:              "with custom deploy and online",
			customDeploy:      "install_everything",
			online:            true,
			needsProvisioning: true,
		},
		{
			name:                "with matching custom deploy and online",
			customDeploy:        "install_everything",
			currentCustomDeploy: "install_everything",
			online:              true,
			needsProvisioning:   false,
		},
		{
			name:                "with custom deploy and new image",
			customDeploy:        "install_everything",
			currentCustomDeploy: "install_everything",
			image: &metal3api.Image{
				URL:      "https://example.com/image-name",
				Checksum: "12345",
			},
			online:            true,
			needsProvisioning: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := newDefaultHost(t)

			host.Spec.Online = tc.online
			if tc.customDeploy != "" {
				host.Spec.CustomDeploy = &metal3api.CustomDeploy{
					Method: tc.customDeploy,
				}
			}
			if tc.currentCustomDeploy != "" {
				host.Status.Provisioning.CustomDeploy = &metal3api.CustomDeploy{
					Method: tc.currentCustomDeploy,
				}
			}
			if tc.image != nil {
				host.Spec.Image = tc.image
			}

			assert.Equal(t, tc.needsProvisioning, host.NeedsProvisioning())
		})
	}
}

// TestProvision ensures that the Provisioning.Image portion of the
// status block is filled in for provisioned hosts.
func TestProvision(t *testing.T) {
	host := newDefaultHost(t)
	host.Spec.Image = &metal3api.Image{
		URL:      "https://example.com/image-name",
		Checksum: "12345",
	}
	host.Spec.Online = true
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("image details: %v", host.Spec.Image)
			t.Logf("provisioning image details: %v", host.Status.Provisioning.Image)
			t.Logf("provisioning state: %v", host.Status.Provisioning.State)
			return host.Status.Provisioning.Image.URL != ""
		},
	)
}

// TestProvisionCustomDeploy ensures that the Provisioning.CustomDeploy portion
// of the status block is filled in for provisioned hosts.
func TestProvisionCustomDeploy(t *testing.T) {
	host := newDefaultHost(t)
	host.Spec.CustomDeploy = &metal3api.CustomDeploy{
		Method: "install_everything",
	}
	host.Spec.Online = true
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("custom deploy: %v", host.Spec.CustomDeploy)
			t.Logf("provisioning custom deploy: %v", host.Status.Provisioning.CustomDeploy)
			t.Logf("provisioning state: %v", host.Status.Provisioning.State)
			return host.Status.Provisioning.CustomDeploy != nil && host.Status.Provisioning.CustomDeploy.Method == "install_everything" && host.Status.Provisioning.State == metal3api.StateProvisioned
		},
	)
}

// TestProvisionCustomDeployWithURL ensures that the Provisioning.CustomDeploy
// portion of the status block is filled in for provisioned hosts.
func TestProvisionCustomDeployWithURL(t *testing.T) {
	host := newDefaultHost(t)
	host.Spec.CustomDeploy = &metal3api.CustomDeploy{
		Method: "install_everything",
	}
	host.Spec.Image = &metal3api.Image{
		URL:      "https://example.com/image-name",
		Checksum: "12345",
	}
	host.Spec.Online = true
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("image details: %v", host.Spec.Image)
			t.Logf("custom deploy: %v", host.Spec.CustomDeploy)
			t.Logf("provisioning image details: %v", host.Status.Provisioning.Image)
			t.Logf("provisioning custom deploy: %v", host.Status.Provisioning.CustomDeploy)
			t.Logf("provisioning state: %v", host.Status.Provisioning.State)
			return host.Status.Provisioning.CustomDeploy != nil && host.Status.Provisioning.CustomDeploy.Method != "" && host.Status.Provisioning.Image.URL != ""
		},
	)
}

// TestExternallyProvisionedTransitions ensures that host enters the
// expected states when it looks like it has been provisioned by
// another tool.
func TestExternallyProvisionedTransitions(t *testing.T) {
	t.Run("registered to externally provisioned", func(t *testing.T) {
		host := newDefaultHost(t)
		host.Spec.Online = true
		host.Spec.ConsumerRef = &corev1.ObjectReference{} // it doesn't have to point to a real object
		host.Spec.ExternallyProvisioned = true
		r := newTestReconciler(host)

		waitForProvisioningState(t, r, host, metal3api.StateExternallyProvisioned)
	})

	t.Run("externally provisioned to inspecting", func(t *testing.T) {
		host := newDefaultHost(t)
		host.Spec.Online = true
		host.Spec.ExternallyProvisioned = true
		r := newTestReconciler(host)

		waitForProvisioningState(t, r, host, metal3api.StateExternallyProvisioned)

		host.Spec.ExternallyProvisioned = false
		err := r.Update(context.TODO(), host)
		if err != nil {
			t.Fatal(err)
		}
		t.Log("set externally provisioned to false")

		waitForProvisioningState(t, r, host, metal3api.StateInspecting)
	})

	t.Run("preparing to externally provisioned", func(t *testing.T) {
		host := newDefaultHost(t)
		host.Spec.Online = true
		r := newTestReconciler(host)

		waitForProvisioningState(t, r, host, metal3api.StatePreparing)

		host.Spec.ExternallyProvisioned = true
		err := r.Update(context.TODO(), host)
		if err != nil {
			t.Fatal(err)
		}
		t.Log("set externally provisioned to true")

		waitForProvisioningState(t, r, host, metal3api.StateExternallyProvisioned)
	})
}

// TestPowerOn verifies that the controller turns the host on when it
// should.
func TestPowerOn(t *testing.T) {
	host := newDefaultHost(t)
	host.Spec.Online = true
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("power status: %v", host.Status.PoweredOn)
			return host.Status.PoweredOn
		},
	)
}

// TestPowerOff verifies that the controller turns the host on when it
// should.
func TestPowerOff(t *testing.T) {
	host := newDefaultHost(t)
	host.Spec.Online = false
	r := newTestReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("power status: %v", host.Status.PoweredOn)
			return !host.Status.PoweredOn
		},
	)
}

// TestDeleteHost verifies several delete cases.
func TestDeleteHost(t *testing.T) {
	now := metav1.Now()

	type HostFactory func() *metal3api.BareMetalHost

	testCases := []HostFactory{
		func() *metal3api.BareMetalHost {
			host := newDefaultNamedHost(t, "with-finalizer")
			host.Finalizers = append(host.Finalizers,
				metal3api.BareMetalHostFinalizer)
			return host
		},
		func() *metal3api.BareMetalHost {
			host := newDefaultNamedHost(t, "without-bmc")
			host.Spec.BMC = metal3api.BMCDetails{}
			host.Finalizers = append(host.Finalizers,
				metal3api.BareMetalHostFinalizer)
			return host
		},
		func() *metal3api.BareMetalHost {
			t.Logf("host with bad credentials, no user")
			host := newHost("fix-secret",
				&metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://192.168.122.1:6233",
						CredentialsName: "bmc-creds-no-user",
					},
				})
			host.Finalizers = append(host.Finalizers,
				metal3api.BareMetalHostFinalizer)
			return host
		},
		func() *metal3api.BareMetalHost {
			host := newDefaultNamedHost(t, "host-with-hw-details")
			host.Status.HardwareDetails = &metal3api.HardwareDetails{}
			host.Finalizers = append(host.Finalizers,
				metal3api.BareMetalHostFinalizer)
			return host
		},
		func() *metal3api.BareMetalHost {
			host := newDefaultNamedHost(t, "provisioned-host")
			host.Status.HardwareDetails = &metal3api.HardwareDetails{}
			host.Status.Provisioning.Image = metal3api.Image{
				URL:      "image-url",
				Checksum: "image-checksum",
			}
			host.Spec.Image = &host.Status.Provisioning.Image
			host.Finalizers = append(host.Finalizers,
				metal3api.BareMetalHostFinalizer)
			return host
		},
	}

	for _, factory := range testCases {
		host := factory()
		t.Run(host.Name, func(t *testing.T) {
			host.DeletionTimestamp = &now
			host.Status.Provisioning.ID = "made-up-id"
			badSecret := newBMCCredsSecret("bmc-creds-no-user", "", "Pass")
			fix := fixture.Fixture{}
			r := newTestReconcilerWithFixture(&fix, host, badSecret)

			tryReconcile(t, r, host,
				func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
					return fix.Deleted && host == nil && !utils.StringInList(badSecret.Finalizers, secretutils.SecretsFinalizer)
				},
			)
		})
	}
}

// TestUpdateRootDeviceHints verifies that we apply the correct
// precedence rules to the root device hints settings for a host.
func TestUpdateRootDeviceHints(t *testing.T) {
	rotational := true
	rotationalTwo := true
	rotationalFalse := false

	testCases := []struct {
		Scenario string
		Host     metal3api.BareMetalHost
		Dirty    bool
		Expected *metal3api.RootDeviceHints
	}{
		{
			Scenario: "override profile with explicit hints",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
					UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
				},
				Spec: metal3api.BareMetalHostSpec{
					HardwareProfile: "libvirt",
					RootDeviceHints: &metal3api.RootDeviceHints{
						DeviceName:         "userd_devicename",
						HCTL:               "1:2:3:4",
						Model:              "userd_model",
						Vendor:             "userd_vendor",
						SerialNumber:       "userd_serial",
						MinSizeGigabytes:   40,
						WWN:                "userd_wwn",
						WWNWithExtension:   "userd_with_extension",
						WWNVendorExtension: "userd_vendor_extension",
						Rotational:         &rotational,
					},
					RAID: &metal3api.RAIDConfig{
						SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
					},
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareProfile: "libvirt",
				},
			},
			Dirty: true,
			Expected: &metal3api.RootDeviceHints{
				DeviceName:         "userd_devicename",
				HCTL:               "1:2:3:4",
				Model:              "userd_model",
				Vendor:             "userd_vendor",
				SerialNumber:       "userd_serial",
				MinSizeGigabytes:   40,
				WWN:                "userd_wwn",
				WWNWithExtension:   "userd_with_extension",
				WWNVendorExtension: "userd_vendor_extension",
				Rotational:         &rotational,
			},
		},

		{
			Scenario: "use profile hints",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
					UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
				},
				Spec: metal3api.BareMetalHostSpec{
					HardwareProfile: "libvirt",
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareProfile: "libvirt",
				},
			},
			Dirty: true,
			Expected: &metal3api.RootDeviceHints{
				DeviceName: "/dev/vda",
			},
		},

		{
			Scenario: "default profile hints",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
					UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
				},
				Spec: metal3api.BareMetalHostSpec{
					HardwareProfile: "unknown",
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareProfile: "unknown",
				},
			},
			Dirty: true,
			Expected: &metal3api.RootDeviceHints{
				DeviceName: "/dev/sda",
			},
		},

		{
			Scenario: "rotational values same",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
					UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
				},
				Spec: metal3api.BareMetalHostSpec{
					HardwareProfile: "libvirt",
					RootDeviceHints: &metal3api.RootDeviceHints{
						MinSizeGigabytes: 40,
						Rotational:       &rotational,
					},
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareProfile: "libvirt",
					Provisioning: metal3api.ProvisionStatus{
						RootDeviceHints: &metal3api.RootDeviceHints{
							MinSizeGigabytes: 40,
							Rotational:       &rotationalTwo,
						},
						RAID: &metal3api.RAIDConfig{
							SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
						}},
				},
			},
			Dirty: false,
			Expected: &metal3api.RootDeviceHints{
				MinSizeGigabytes: 40,
				Rotational:       &rotational,
			},
		},

		{
			Scenario: "rotational values different",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
					UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
				},
				Spec: metal3api.BareMetalHostSpec{
					HardwareProfile: "libvirt",
					RootDeviceHints: &metal3api.RootDeviceHints{
						MinSizeGigabytes: 40,
						Rotational:       &rotational,
					},
				},
				Status: metal3api.BareMetalHostStatus{
					HardwareProfile: "libvirt",
					Provisioning: metal3api.ProvisionStatus{
						RootDeviceHints: &metal3api.RootDeviceHints{
							MinSizeGigabytes: 40,
							Rotational:       &rotationalFalse,
						},
						RAID: &metal3api.RAIDConfig{
							SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
						}},
				},
			},
			Dirty: true,
			Expected: &metal3api.RootDeviceHints{
				MinSizeGigabytes: 40,
				Rotational:       &rotational,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			host := tc.Host
			info := makeReconcileInfo(&host)
			dirty, newStatus, err := getHostProvisioningSettings(&host, info)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.Dirty, dirty, "dirty flag did not match")
			assert.Equal(t, tc.Expected, newStatus.Provisioning.RootDeviceHints)

			dirty, err = saveHostProvisioningSettings(&host, info)
			tc.Host = host

			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.Dirty, dirty, "dirty flag did not match")
			assert.Equal(t, tc.Expected, tc.Host.Status.Provisioning.RootDeviceHints)
		})
	}
}

func TestProvisionerIsReady(t *testing.T) {
	host := newDefaultHost(t)

	fix := fixture.Fixture{BecomeReadyCounter: 5}
	r := newTestReconcilerWithFixture(&fix, host)

	assert.Equal(t, 0.0, promutil.ToFloat64(provisionerNotReady))
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return host.Status.Provisioning.State != metal3api.StateNone
		},
	)
	assert.Equal(t, 4.0, promutil.ToFloat64(provisionerNotReady))
}

func TestUpdateEventHandler(t *testing.T) {
	cases := []struct {
		name            string
		event           event.UpdateEvent
		expectedProcess bool
	}{
		{
			name: "process-non-bmh-events",
			event: event.UpdateEvent{
				ObjectOld: &corev1.Secret{},
				ObjectNew: &corev1.Secret{},
			},
			expectedProcess: true,
		},
		{
			name: "process-generation-change",
			event: event.UpdateEvent{
				ObjectOld: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{Generation: 0}},
				ObjectNew: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			},

			expectedProcess: true,
		},
		{
			name: "skip-if-same-generation-finalizers-and-annotations",
			event: event.UpdateEvent{
				ObjectOld: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{
					Generation: 0,
					Finalizers: []string{metal3api.BareMetalHostFinalizer},
					Annotations: map[string]string{
						metal3api.PausedAnnotation: "true",
					},
				}},
				ObjectNew: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{
					Generation: 0,
					Finalizers: []string{metal3api.BareMetalHostFinalizer},
					Annotations: map[string]string{
						metal3api.PausedAnnotation: "true",
					},
				}},
			},

			expectedProcess: false,
		},
		{
			name: "process-same-generation-annotations-change",
			event: event.UpdateEvent{
				ObjectOld: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{
					Generation:  0,
					Finalizers:  []string{metal3api.BareMetalHostFinalizer},
					Annotations: map[string]string{},
				}},
				ObjectNew: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{
					Generation: 0,
					Finalizers: []string{metal3api.BareMetalHostFinalizer},
					Annotations: map[string]string{
						metal3api.PausedAnnotation: "true",
					},
				}},
			},

			expectedProcess: true,
		},
		{
			name: "process-same-generation-finalizers-change",
			event: event.UpdateEvent{
				ObjectOld: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{
					Generation: 0,
					Finalizers: []string{},
					Annotations: map[string]string{
						metal3api.PausedAnnotation: "true",
					},
				}},
				ObjectNew: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{
					Generation: 0,
					Finalizers: []string{metal3api.BareMetalHostFinalizer},
					Annotations: map[string]string{
						metal3api.PausedAnnotation: "true",
					},
				}},
			},

			expectedProcess: true,
		},
		{
			name: "process-same-generation-finalizers-and-annotation-change",
			event: event.UpdateEvent{
				ObjectOld: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{
					Generation:  0,
					Finalizers:  []string{},
					Annotations: map[string]string{},
				}},
				ObjectNew: &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{
					Generation: 0,
					Finalizers: []string{metal3api.BareMetalHostFinalizer},
					Annotations: map[string]string{
						metal3api.PausedAnnotation: "true",
					},
				}},
			},

			expectedProcess: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestReconciler()
			assert.Equal(t, tc.expectedProcess, r.updateEventHandler(tc.event))
		})
	}
}

func TestErrorCountIncrementsAlways(t *testing.T) {
	errorTypes := []metal3api.ErrorType{metal3api.RegistrationError, metal3api.InspectionError, metal3api.ProvisioningError, metal3api.PowerManagementError}

	b := &metal3api.BareMetalHost{}
	assert.Equal(t, b.Status.ErrorCount, 0)

	for _, c := range errorTypes {
		before := b.Status.ErrorCount
		setErrorMessage(b, c, "An error message")
		assert.Equal(t, before+1, b.Status.ErrorCount)
	}
}

func TestGetImageAvailable(t *testing.T) {
	host := metal3api.BareMetalHost{
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL: "http://example.test/image",
			},
		},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				State: metal3api.StateAvailable,
			},
		},
	}

	img := getCurrentImage(&host)

	assert.Nil(t, img)
}

func TestGetImageProvisioning(t *testing.T) {
	host := metal3api.BareMetalHost{
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL: "http://example.test/image",
			},
		},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				State: metal3api.StateProvisioning,
			},
		},
	}

	img := getCurrentImage(&host)

	assert.NotNil(t, img)
	assert.NotSame(t, host.Spec.Image, img)
	assert.Exactly(t, *host.Spec.Image, *img)
}

func TestGetImageProvisioned(t *testing.T) {
	host := metal3api.BareMetalHost{
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL: "http://example.test/image2",
			},
		},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				State: metal3api.StateProvisioned,
				Image: metal3api.Image{
					URL: "http://example.test/image",
				},
			},
		},
	}

	img := getCurrentImage(&host)

	assert.NotNil(t, img)
	assert.NotSame(t, &host.Status.Provisioning.Image, img)
	assert.Exactly(t, host.Status.Provisioning.Image, *img)
}

func TestGetImageDeprovisioning(t *testing.T) {
	host := metal3api.BareMetalHost{
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL: "http://example.test/image2",
			},
		},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				State: metal3api.StateDeprovisioning,
				Image: metal3api.Image{
					URL: "http://example.test/image",
				},
			},
		},
	}

	img := getCurrentImage(&host)

	assert.NotNil(t, img)
	assert.NotSame(t, &host.Status.Provisioning.Image, img)
	assert.Exactly(t, host.Status.Provisioning.Image, *img)
}

func TestGetImageExternallyPprovisioned(t *testing.T) {
	host := metal3api.BareMetalHost{
		Spec: metal3api.BareMetalHostSpec{
			Image: &metal3api.Image{
				URL: "http://example.test/image",
			},
		},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				State: metal3api.StateExternallyProvisioned,
			},
		},
	}

	img := getCurrentImage(&host)

	assert.NotNil(t, img)
	assert.NotSame(t, host.Spec.Image, img)
	assert.Exactly(t, *host.Spec.Image, *img)
}

func TestUpdateRAID(t *testing.T) {
	host := metal3api.BareMetalHost{
		Spec: metal3api.BareMetalHostSpec{
			HardwareProfile: "libvirt",
			RootDeviceHints: &metal3api.RootDeviceHints{
				DeviceName:         "userd_devicename",
				HCTL:               "1:2:3:4",
				Model:              "userd_model",
				Vendor:             "userd_vendor",
				SerialNumber:       "userd_serial",
				MinSizeGigabytes:   40,
				WWN:                "userd_wwn",
				WWNWithExtension:   "userd_with_extension",
				WWNVendorExtension: "userd_vendor_extension",
			},
		},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				RAID: &metal3api.RAIDConfig{
					HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{},
					SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
				},
				RootDeviceHints: &metal3api.RootDeviceHints{
					DeviceName:         "userd_devicename",
					HCTL:               "1:2:3:4",
					Model:              "userd_model",
					Vendor:             "userd_vendor",
					SerialNumber:       "userd_serial",
					MinSizeGigabytes:   40,
					WWN:                "userd_wwn",
					WWNWithExtension:   "userd_with_extension",
					WWNVendorExtension: "userd_vendor_extension",
				},
			},
		},
	}

	cases := []struct {
		name     string
		raid     *metal3api.RAIDConfig
		dirty    bool
		expected *metal3api.RAIDConfig
	}{
		{
			name:  "keep current hardware RAID, clear current software RAID",
			raid:  nil,
			dirty: true,
			expected: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
		},
		{
			name:  "keep current hardware RAID, clear current software RAID",
			raid:  &metal3api.RAIDConfig{},
			dirty: false,
			expected: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
		},
		{
			name: "Configure hardwareRAIDVolumes",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			dirty: true,
			expected: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
		},
		{
			name: "Configure hardwareRAIDVolumes",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Level: "1",
					},
				},
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			dirty: true,
			expected: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Level: "1",
					},
				},
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
		},
		{
			name: "Clear hardwareRAIDVolumes",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{},
			},
			dirty: true,
			expected: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{},
			},
		},
		{
			name: "Clear hardwareRAIDVolumes",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{},
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
			dirty: true,
			expected: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{},
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
		},
		{
			name: "Configure SoftwareRAIDVolumes",
			raid: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			dirty: true,
			expected: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
		},
		{
			name: "Clear softwareRAIDVolumes",
			raid: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
			dirty: true,
			expected: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host.Spec.RAID = c.raid
			info := makeReconcileInfo(&host)
			dirty, _ := saveHostProvisioningSettings(&host, info)
			assert.Equal(t, c.dirty, dirty)
			assert.Equal(t, c.expected, host.Status.Provisioning.RAID)
			dirty, _, _ = getHostProvisioningSettings(&host, info)
			assert.Equal(t, false, dirty)
		})
	}
}

func doDeleteHost(host *metal3api.BareMetalHost, reconciler *BareMetalHostReconciler) error {
	return reconciler.Client.Delete(context.Background(), host)
}

func TestInvalidBMHCanBeDeleted(t *testing.T) {
	host := newDefaultHost(t)
	host.Spec.BMC.Address = fmt.Sprintf("%s%s%s", "<", host.Spec.BMC.Address, ">")

	var fix fixture.Fixture
	r := newTestReconcilerWithFixture(&fix, host)

	fix.SetValidateError("malformed url")
	waitForError(t, r, host)
	assert.Equal(t, metal3api.StateRegistering, host.Status.Provisioning.State)
	assert.Equal(t, metal3api.OperationalStatusError, host.Status.OperationalStatus)
	assert.Equal(t, metal3api.RegistrationError, host.Status.ErrorType)
	assert.Equal(t, "malformed url", host.Status.ErrorMessage)

	err := doDeleteHost(host, r)
	assert.NoError(t, err)

	tryReconcile(t, r, host, func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
		return host == nil
	})
}

func TestCredentialsFromSecret(t *testing.T) {
	cases := []struct {
		name     string
		input    corev1.Secret
		expected bmc.Credentials
	}{
		{
			name:     "empty",
			input:    corev1.Secret{},
			expected: bmc.Credentials{},
		},
		{
			name: "clean",
			input: corev1.Secret{
				Data: map[string][]byte{
					"username": []byte("username"),
					"password": []byte("password"),
				},
			},
			expected: bmc.Credentials{
				Username: "username",
				Password: "password",
			},
		},
		{
			name: "newline",
			input: corev1.Secret{
				Data: map[string][]byte{
					"username": []byte("username\n"),
					"password": []byte("password\n"),
				},
			},
			expected: bmc.Credentials{
				Username: "username",
				Password: "password",
			},
		},
		{
			name: "non-newline",
			input: corev1.Secret{
				Data: map[string][]byte{
					"username": []byte(" username\t"),
					"password": []byte(" password\t"),
				},
			},
			expected: bmc.Credentials{
				Username: "username",
				Password: "password",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := c.input
			actual := credentialsFromSecret(&input)
			assert.Equal(t, c.expected, *actual)
		})
	}
}

func TestGetHardwareProfileName(t *testing.T) {
	testCases := []struct {
		Scenario      string
		Address       string
		StatusProfile string
		SpecProfile   string
		Expected      string
	}{
		{
			Scenario: "default",
			Expected: "unknown",
		},
		{
			Scenario: "infer libvirt",
			Address:  "libvirt://example.test",
			Expected: "libvirt",
		},
		{
			Scenario:    "not yet set",
			SpecProfile: "foo",
			Expected:    "foo",
		},
		{
			Scenario:      "already set",
			SpecProfile:   "foo",
			StatusProfile: "foo",
			Expected:      "foo",
		},
		{
			Scenario:      "changed",
			SpecProfile:   "bar",
			StatusProfile: "foo",
			Expected:      "foo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			host := newHost("test", &metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address: tc.Address,
				},
				HardwareProfile: tc.SpecProfile,
			})
			host.Status.HardwareProfile = tc.StatusProfile

			assert.Equal(t, tc.Expected, getHardwareProfileName(host))
		})
	}
}

func TestGetHostArchitecture(t *testing.T) {
	host := newDefaultHost(t)
	assert.Equal(t, "x86_64", getHostArchitecture(host))

	host.Spec.Architecture = "aarch64"
	assert.Equal(t, "aarch64", getHostArchitecture(host))

	host.Spec.Architecture = ""
	host.Status.HardwareDetails = &metal3api.HardwareDetails{
		CPU: metal3api.CPU{
			Arch: "aarch64",
		},
	}
	assert.Equal(t, "aarch64", getHostArchitecture(host))
}

func TestGetPreprovImageNoFormats(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host)
	i := makeReconcileInfo(host)

	imgData, err := r.getPreprovImage(i, nil)

	assert.NoError(t, err)
	assert.Nil(t, imgData)

	imgData, err = r.getPreprovImage(i, []metal3api.ImageFormat{})
	assert.True(t, errors.As(err, &imageBuildError{}))
	assert.EqualError(t, err, "no acceptable formats for preprovisioning image")
	assert.Nil(t, imgData)

	assert.Error(t, r.Client.Get(context.TODO(), client.ObjectKey{
		Name:      host.Name,
		Namespace: host.Namespace,
	},
		&metal3api.PreprovisioningImage{}))
}

func TestGetPreprovImageCreateUpdate(t *testing.T) {
	secretName := "net_secret"
	host := newDefaultHost(t)
	host.Spec.PreprovisioningNetworkDataName = secretName
	host.Labels = map[string]string{
		"answer.metal3.io": "42",
	}
	r := newTestReconciler(host, newSecret(secretName, nil))
	i := makeReconcileInfo(host)

	imgData, err := r.getPreprovImage(i, []metal3api.ImageFormat{"iso"})
	assert.NoError(t, err)
	assert.Nil(t, imgData)

	img := metal3api.PreprovisioningImage{}
	assert.NoError(t, r.Client.Get(context.TODO(), client.ObjectKey{
		Name:      host.Name,
		Namespace: host.Namespace,
	},
		&img))
	assert.Equal(t, "x86_64", img.Spec.Architecture)
	assert.Equal(t, secretName, img.Spec.NetworkDataName)
	assert.Equal(t, "42", img.Labels["answer.metal3.io"])

	newSecretName := "new_net_secret"
	host.Spec.PreprovisioningNetworkDataName = newSecretName
	host.Labels["cat.metal3.io"] = "meow"

	imgData, err = r.getPreprovImage(i, []metal3api.ImageFormat{"iso"})
	assert.NoError(t, err)
	assert.Nil(t, imgData)

	assert.NoError(t, r.Client.Get(context.TODO(), client.ObjectKey{
		Name:      host.Name,
		Namespace: host.Namespace,
	},
		&img))
	assert.Equal(t, newSecretName, img.Spec.NetworkDataName)
	assert.Equal(t, "42", img.Labels["answer.metal3.io"])
	assert.Equal(t, "meow", img.Labels["cat.metal3.io"])
}

func TestGetPreprovImage(t *testing.T) {
	host := newDefaultHost(t)
	imageURL := "http://example.test/image.iso"
	acceptFormats := []metal3api.ImageFormat{metal3api.ImageFormatISO, metal3api.ImageFormatInitRD}
	image := &metal3api.PreprovisioningImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      host.Name,
			Namespace: namespace,
		},
		Spec: metal3api.PreprovisioningImageSpec{
			Architecture:  "x86_64",
			AcceptFormats: acceptFormats,
		},
		Status: metal3api.PreprovisioningImageStatus{
			Architecture: "x86_64",
			Format:       metal3api.ImageFormatISO,
			ImageUrl:     imageURL,
			Conditions: []metav1.Condition{
				{
					Type:   string(metal3api.ConditionImageReady),
					Status: metav1.ConditionTrue,
				},
				{
					Type:   string(metal3api.ConditionImageError),
					Status: metav1.ConditionFalse,
				},
			},
		},
	}
	r := newTestReconciler(host, image)
	i := makeReconcileInfo(host)

	imgData, err := r.getPreprovImage(i, acceptFormats)
	assert.NoError(t, err)
	assert.NotNil(t, imgData)
	assert.Equal(t, imageURL, imgData.ImageURL)
	assert.Equal(t, metal3api.ImageFormatISO, imgData.Format)
}

func TestGetPreprovImageNotCurrent(t *testing.T) {
	host := newDefaultHost(t)
	imageURL := "http://example.test/image.iso"
	image := &metal3api.PreprovisioningImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      host.Name,
			Namespace: namespace,
		},
		Spec: metal3api.PreprovisioningImageSpec{
			Architecture: "x86_64",
		},
		Status: metal3api.PreprovisioningImageStatus{
			Architecture: "x86_64",
			Format:       metal3api.ImageFormatISO,
			ImageUrl:     imageURL,
			Conditions: []metav1.Condition{
				{
					Type:   string(metal3api.ConditionImageReady),
					Status: metav1.ConditionFalse,
				},
				{
					Type:   string(metal3api.ConditionImageError),
					Status: metav1.ConditionFalse,
				},
			},
		},
	}
	r := newTestReconciler(host, image)
	i := makeReconcileInfo(host)

	imgData, err := r.getPreprovImage(i, []metal3api.ImageFormat{metal3api.ImageFormatISO})
	assert.NoError(t, err)
	assert.Nil(t, imgData)
}

func TestPreprovImageAvailable(t *testing.T) {
	host := newDefaultHost(t)
	r := newTestReconciler(host, newSecret("network_secret_1", nil))

	testCases := []struct {
		Scenario   string
		Spec       metal3api.PreprovisioningImageSpec
		Status     metal3api.PreprovisioningImageStatus
		Available  bool
		BuildError bool
	}{
		{
			Scenario: "ready no netdata",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:  "x86_64",
				AcceptFormats: []metal3api.ImageFormat{"iso", "initrd"},
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "iso",
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: true,
		},
		{
			Scenario: "ready",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:    "x86_64",
				AcceptFormats:   []metal3api.ImageFormat{"iso", "initrd"},
				NetworkDataName: "network_secret_1",
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "iso",
				NetworkData: metal3api.SecretStatus{
					Name:    "network_secret_1",
					Version: "1000",
				},
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: true,
		},
		{
			Scenario: "ready initrd",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:    "x86_64",
				AcceptFormats:   []metal3api.ImageFormat{"initrd"},
				NetworkDataName: "network_secret_1",
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "initrd",
				NetworkData: metal3api.SecretStatus{
					Name:    "network_secret_1",
					Version: "1000",
				},
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: true,
		},
		{
			Scenario: "ready initrd fallback",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:    "x86_64",
				AcceptFormats:   []metal3api.ImageFormat{"iso", "initrd"},
				NetworkDataName: "network_secret_1",
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "initrd",
				NetworkData: metal3api.SecretStatus{
					Name:    "network_secret_1",
					Version: "1000",
				},
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: true,
		},
		{
			Scenario: "ready secret outdated",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:    "x86_64",
				AcceptFormats:   []metal3api.ImageFormat{"iso", "initrd"},
				NetworkDataName: "network_secret_1",
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "iso",
				NetworkData: metal3api.SecretStatus{
					Name:    "network_secret_1",
					Version: "42",
				},
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: false,
		},
		{
			Scenario: "ready secret mismatch",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:    "x86_64",
				AcceptFormats:   []metal3api.ImageFormat{"iso", "initrd"},
				NetworkDataName: "network_secret_1",
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "iso",
				NetworkData: metal3api.SecretStatus{
					Name:    "network_secret_0",
					Version: "1",
				},
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: false,
		},
		{
			Scenario: "ready arch mismatch",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:  "aarch64",
				AcceptFormats: []metal3api.ImageFormat{"iso", "initrd"},
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "iso",
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: false,
		},
		{
			Scenario: "ready format mismatch",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:  "x86_64",
				AcceptFormats: []metal3api.ImageFormat{"initrd"},
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "iso",
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: false,
		},
		{
			Scenario: "not ready",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:  "x86_64",
				AcceptFormats: []metal3api.ImageFormat{"iso", "initrd"},
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "iso",
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionFalse,
					},
					{
						Type:   string(metal3api.ConditionImageError),
						Status: metav1.ConditionFalse,
					},
				},
			},
			Available: false,
		},
		{
			Scenario: "failed",
			Spec: metal3api.PreprovisioningImageSpec{
				Architecture:  "x86_64",
				AcceptFormats: []metal3api.ImageFormat{"iso", "initrd"},
			},
			Status: metal3api.PreprovisioningImageStatus{
				Architecture: "x86_64",
				Format:       "iso",
				Conditions: []metav1.Condition{
					{
						Type:   string(metal3api.ConditionImageReady),
						Status: metav1.ConditionFalse,
					},
					{
						Type:    string(metal3api.ConditionImageError),
						Status:  metav1.ConditionTrue,
						Message: "oops",
					},
				},
			},
			Available:  false,
			BuildError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			image := metal3api.PreprovisioningImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      host.Name,
					Namespace: namespace,
				},
				Spec:   tc.Spec,
				Status: tc.Status,
			}
			available, err := r.preprovImageAvailable(makeReconcileInfo(host), &image)
			if tc.BuildError {
				assert.EqualError(t, err, "oops")
				assert.True(t, errors.As(err, &imageBuildError{}))
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.Available, available)
		})
	}
}

// TestHostFirwmareSettings verifies that a change to the HFS
// can be detected as it will be used to set state to Preparing.
func TestHostFirmwareSettings(t *testing.T) {
	testCases := []struct {
		Scenario   string
		Conditions []metav1.Condition
		Dirty      bool
	}{
		{
			Scenario: "spec and status the same",
			Conditions: []metav1.Condition{
				{Type: "Valid", Status: "True", Reason: "Success"},
			},
			Dirty: false,
		},
		{
			Scenario: "spec changed",
			Conditions: []metav1.Condition{
				{Type: "ChangeDetected", Status: "True", Reason: "Success"},
				{Type: "Valid", Status: "True", Reason: "Success"},
			},
			Dirty: true,
		},
		{
			Scenario: "spec invalid",
			Conditions: []metav1.Condition{
				{Type: "ChangeDetected", Status: "True", Reason: "Success"},
				{Type: "Valid", Status: "False", Reason: "Success"},
			},
			Dirty: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			host := newDefaultHost(t)
			r := newTestReconciler(host)
			i := makeReconcileInfo(host)
			i.request = newRequest(host)

			hfs := newHostFirmwareSettings(host, tc.Conditions)
			r.Create(context.TODO(), hfs)

			dirty, _, err := r.getHostFirmwareSettings(i)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.Dirty, dirty, "dirty flag did not match")
		})
	}
}

func TestBMHTransitionToPreparing(t *testing.T) {
	var True = true
	var False = false
	host := newDefaultHost(t)
	host.Spec.Online = true
	host.Spec.ExternallyProvisioned = false
	host.Spec.ConsumerRef = &corev1.ObjectReference{}
	r := newTestReconciler(host)

	waitForProvisioningState(t, r, host, metal3api.StateAvailable)

	// use different values between spec and status to force cleaning
	host.Status.Provisioning.Firmware = &metal3api.FirmwareConfig{
		VirtualizationEnabled:             &True,
		SimultaneousMultithreadingEnabled: &False,
	}
	host.Spec.Firmware = &metal3api.FirmwareConfig{
		VirtualizationEnabled:             &False,
		SimultaneousMultithreadingEnabled: &True,
	}

	err := r.Update(context.TODO(), host)
	if err != nil {
		t.Fatal(err)
	}

	waitForProvisioningState(t, r, host, metal3api.StatePreparing)
}

func TestHFSTransitionToPreparing(t *testing.T) {
	host := newDefaultHost(t)
	host.Spec.Online = true
	host.Spec.ConsumerRef = &corev1.ObjectReference{}
	host.Spec.ExternallyProvisioned = false
	r := newTestReconciler(host)

	waitForProvisioningState(t, r, host, metal3api.StateAvailable)

	// Update HFS so host will go through cleaning
	hfs := &metal3api.HostFirmwareSettings{}
	key := client.ObjectKey{
		Namespace: host.ObjectMeta.Namespace, Name: host.ObjectMeta.Name}
	if err := r.Get(context.TODO(), key, hfs); err != nil {
		t.Fatal(err)
	}

	hfs.Status = metal3api.HostFirmwareSettingsStatus{
		Conditions: []metav1.Condition{
			{Type: "ChangeDetected", Status: "True", Reason: "Success"},
			{Type: "Valid", Status: "True", Reason: "Success"},
		},
		Settings: metal3api.SettingsMap{
			"ProcVirtualization": "Enabled",
			"SecureBoot":         "Enabled",
		},
	}

	r.Update(context.TODO(), hfs)

	waitForProvisioningState(t, r, host, metal3api.StatePreparing)
}

// TestHFSEmptyStatusSettings ensures that BMH does not move to the next state
// when a user provides the BIOS settings on a hardware server that does not
// have the required license to configure BIOS.
func TestHFSEmptyStatusSettings(t *testing.T) {
	host := newDefaultHost(t)
	host.Spec.Online = true
	host.Spec.ConsumerRef = &corev1.ObjectReference{}
	host.Spec.ExternallyProvisioned = false
	r := newTestReconciler(host)

	waitForProvisioningState(t, r, host, metal3api.StatePreparing)

	// Update HFS so host will go through cleaning
	hfs := &metal3api.HostFirmwareSettings{}
	key := client.ObjectKey{
		Namespace: host.ObjectMeta.Namespace, Name: host.ObjectMeta.Name}
	if err := r.Get(context.TODO(), key, hfs); err != nil {
		t.Fatal(err)
	}

	hfs.Status = metal3api.HostFirmwareSettingsStatus{
		Conditions: []metav1.Condition{
			{Type: "ChangeDetected", Status: "True", Reason: "Success"},
			{Type: "Valid", Status: "True", Reason: "Success"},
		},
	}

	r.Update(context.TODO(), hfs)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return host.Status.Provisioning.State == metal3api.StatePreparing
		},
	)

	// Clear the change, it will no longer be blocked
	hfs.Status = metal3api.HostFirmwareSettingsStatus{
		Conditions: []metav1.Condition{
			{Type: "ChangeDetected", Status: "False", Reason: "Success"},
			{Type: "Valid", Status: "True", Reason: "Success"},
		},
	}

	r.Update(context.TODO(), hfs)
	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			return host.Status.Provisioning.State == metal3api.StateAvailable
		},
	)
}
