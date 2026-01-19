/*
Copyright 2025 The Metal3 Authors.

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

package hostclaim

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Requeue After Error tests", func() {
	It("Check a requeue error is catched", func() {
		duration := 20 * time.Second
		var err = &RequeueAfterError{RequeueAfter: duration}
		ok, t := IsRequeueAfterError(err)
		Expect(ok).To(BeTrue())
		Expect(t).To(Equal(duration))
	})
	It("Check it is catched even if behind wrapper", func() {
		duration := 30 * time.Second
		var err error = &RequeueAfterError{RequeueAfter: duration}
		err = errors.Wrap(err, "Wrapped error")
		ok, t := IsRequeueAfterError(err)
		Expect(ok).To(BeTrue())
		Expect(t).To(Equal(duration))
	})
	It("Other errors are not catched", func() {
		var err error = errors.New("other")
		err = errors.Wrap(err, "Wrapped error")
		ok, _ := IsRequeueAfterError(err)
		Expect(ok).To(BeFalse())
	})

})
