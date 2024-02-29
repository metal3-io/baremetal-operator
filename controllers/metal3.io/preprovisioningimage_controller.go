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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/imageprovider"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	"github.com/metal3-io/baremetal-operator/pkg/utils"
)

const (
	minRetryDelay = time.Second * 10
	maxRetryDelay = time.Minute * 10
)

// PreprovisioningImageReconciler reconciles a PreprovisioningImage object.
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
	reasonImageBuildInvalid       imageConditionReason = "ImageBuildInvalid"
)

// +kubebuilder:rbac:groups=metal3.io,resources=preprovisioningimages,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=metal3.io,resources=preprovisioningimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update

func (r *PreprovisioningImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("preprovisioningimage", req.NamespacedName)

	result := ctrl.Result{}

	img := metal3api.PreprovisioningImage{}
	err := r.Get(ctx, req.NamespacedName, &img)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("PreprovisioningImage not found")
			err = nil
		}
		return ctrl.Result{}, err
	}

	if !img.DeletionTimestamp.IsZero() {
		log.Info("cleaning up deleted resource")
		if err := r.discardExistingImage(&img, log); err != nil {
			return ctrl.Result{}, err
		}
		img.Finalizers = utils.FilterStringFromList(
			img.Finalizers, metal3api.PreprovisioningImageFinalizer)
		err := r.Update(ctx, &img)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}
		return ctrl.Result{}, nil
	}

	if !utils.StringInList(img.Finalizers, metal3api.PreprovisioningImageFinalizer) {
		log.Info("adding finalizer")
		img.Finalizers = append(img.Finalizers, metal3api.PreprovisioningImageFinalizer)
		err := r.Update(ctx, &img)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to add finalizer")
		}
		return ctrl.Result{}, nil
	}

	changed, err := r.update(ctx, &img, log)

	if k8serrors.IsNotFound(err) {
		delay := getErrorRetryDelay(img.Status)
		log.Info("requeuing to check for secret", "after", delay)
		result.RequeueAfter = delay
	}

	notReady := imageprovider.ImageNotReady{}
	if errors.As(err, &notReady) {
		log.Info("image is not ready yet, requeuing", "after", minRetryDelay)
		if setUnready(img.GetGeneration(), &img.Status, err.Error()) {
			changed = true
		}
		result.RequeueAfter = minRetryDelay
	}

	if changed {
		log.Info("updating status")
		err = r.Status().Update(ctx, &img)
	}

	return result, err
}

func configChanged(img *metal3api.PreprovisioningImage, format metal3api.ImageFormat, networkDataStatus metal3api.SecretStatus) bool {
	return !(img.Status.Format == format &&
		img.Status.Architecture == img.Spec.Architecture &&
		img.Status.NetworkData == networkDataStatus)
}

func (r *PreprovisioningImageReconciler) update(ctx context.Context, img *metal3api.PreprovisioningImage, log logr.Logger) (bool, error) {
	generation := img.GetGeneration()

	if !r.ImageProvider.SupportsArchitecture(img.Spec.Architecture) {
		log.Info("image architecture not supported", "architecture", img.Spec.Architecture)
		return setError(generation, &img.Status, reasonImageConfigurationError, "Architecture not supported"), nil
	}

	format := r.getImageFormat(img.Spec, log)
	if format == "" {
		return setError(generation, &img.Status, reasonImageConfigurationError, "No acceptable image format supported"), nil
	}

	secretManager := secretutils.NewSecretManager(ctx, log, r.Client, r.APIReader)
	networkData, secretStatus, err := getNetworkData(secretManager, img)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("network data Secret does not exist")
			return setError(generation, &img.Status, reasonImageMissingNetworkData, "NetworkData secret not found"), err
		}
		return false, err
	}

	if configChanged(img, format, secretStatus) {
		reason := "Config changed"
		if meta.IsStatusConditionTrue(img.Status.Conditions, string(metal3api.ConditionImageReady)) {
			// Ensure we mark the status as not ready before we remove the build
			// from the image cache.
			setUnready(generation, &img.Status, reason)
		} else {
			if err := r.discardExistingImage(img, log); err != nil {
				return false, err
			}
			// Set up all the data before building the image and adding the URL,
			// so that even if we fail to write the built image status and the
			// config subsequently changes, the image cache cannot leak.
			setImage(generation, &img.Status, imageprovider.GeneratedImage{},
				format, secretStatus, img.Spec.Architecture,
				reason)
		}
		return true, nil
	}

	var networkDataContent imageprovider.NetworkData
	if networkData != nil {
		networkDataContent = networkData.Data
	}
	image, err := r.ImageProvider.BuildImage(imageprovider.ImageData{
		ImageMetadata:     img.ObjectMeta.DeepCopy(),
		Format:            format,
		Architecture:      img.Spec.Architecture,
		NetworkDataStatus: secretStatus,
	}, networkDataContent, log)
	if err != nil {
		failure := imageprovider.ImageBuildInvalid{}
		if errors.As(err, &failure) {
			log.Info("image build failed", "error", "err")
			return setError(generation, &img.Status, reasonImageBuildInvalid, failure.Error()), nil
		}
		return false, err
	}
	log.Info("image URL available", "url", image, "format", format)

	return setImage(generation, &img.Status, image, format,
		secretStatus, img.Spec.Architecture,
		"Generated image"), nil
}

func (r *PreprovisioningImageReconciler) getImageFormat(spec metal3api.PreprovisioningImageSpec, log logr.Logger) (format metal3api.ImageFormat) {
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

func (r *PreprovisioningImageReconciler) discardExistingImage(img *metal3api.PreprovisioningImage, log logr.Logger) error {
	if img.Status.Format == "" {
		return nil
	}
	log.Info("discarding existing image", "image_url", img.Status.ImageUrl)
	return r.ImageProvider.DiscardImage(imageprovider.ImageData{
		ImageMetadata:     img.ObjectMeta.DeepCopy(),
		Format:            img.Status.Format,
		Architecture:      img.Status.Architecture,
		NetworkDataStatus: img.Status.NetworkData,
	})
}

func getErrorRetryDelay(status metal3api.PreprovisioningImageStatus) time.Duration {
	errorCond := meta.FindStatusCondition(status.Conditions, string(metal3api.ConditionImageError))
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

func getNetworkData(secretManager secretutils.SecretManager, img *metal3api.PreprovisioningImage) (*corev1.Secret, metal3api.SecretStatus, error) {
	networkDataSecret := img.Spec.NetworkDataName
	if networkDataSecret == "" {
		return nil, metal3api.SecretStatus{}, nil
	}

	secretKey := client.ObjectKey{
		Name:      networkDataSecret,
		Namespace: img.ObjectMeta.Namespace,
	}
	secret, err := secretManager.AcquireSecret(secretKey, img, false)
	if err != nil {
		return nil, metal3api.SecretStatus{}, err
	}

	return secret, metal3api.SecretStatus{
		Name:    networkDataSecret,
		Version: secret.GetResourceVersion(),
	}, nil
}

func setImageCondition(generation int64, status *metal3api.PreprovisioningImageStatus,
	cond metal3api.ImageStatusConditionType, newStatus metav1.ConditionStatus,
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

func setImage(generation int64, status *metal3api.PreprovisioningImageStatus, image imageprovider.GeneratedImage,
	format metal3api.ImageFormat, networkData metal3api.SecretStatus, arch string,
	message string) bool {
	newStatus := status.DeepCopy()
	newStatus.ImageUrl = image.ImageURL
	newStatus.KernelUrl = image.KernelURL
	newStatus.ExtraKernelParams = image.ExtraKernelParams
	newStatus.Format = format
	newStatus.Architecture = arch
	newStatus.NetworkData = networkData

	time := metav1.Now()
	reason := reasonImageSuccess
	ready := metav1.ConditionFalse
	if newStatus.ImageUrl != "" {
		ready = metav1.ConditionTrue
	}
	setImageCondition(generation, newStatus,
		metal3api.ConditionImageReady, ready,
		time, reason, message)
	setImageCondition(generation, newStatus,
		metal3api.ConditionImageError, metav1.ConditionFalse,
		time, reason, "")

	changed := !apiequality.Semantic.DeepEqual(status, &newStatus)
	*status = *newStatus
	return changed
}

func setUnready(generation int64, status *metal3api.PreprovisioningImageStatus, message string) bool {
	newStatus := status.DeepCopy()

	time := metav1.Now()
	reason := reasonImageSuccess
	setImageCondition(generation, newStatus,
		metal3api.ConditionImageReady, metav1.ConditionFalse,
		time, reason, message)

	changed := !apiequality.Semantic.DeepEqual(status, &newStatus)
	*status = *newStatus
	return changed
}

func setError(generation int64, status *metal3api.PreprovisioningImageStatus, reason imageConditionReason, message string) bool {
	newStatus := status.DeepCopy()
	newStatus.ImageUrl = ""

	time := metav1.Now()
	setImageCondition(generation, newStatus,
		metal3api.ConditionImageReady, metav1.ConditionFalse,
		time, reason, "")
	setImageCondition(generation, newStatus,
		metal3api.ConditionImageError, metav1.ConditionTrue,
		time, reason, message)

	changed := !apiequality.Semantic.DeepEqual(status, &newStatus)
	*status = *newStatus
	return changed
}

func (r *PreprovisioningImageReconciler) CanStart() bool {
	for _, fmt := range []metal3api.ImageFormat{metal3api.ImageFormatISO, metal3api.ImageFormatInitRD} {
		if r.ImageProvider.SupportsFormat(fmt) {
			return true
		}
	}
	r.Log.Info("not starting preprovisioning image controller; no image data available")
	return false
}

func (r *PreprovisioningImageReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconcile int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.PreprovisioningImage{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconcile}).
		Owns(&corev1.Secret{}, builder.MatchEveryOwner).
		Complete(r)
}
