FROM registry.svc.ci.openshift.org/ocp/builder:rhel-8-golang-1.15-openshift-4.7 AS builder
WORKDIR /go/src/github.com/metal3-io/baremetal-operator
COPY . .
RUN make build
RUN make tools

FROM registry.svc.ci.openshift.org/ocp/4.7:base
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/bin/manager /baremetal-operator
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/bin/get-hardware-details /
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/bin/make-bm-worker /
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/bin/make-virt-host /
RUN if ! rpm -q genisoimage; then yum install -y genisoimage && yum clean all && rm -rf /var/cache/yum/*; fi
