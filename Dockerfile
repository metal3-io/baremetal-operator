FROM registry.svc.ci.openshift.org/openshift/release:golang-1.14 AS builder
WORKDIR /go/src/github.com/metal3-io/baremetal-operator
COPY . .
# cache deps before building and copying source so that we don't need
# to re-download as much and so that source changes don't invalidate
# our downloaded layer
RUN go mod vendor
RUN make manager


FROM quay.io/metal3-io/base-image
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/bin/baremetal-operator /
RUN if ! rpm -q genisoimage; \
    then yum install -y genisoimage && \
    yum clean all && \
    rm -rf /var/cache/yum/*; \
    fi
LABEL io.k8s.display-name="Metal3 BareMetal Operator" \
      io.k8s.description="This is the image for the Metal3 BareMetal Operator."
