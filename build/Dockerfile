FROM registry.hub.docker.com/library/golang:1.14 AS builder
WORKDIR /go/src/github.com/metal3-io/baremetal-operator
COPY . .
RUN make build

# Copy the controller-manager into a thin image
# BMO has a dependency preventing us to use the static one,
# using the base one instead
FROM gcr.io/distroless/base:latest
WORKDIR /
COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/build/_output/bin/baremetal-operator /
USER nobody

LABEL io.k8s.display-name="Metal3 BareMetal Operator" \
      io.k8s.description="This is the image for the Metal3 BareMetal Operator."
