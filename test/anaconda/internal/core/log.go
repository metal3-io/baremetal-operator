// SPDX-License-Identifier: Apache-2.0

// Package core holds the values every layer shares. It imports none of them,
// which is what keeps the redfish, kube and httpapi packages from cycling.
package core

import (
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var Log = logf.Log.WithName("provisioner").WithName("anaconda")
