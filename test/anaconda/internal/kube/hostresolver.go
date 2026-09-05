// SPDX-License-Identifier: Apache-2.0

// Package kube is everything the plugin reads from or writes to the cluster,
// the host, its kickstart Secret and the annotations carrying install state.
package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"metal3.local/anaconda/internal/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HostResolver struct {
	Client client.Client
	// APIReader is the uncached reader used when a get bypasses the informer cache.
	APIReader client.Reader
}

// WWNLink names the by-id link udev creates for a WWN. The hint is free text
// and may or may not carry the 0x prefix, so both forms are normalized.
func WWNLink(wwn string) string {
	wwn = strings.TrimSpace(wwn)

	if !strings.HasPrefix(wwn, "0x") {
		wwn = "0x" + wwn
	}

	return "disk/by-id/wwn-" + wwn
}

// RootDeviceSpec renders root device hints as a kickstart device spec, most
// specific hint first. Only a device name or a by-id link is expressible.
func RootDeviceSpec(hints *metal3api.RootDeviceHints) string {
	if hints == nil {
		return ""
	}

	switch {
	case hints.DeviceName != "":
		return core.NormalizeDisk(hints.DeviceName)

	// udev folds the vendor extension into the same wwn link, so the longer hint
	// is tried first and both still land on a path that exists.
	case hints.WWNWithExtension != "":
		return WWNLink(hints.WWNWithExtension)

	case hints.WWN != "":
		return WWNLink(hints.WWN)

	// The by-id prefix is the transport and the hint does not carry it, so the
	// serial matches as a suffix. An empty serial never reaches this arm.
	case hints.SerialNumber != "":
		return "disk/by-id/*" + hints.SerialNumber
	}

	// Model, vendor, size, rotational and HCTL select by property, which no
	// kickstart command can express, so the configured default has to answer.
	return ""
}

// GetHost fetches the BareMetalHost behind a call. The caller name goes into the
// error when no Kubernetes client is configured.
func (r *HostResolver) GetHost(ctx context.Context, caller, namespace, name string) (*metal3api.BareMetalHost, error) {
	if r.Client == nil {
		return nil, fmt.Errorf("%s requires a Kubernetes client", caller)
	}

	host := &metal3api.BareMetalHost{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, host); err != nil {
		return nil, fmt.Errorf("get BareMetalHost: %w", err)
	}

	return host, nil
}

func NewHostRef(host *metal3api.BareMetalHost) core.HostRef {
	return core.HostRef{
		Name:            host.Name,
		Namespace:       host.Namespace,
		UID:             string(host.UID),
		BootMAC:         host.Spec.BootMACAddress,
		KickstartSecret: host.Spec.PreprovisioningNetworkDataName,
		InstallDisk:     RootDeviceSpec(host.Spec.RootDeviceHints),
	}
}

// FindHostsByMAC returns the hosts declaring any of the MACs as their boot MAC.
// An inspected NIC is not a match, provisioning requires the field to be set.
func (r *HostResolver) FindHostsByMAC(ctx context.Context, macs []string) ([]core.HostRef, error) {
	if r.Client == nil {
		return nil, errors.New("kickstart lookup requires a Kubernetes client")
	}

	want := make(map[string]bool, len(macs))

	for _, m := range macs {
		if n := core.NormalizeMAC(m); n != "" {
			want[n] = true
		}
	}

	if len(want) == 0 {
		return nil, nil
	}

	// Unscoped, so the host's own namespace is what every later read uses. The
	// cached client already narrows this to whatever BMO was told to watch.
	list := &metal3api.BareMetalHostList{}
	if err := r.Client.List(ctx, list); err != nil {
		return nil, err
	}

	var found []core.HostRef

	for i := range list.Items {
		host := &list.Items[i]

		// An empty boot MAC normalizes to the empty string, which is never in
		// the wanted set, so a host declaring none cannot match.
		if want[core.NormalizeMAC(host.Spec.BootMACAddress)] {
			found = append(found, NewHostRef(host))
		}
	}

	return found, nil
}

// Install state is kept in annotations on the host. The status subresource is
// not usable for it, BMO replaces the whole status from its own copy each pass.
const (
	InstallResultAnnotation  = "anaconda.metal3.io/install-result"
	InstallMessageAnnotation = "anaconda.metal3.io/install-message"
	// InstallStartedAnnotation is stamped when the ISO goes in. BMO's own
	// provision start is stamped on entering provisioning, which can be hours older.
	InstallStartedAnnotation = "anaconda.metal3.io/install-started"
)

// InstallResultSucceeded and InstallResultFailed are the two values the result
// annotation takes, readable with kubectl get bmh -o yaml.
const (
	InstallResultSucceeded = "succeeded"
	InstallResultFailed    = "failed"
)

// MutateHostAnnotations patches the host annotations. A merge patch carries no
// resourceVersion, so a controller write at the same moment cannot fail it.
func (r *HostResolver) MutateHostAnnotations(ctx context.Context, namespace, name string, mutate func(map[string]string)) error {
	host, err := r.GetHost(ctx, "annotate", namespace, name)
	if err != nil {
		return err
	}

	patch := client.MergeFrom(host.DeepCopy())

	annotations := host.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}

	mutate(annotations)
	host.Annotations = annotations

	return r.Client.Patch(ctx, host, patch)
}

// WriteInstallReport records the verdict on the host. The posted body is not
// kept, nothing reads it again and annotations share a 256 KiB budget.
func (r *HostResolver) WriteInstallReport(ctx context.Context, namespace, name string, report core.InstallReport) error {
	return r.MutateHostAnnotations(ctx, namespace, name, func(a map[string]string) {
		if report.Succeeded {
			a[InstallResultAnnotation] = InstallResultSucceeded
			delete(a, InstallMessageAnnotation)

			return
		}

		a[InstallResultAnnotation] = InstallResultFailed
		a[InstallMessageAnnotation] = report.Message
	})
}

// ReadInstallReport returns the recorded verdict, nil when none arrived yet.
func (r *HostResolver) ReadInstallReport(ctx context.Context, namespace, name string) (*core.InstallReport, error) {
	host, err := r.GetHost(ctx, "read install report", namespace, name)
	if err != nil {
		return nil, err
	}

	result, ok := host.Annotations[InstallResultAnnotation]
	if !ok {
		//nolint:nilnil // no verdict recorded yet is a normal answer, not a failure
		return nil, nil
	}

	return &core.InstallReport{
		Succeeded: result == InstallResultSucceeded,
		Message:   host.Annotations[InstallMessageAnnotation],
	}, nil
}

// ClearInstallReport drops the recorded verdict and the start stamp, so a
// reinstall cannot read the last run's and finish before anaconda has booted.
func (r *HostResolver) ClearInstallReport(ctx context.Context, namespace, name string) error {
	return r.MutateHostAnnotations(ctx, namespace, name, func(a map[string]string) {
		delete(a, InstallResultAnnotation)
		delete(a, InstallMessageAnnotation)
		delete(a, InstallStartedAnnotation)
	})
}

// MarkInstallStarted stamps the moment the ISO went in, which is what the
// install timeout runs from and what says the media in the drive is this run's.
func (r *HostResolver) MarkInstallStarted(ctx context.Context, namespace, name string, at time.Time) error {
	return r.MutateHostAnnotations(ctx, namespace, name, func(a map[string]string) {
		a[InstallStartedAnnotation] = at.UTC().Format(time.RFC3339)
	})
}

// InstallStartedAt returns when the ISO went in, zero when no install is under
// way. BMO's own stamp is not usable, it survives a host parked on a refusal.
func (r *HostResolver) InstallStartedAt(ctx context.Context, namespace, name string) (time.Time, error) {
	host, err := r.GetHost(ctx, "read install start", namespace, name)
	if err != nil {
		return time.Time{}, err
	}

	stamped, ok := host.Annotations[InstallStartedAnnotation]
	if !ok {
		return time.Time{}, nil
	}

	at, err := time.Parse(time.RFC3339, stamped)
	if err != nil {
		return time.Time{}, fmt.Errorf("install start %q on %s/%s: %w", stamped, namespace, name, err)
	}

	return at, nil
}

// HostUID returns the BareMetalHost UID, the only thing between an
// unauthenticated callback route and anyone able to guess a host name.
func (r *HostResolver) HostUID(ctx context.Context, namespace, name string) (string, error) {
	host, err := r.GetHost(ctx, "callback", namespace, name)
	if err != nil {
		return "", err
	}

	return string(host.UID), nil
}

// CallbackReader returns the uncached reader when available, since BMO caches
// only Secrets carrying its own label and a kickstart Secret carries none.
func (r *HostResolver) CallbackReader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}

	return r.Client
}

// HostHardwareData returns the details BMO keeps in the host's HardwareData CR,
// nil when there is none. Status carries a copy of this that can be stale.
func (r *HostResolver) HostHardwareData(ctx context.Context, namespace, name string) (*metal3api.HardwareDetails, error) {
	if r.Client == nil {
		return nil, errors.New("hardware data lookup requires a Kubernetes client")
	}

	// BMO names the HardwareData after the host and keeps it in the host's namespace.
	data := &metal3api.HardwareData{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, data); err != nil {
		if k8serrors.IsNotFound(err) {
			//nolint:nilnil // no HardwareData for this host is a normal answer, not a failure
			return nil, nil
		}

		return nil, err
	}

	return data.Spec.HardwareDetails, nil
}

// HasKickstart reports whether a host names a Secret that carries a kickstart,
// so no machine is booted only to be served the fallback and power itself off.
func (r *HostResolver) HasKickstart(ctx context.Context, namespace, name string) (bool, error) {
	host, err := r.GetHost(ctx, "kickstart precondition", namespace, name)
	if err != nil {
		return false, err
	}

	_, found, err := r.ReadKickstart(ctx, namespace, host.Spec.PreprovisioningNetworkDataName)

	return found, err
}

// HostInstallDisk renders spec.rootDeviceHints as a device spec, empty when unset.
// Status is unusable here, BMO fills it with the profile guess of /dev/sda.
func (r *HostResolver) HostInstallDisk(ctx context.Context, namespace, name string) (string, error) {
	host, err := r.GetHost(ctx, "root device hints", namespace, name)
	if err != nil {
		return "", err
	}

	return RootDeviceSpec(host.Spec.RootDeviceHints), nil
}

// ReadKickstart returns the kickstart in a host's preprovisioning Secret, found
// false when Secret or key is absent so the caller can serve the fallback.
func (r *HostResolver) ReadKickstart(ctx context.Context, namespace, secretName string) (string, bool, error) {
	if r.Client == nil {
		return "", false, errors.New("kickstart lookup requires a Kubernetes client")
	}

	if secretName == "" {
		return "", false, nil
	}

	sec := &corev1.Secret{}
	if err := r.CallbackReader().Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, sec); err != nil {
		if k8serrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, err
	}

	v, ok := sec.Data[core.KickstartSecretKey]
	if !ok {
		return "", false, nil
	}

	return string(v), true, nil
}
