# Set up the builder image
FROM registry.svc.ci.openshift.org/openshift/release:golang-1.14 AS builder

WORKDIR /go/src/github.com/metal3-io/baremetal-operator

# Copy all of the source into the build image
COPY . .

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
#
# NOTE: `go mod download` should do the same thing, but does not work, resulting in
# errors like go build sigs.k8s.io/controller-runtime/pkg/runtime/log: no Go files in /go/src/github.com/metal3-io/baremetal-operator/vendor/sigs.k8s.io/controller-runtime/pkg/runtime/log
RUN go mod vendor

# Build controller binary
RUN CGO_ENABLED=0 make -e manager

# Set up the runtime image
FROM quay.io/metal3-io/base-image

# Add dependencies
RUN if ! rpm -q genisoimage; \
    then yum install -y genisoimage && \
    yum clean all && \
    rm -rf /var/cache/yum/*; \
    fi

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/bin/manager .
USER nonroot:nonroot

LABEL io.k8s.display-name="Metal3 BareMetal Operator" \
      io.k8s.description="This is the image for the Metal3 BareMetal Operator."

ENTRYPOINT ["/manager"]
