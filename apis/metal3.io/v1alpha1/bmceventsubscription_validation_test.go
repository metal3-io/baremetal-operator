package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBMCEventSubscriptionValidateCreate(t *testing.T) {
	tm := metav1.TypeMeta{
		Kind:       "BMCEventSubscription",
		APIVersion: "metal3.io/v1alpha1",
	}

	om := metav1.ObjectMeta{
		Name:      "test",
		Namespace: "test-namespace",
	}

	tests := []struct {
		name      string
		newS      *BMCEventSubscription
		oldS      *BMCEventSubscription
		wantedErr string
	}{
		{
			name:      "valid",
			newS:      &BMCEventSubscription{TypeMeta: tm, ObjectMeta: om, Spec: BMCEventSubscriptionSpec{HostName: "worker-01", Destination: "http://localhost/abc/abc.php"}},
			oldS:      nil,
			wantedErr: "",
		},
		{
			name: "missingHostName",
			newS: &BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec:       BMCEventSubscriptionSpec{Destination: "http://localhost/abc/abc"},
			},
			oldS:      nil,
			wantedErr: "hostName cannot be empty",
		},
		{
			name: "missingDestination",
			newS: &BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec:       BMCEventSubscriptionSpec{HostName: "worker-01"},
			},
			oldS:      nil,
			wantedErr: "destination cannot be empty",
		},
		{
			name: "destinationNotUrl",
			newS: &BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec:       BMCEventSubscriptionSpec{HostName: "worker-01", Destination: "abc"},
			},
			oldS:      nil,
			wantedErr: "destination is invalid: parse \"abc\": invalid URI for request",
		},
		{
			name: "destinationMissingTrailingSlash",
			newS: &BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec:       BMCEventSubscriptionSpec{HostName: "worker-01", Destination: "http://localhost"},
			},
			oldS:      nil,
			wantedErr: "hostname-only destination must have a trailing slash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.newS.validateSubscription(); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("BareMetalHost.validateSubscription() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
