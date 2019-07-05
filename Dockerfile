FROM registry.svc.ci.openshift.org/openshift/release:golang-1.10 AS builder
WORKDIR /go/src/github.com/metal3-io/baremetal-operator
COPY . .
RUN make build

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/build/_output/bin/baremetal-operator /
RUN if ! rpm -q genisoimage; then yum install -y genisoimage && yum clean all && rm -rf /var/cache/yum/*; fi
