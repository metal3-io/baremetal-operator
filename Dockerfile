FROM registry.svc.ci.openshift.org/ocp/builder:rhel-8-golang-1.15-openshift-4.6 AS builder
WORKDIR /go/src/github.com/metal3-io/baremetal-operator
COPY . .
RUN make build
RUN make tools

FROM registry.svc.ci.openshift.org/ocp/4.6:base
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/build/_output/bin/baremetal-operator /
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/build/_output/bin/get-hardware-details /
RUN if ! rpm -q genisoimage; then yum install -y genisoimage && yum clean all && rm -rf /var/cache/yum/*; fi
