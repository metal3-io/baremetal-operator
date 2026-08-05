package controllers

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// dataImageFixture allows configuring GetDataImageStatus behavior for tests.
type dataImageFixture struct {
	fixture.Fixture
	imageAttached bool
	statusError   error
}

// dataImageProvisioner wraps the fixture provisioner to override GetDataImageStatus.
type dataImageProvisioner struct {
	provisioner.Provisioner
	imageAttached bool
	statusError   error
}

func (p *dataImageProvisioner) GetDataImageStatus(_ context.Context) (bool, error) {
	return p.imageAttached, p.statusError
}

func (f *dataImageFixture) NewProvisioner(ctx context.Context, hostData provisioner.HostData, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
	baseProv, err := f.Fixture.NewProvisioner(ctx, hostData, publisher)
	if err != nil {
		return nil, err
	}
	return &dataImageProvisioner{
		Provisioner:   baseProv,
		imageAttached: f.imageAttached,
		statusError:   f.statusError,
	}, nil
}

func createDataImageReconciler(t *testing.T, fix provisioner.Factory, initObjs ...runtime.Object) *DataImageReconciler {
	t.Helper()
	clientBuilder := fakeclient.NewClientBuilder().WithRuntimeObjects(initObjs...)
	for _, v := range initObjs {
		object, ok := v.(client.Object)
		require.True(t, ok, "failed to cast object to client.Object")
		clientBuilder = clientBuilder.WithStatusSubresource(object)
	}
	c := clientBuilder.Build()

	return &DataImageReconciler{
		Client:             c,
		ProvisionerFactory: fix,
		Log:                ctrl.Log.WithName("controllers").WithName("DataImage"),
	}
}

func createDataImage(name string, url string) *metal3api.DataImage {
	return &metal3api.DataImage{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DataImage",
			APIVersion: "metal3.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: metal3api.DataImageSpec{
			URL: url,
		},
	}
}

func createDataImageWithFinalizer(name string, url string) *metal3api.DataImage {
	di := createDataImage(name, url)
	di.Finalizers = []string{metal3api.DataImageFinalizer}
	return di
}

func createDataImageBeingDeleted(name string, url string) *metal3api.DataImage {
	di := createDataImageWithFinalizer(name, url)
	now := metav1.Now()
	di.DeletionTimestamp = &now
	return di
}

func createDataImageRequest(di *metal3api.DataImage) ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: di.Namespace,
			Name:      di.Name,
		},
	}
}

// TestDataImageAddFinalizer verifies that when a DataImage is created without
// a finalizer and a matching BareMetalHost exists, the reconciler adds the
// finalizer.
func TestDataImageAddFinalizer(t *testing.T) {
	host := newDefaultHost(t)
	di := createDataImage(t.Name(), "http://example.com/image.iso")
	fix := &fixture.Fixture{}

	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)

	// Should continue reconciliation after setting owner reference
	assert.True(t, result.RequeueAfter > 0)

	// Wait for finalizer appears
	updatedDI := &metal3api.DataImage{}
	for i := 0; i < 10; i++ {
		_, err = r.Reconcile(t.Context(), request)
		require.NoError(t, err)
		err = r.Get(t.Context(), request.NamespacedName, updatedDI)
		require.NoError(t, err)
		if len(updatedDI.Finalizers) > 0 {
			break
		}
	}

	assert.Contains(t, updatedDI.Finalizers, metal3api.DataImageFinalizer,
		"expected finalizer to be added to DataImage")
}

// TestDataImageRemoveFinalizerNoBMH tests that if a DataImage is marked
// for deletion and no matching BareMetalHost exists, the finalizer is removed
func TestDataImageRemoveFinalizerNoBMH(t *testing.T) {
	di := createDataImageBeingDeleted(t.Name(), "http://example.com/image.iso")
	fix := &fixture.Fixture{}

	r := createDataImageReconciler(t, fix, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)

	// Verify the finalizer was removed
	updatedDI := &metal3api.DataImage{}
	err = r.Get(t.Context(), request.NamespacedName, updatedDI)
	require.NoError(t, err)
	assert.NotContains(t, updatedDI.Finalizers, metal3api.DataImageFinalizer,
		"expected finalizer to be removed when no BMH exists")
}

// TestDataImageRemoveFinalizerDetachedBMH checks that when a DataImage is
// marked for deletion and the associated BareMetalHost has the detached
// annotation, the finalizer is removed without checking the status.
func TestDataImageRemoveFinalizerDetachedBMH(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.DetachedAnnotation: "{}",
	}
	di := createDataImageBeingDeleted(t.Name(), "http://example.com/image.iso")

	// Set owner reference manually
	di.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
			Name:       host.Name,
			UID:        host.UID,
		},
	}

	fix := &fixture.Fixture{}
	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)

	// Verify the finalizer was removed
	updatedDI := &metal3api.DataImage{}
	err = r.Get(t.Context(), request.NamespacedName, updatedDI)
	require.NoError(t, err)
	assert.NotContains(t, updatedDI.Finalizers, metal3api.DataImageFinalizer,
		"expected finalizer to be removed when BMH is detached")
}

// TestDataImageDetachedBMHNotDeleting verifies that when the BareMetalHost is
// detached but the DataImage is not being deleted, the controller requeues
// without removing the finalizer, waiting for the detached annotation to be
// removed.
func TestDataImageDetachedBMHNotDeleting(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.DetachedAnnotation: "{}",
	}
	di := createDataImageWithFinalizer(t.Name(), "http://example.com/image.iso")
	di.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
			Name:       host.Name,
			UID:        host.UID,
		},
	}

	fix := &fixture.Fixture{}
	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0,
		"expected requeue when BMH is detached and DataImage is not being deleted")

	// Verify the finalizer is still present
	updatedDI := &metal3api.DataImage{}
	err = r.Get(t.Context(), request.NamespacedName, updatedDI)
	require.NoError(t, err)
	assert.Contains(t, updatedDI.Finalizers, metal3api.DataImageFinalizer,
		"expected finalizer to remain when BMH is detached but DataImage is not being deleted")
}

// TestDataImageRemoveFinalizerImageDetached checks that when a DataImage is
// marked for deletion and the provisioner confirms the image is no longer
// attached to the server, the finalizer is removed
func TestDataImageRemoveFinalizerImageDetached(t *testing.T) {
	host := newDefaultHost(t)
	di := createDataImageBeingDeleted(t.Name(), "http://example.com/image.iso")
	di.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
			Name:       host.Name,
			UID:        host.UID,
		},
	}

	// Image is detached
	fix := &dataImageFixture{
		imageAttached: false,
		statusError:   nil,
	}

	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)

	updatedDI := &metal3api.DataImage{}
	err = r.Get(t.Context(), request.NamespacedName, updatedDI)
	require.NoError(t, err)
	assert.NotContains(t, updatedDI.Finalizers, metal3api.DataImageFinalizer,
		"expected finalizer to be removed when image is detached")
}

// TestDataImageKeepFinalizerImageAttached verifies that when a DataImage is
// marked for deletion but the image is still attached to the server's virtual
// media, the finalizer is NOT removed and reconciliation requeues to wait for
// the detachment to complete.
func TestDataImageKeepFinalizerImageAttached(t *testing.T) {
	host := newDefaultHost(t)
	di := createDataImageBeingDeleted(t.Name(), "http://example.com/image.iso")
	di.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
			Name:       host.Name,
			UID:        host.UID,
		},
	}

	// Image is still attached
	fix := &dataImageFixture{
		imageAttached: true,
		statusError:   nil,
	}

	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0,
		"expected requeue when image is still attached")

	updatedDI := &metal3api.DataImage{}
	err = r.Get(t.Context(), request.NamespacedName, updatedDI)
	require.NoError(t, err)
	assert.Contains(t, updatedDI.Finalizers, metal3api.DataImageFinalizer,
		"expected finalizer to remain while image is still attached")
}

// TestDataImageStatusErrorOnGetImageStatus checks that when
// GetDataImageStatus returns a non-transient error, the controller records
// the error message and count in the DataImage status and requeues.
func TestDataImageStatusErrorOnGetImageStatus(t *testing.T) {
	host := newDefaultHost(t)
	di := createDataImageWithFinalizer(t.Name(), "http://example.com/image.iso")
	di.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
			Name:       host.Name,
			UID:        host.UID,
		},
	}

	fix := &dataImageFixture{
		imageAttached: false,
		statusError:   fmt.Errorf("virtual media error"),
	}

	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0,
		"expected requeue when GetDataImageStatus fails")

	updatedDI := &metal3api.DataImage{}
	err = r.Get(t.Context(), request.NamespacedName, updatedDI)
	require.NoError(t, err)
	assert.Equal(t, "virtual media error", updatedDI.Status.Error.Message)
	assert.Equal(t, 1, updatedDI.Status.Error.Count)
}

// TestDataImagePausedBMH verifies that when the associated BareMetalHost has
// the paused annotation, reconciliation returns immediately without requeue
// since the controller should not act on paused hosts.
func TestDataImagePausedBMH(t *testing.T) {
	host := newDefaultHost(t)
	host.Annotations = map[string]string{
		metal3api.PausedAnnotation: "true",
	}
	di := createDataImageWithFinalizer(t.Name(), "http://example.com/image.iso")
	di.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
			Name:       host.Name,
			UID:        host.UID,
		},
	}

	fix := &fixture.Fixture{}
	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter,
		"expected no requeue when BMH is paused")
}

// TestDataImageNotFound verifies that when the DataImage resource does not
// exist in the cluster, reconciliation returns successfully with no error
// and no requeue since the resource was already deleted.
func TestDataImageNotFound(t *testing.T) {
	fix := &fixture.Fixture{}
	r := createDataImageReconciler(t, fix)

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      "nonexistent-dataimage",
		},
	}

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
}

// TestDataImageProvisionerNotReady verifies that when the provisioner is not
// yet ready to accept requests, the controller requeues with the standard
// provisioner retry delay.
func TestDataImageProvisionerNotReady(t *testing.T) {
	host := newDefaultHost(t)
	di := createDataImageWithFinalizer(t.Name(), "http://example.com/image.iso")
	di.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
			Name:       host.Name,
			UID:        host.UID,
		},
	}

	fix := &dataImageFixture{}
	fix.BecomeReadyCounter = 1 // Provisioner not ready on first attempt

	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.Equal(t, provisionerRetryDelay, result.RequeueAfter,
		"expected requeue when provisioner is not ready")
}

// TestDataImageNodeBusyError verifies that when GetDataImageStatus returns
// ErrNodeIsBusy, the controller requeues without recording an error
// status since this is a transient condition.
func TestDataImageNodeBusyError(t *testing.T) {
	host := newDefaultHost(t)
	di := createDataImageWithFinalizer(t.Name(), "http://example.com/image.iso")
	di.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "BareMetalHost",
			Name:       host.Name,
			UID:        host.UID,
		},
	}

	fix := &dataImageFixture{
		imageAttached: false,
		statusError:   provisioner.ErrNodeIsBusy,
	}

	r := createDataImageReconciler(t, fix, host, di)
	request := createDataImageRequest(di)

	result, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	assert.Equal(t, dataImageUpdateDelay, result.RequeueAfter,
		"expected requeue when node is busy")

	// Verify error was NOT recorded in status (ErrNodeIsBusy is transient)
	updatedDI := &metal3api.DataImage{}
	err = r.Get(t.Context(), request.NamespacedName, updatedDI)
	require.NoError(t, err)
	assert.Empty(t, updatedDI.Status.Error.Message,
		"expected no error message for ErrNodeIsBusy")
	assert.Zero(t, updatedDI.Status.Error.Count,
		"expected error count to remain zero for ErrNodeIsBusy")
}
