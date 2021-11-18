/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metal3 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/imageprovider"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
)

const (
	minRetryDelay = time.Second * 10
	maxRetryDelay = time.Minute * 10
)

// PreprovisioningImageReconciler reconciles a PreprovisioningImage object
type PreprovisioningImageReconciler struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	APIReader     client.Reader
	ImageProvider imageprovider.ImageProvider
}

type imageConditionReason string

const (
	reasonImageSuccess            imageConditionReason = "ImageSuccess"
	reasonImageConfigurationError imageConditionReason = "ConfigurationError"
	reasonImageMissingNetworkData imageConditionReason = "MissingNetworkData"
)

// +kubebuilder:rbac:groups=metal3.io,resources=preprovisioningimages,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=metal3.io,resources=preprovisioningimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update

func (r *PreprovisioningImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("preprovisioningimage", req.NamespacedName)

	result := ctrl.Result{}

	img := metal3.PreprovisioningImage{}
	err := r.Get(ctx, req.NamespacedName, &img)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("PreprovisioningImage not found")
			err = nil
		}
		return ctrl.Result{}, err
	}

	changed, err := r.update(&img, log)

	if k8serrors.IsNotFound(err) {
		delay := getErrorRetryDelay(img.Status)
		log.Info("requeuing to check for secret", "after", delay)
		result.RequeueAfter = delay
	}
	if changed {
		log.Info("updating status")
		err = r.Status().Update(ctx, &img)
	}

	return result, err
}

func (r *PreprovisioningImageReconciler) update(img *metal3.PreprovisioningImage, log logr.Logger) (bool, error) {
	generation := img.GetGeneration()

	format := r.getImageFormat(img.Spec, log)
	if format == "" {
		return setError(generation, &img.Status, reasonImageConfigurationError, "No acceptable image format supported"), nil
	}

	url, err := r.ImageProvider.BuildImage(format)
	if err != nil {
		return false, err
	}

	log.Info("image URL available", "url", url, "format", format)
	secretManager := secretutils.NewSecretManager(log, r.Client, r.APIReader)
	secretStatus, err := getNetworkDataStatus(secretManager, img)
	if err == nil {
		return setImage(generation, &img.Status, url, format,
			secretStatus, img.Spec.Architecture,
			"Generated image"), nil
	}

	if k8serrors.IsNotFound(err) {
		log.Info("network data Secret does not exist")
		return setError(generation, &img.Status, reasonImageMissingNetworkData, "NetworkData secret not found"), err
	}

	return false, err
}

func (r *PreprovisioningImageReconciler) getImageFormat(spec metal3.PreprovisioningImageSpec, log logr.Logger) (format metal3.ImageFormat) {
	for _, acceptableFormat := range spec.AcceptFormats {
		if r.ImageProvider.SupportsFormat(acceptableFormat) {
			return acceptableFormat
		}
	}

	if len(spec.AcceptFormats) > 0 {
		log = log.WithValues("preferredFormat", spec.AcceptFormats[0])
	}
	log.Info("no acceptable image format supported")
	return
}

func getErrorRetryDelay(status metal3.PreprovisioningImageStatus) time.Duration {
	errorCond := meta.FindStatusCondition(status.Conditions, string(metal3.ConditionImageError))
	if errorCond == nil || errorCond.Status != metav1.ConditionTrue {
		return 0
	}

	// exponential delay
	delay := time.Since(errorCond.LastTransitionTime.Time) + minRetryDelay

	if delay > maxRetryDelay {
		return maxRetryDelay
	}
	return delay
}

func getNetworkDataStatus(secretManager secretutils.SecretManager, img *metal3.PreprovisioningImage) (metal3.SecretStatus, error) {
	networkDataSecret := img.Spec.NetworkDataName
	if networkDataSecret == "" {
		return metal3.SecretStatus{}, nil
	}

	secretKey := client.ObjectKey{
		Name:      networkDataSecret,
		Namespace: img.ObjectMeta.Namespace,
	}
	secret, err := secretManager.AcquireSecret(secretKey, img, false, false)
	if err != nil {
		return metal3.SecretStatus{}, err
	}

	return metal3.SecretStatus{
		Name:    networkDataSecret,
		Version: secret.GetResourceVersion(),
	}, nil
}

func setImageCondition(generation int64, status *metal3.PreprovisioningImageStatus,
	cond metal3.ImageStatusConditionType, newStatus metav1.ConditionStatus,
	time metav1.Time, reason imageConditionReason, message string) {
	newCondition := metav1.Condition{
		Type:               string(cond),
		Status:             newStatus,
		LastTransitionTime: time,
		ObservedGeneration: generation,
		Reason:             string(reason),
		Message:            message,
	}
	meta.SetStatusCondition(&status.Conditions, newCondition)
}

func setImage(generation int64, status *metal3.PreprovisioningImageStatus, url string,
	format metal3.ImageFormat, networkData metal3.SecretStatus, arch string,
	message string) bool {

	newStatus := status.DeepCopy()
	newStatus.ImageUrl = url
	newStatus.Format = format
	newStatus.Architecture = arch
	newStatus.NetworkData = networkData

	time := metav1.Now()
	reason := reasonImageSuccess
	setImageCondition(generation, newStatus,
		metal3.ConditionImageReady, metav1.ConditionTrue,
		time, reason, message)
	setImageCondition(generation, newStatus,
		metal3.ConditionImageError, metav1.ConditionFalse,
		time, reason, "")

	changed := !apiequality.Semantic.DeepEqual(status, &newStatus)
	*status = *newStatus
	return changed
}

func setError(generation int64, status *metal3.PreprovisioningImageStatus, reason imageConditionReason, message string) bool {
	newStatus := status.DeepCopy()
	newStatus.ImageUrl = ""

	time := metav1.Now()
	setImageCondition(generation, newStatus,
		metal3.ConditionImageReady, metav1.ConditionFalse,
		time, reason, "")
	setImageCondition(generation, newStatus,
		metal3.ConditionImageError, metav1.ConditionTrue,
		time, reason, message)

	changed := !apiequality.Semantic.DeepEqual(status, &newStatus)
	*status = *newStatus
	return changed
}

func (r *PreprovisioningImageReconciler) CanStart() bool {
	for _, fmt := range []metal3.ImageFormat{metal3.ImageFormatISO, metal3.ImageFormatInitRD} {
		if r.ImageProvider.SupportsFormat(fmt) {
			return true
		}
	}
	r.Log.Info("not starting preprovisioning image controller; no image data available")
	return false
}

func (r *PreprovisioningImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3.PreprovisioningImage{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
