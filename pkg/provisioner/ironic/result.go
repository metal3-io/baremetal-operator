package ironic

import (
	"math/rand/v2"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

// jitterFraction is the maximum fraction of jitter to add (1/4 = 25%).
const jitterFraction = 4

// jitter adds randomness to requeue delays to avoid coordinated load spikes
// when many hosts requeue simultaneously. Override in tests for determinism.
var jitter = func(delay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}
	// Add 0-25% jitter
	maxJitter := int64(delay) / jitterFraction
	if maxJitter <= 0 {
		return delay
	}
	return delay + time.Duration(rand.Int64N(maxJitter)) //nolint:gosec // jitter does not need cryptographic randomness
}

func retryAfterDelay(delay time.Duration) (provisioner.Result, error) {
	// TODO(zaneb): this is currently indistinguishable from the result of
	// operationContinuing() from the caller's perspective. Changes are
	// required to the Result structure to enable this to be distinguished.
	return provisioner.Result{
		Dirty:        true,
		RequeueAfter: jitter(delay),
	}, nil
}

func operationContinuing(delay time.Duration) (provisioner.Result, error) {
	return provisioner.Result{
		Dirty:        true,
		RequeueAfter: jitter(delay),
	}, nil
}

func operationContinuingWithState(delay time.Duration, node *nodes.Node) (provisioner.Result, error) {
	activity, progress, index, allSteps := parseSubStates(node)
	return provisioner.Result{
		Dirty:            true,
		RequeueAfter:     delay,
		CurrentActivity:  activity,
		Progress:         progress,
		CurrentStepIndex: index,
		AllSteps:         allSteps,
	}, nil
}

func operationComplete() (provisioner.Result, error) {
	return provisioner.Result{}, nil
}

func operationFailed(message string) (provisioner.Result, error) {
	return provisioner.Result{ErrorMessage: message}, nil
}

func transientError(err error) (provisioner.Result, error) {
	return provisioner.Result{}, err
}
