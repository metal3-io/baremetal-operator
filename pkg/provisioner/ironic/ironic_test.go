package ironic

import (
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestChecksumIsURLNo(t *testing.T) {
	isURL, err := checksumIsURL("checksum-goes-here")
	if isURL {
		t.Fail()
	}
	if err != nil {
		t.Fail()
	}
}

func TestChecksumIsURLYes(t *testing.T) {
	isURL, err := checksumIsURL("http://checksum-goes-here")
	if !isURL {
		t.Fail()
	}
	if err != nil {
		t.Fail()
	}
}
