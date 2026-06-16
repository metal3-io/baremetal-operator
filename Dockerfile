# Support FROM override
ARG BUILD_IMAGE=docker.io/golang:1.25.11@sha256:dd7d32e19b28621cd982082397fc0510d396805b717d5e77466aa2dd692340de
ARG BASE_IMAGE=gcr.io/distroless/base-debian13:nonroot@sha256:a557d784ac275c287d2bdf3172f47bece8d2a0ef3c0fdefb712e95084a04a562

# Shared SDK stage: pinned Go toolchain, modules, and checked-out tree.
# Downstream builder stages copy from here so we pay the `go mod download`
# cost once. Third parties can also build custom provisioner plugins against
# this image to guarantee toolchain and module-version parity with BMO.
FROM $BUILD_IMAGE AS sdk

WORKDIR /workspace

COPY go.mod go.sum ./
COPY apis/go.mod apis/go.sum apis/
COPY hack/tools/go.mod hack/tools/go.sum hack/tools/
COPY pkg/hardwareutils/go.mod pkg/hardwareutils/go.sum pkg/hardwareutils/
RUN go mod download

COPY . .

ENV CGO_ENABLED=1
ENV GO111MODULE=on

# Build the manager binary
FROM sdk AS builder
ARG ARCH=amd64
ARG LDFLAGS=-s -w
RUN GOOS=linux GOARCH=${ARCH} \
    go build -a -ldflags "${LDFLAGS}" -o baremetal-operator main.go

# Build the ironic provisioner plugin
FROM sdk AS ironic-plugin-builder
ARG ARCH=amd64
ARG LDFLAGS=-s -w
RUN GOOS=linux GOARCH=${ARCH} \
    go build -buildmode=plugin -ldflags "${LDFLAGS}" \
    -o ironic-provisioner.so ./pkg/provisioner/ironic/plugin/

# Build the demo provisioner plugin
FROM sdk AS demo-plugin-builder
ARG ARCH=amd64
ARG LDFLAGS=-s -w
RUN GOOS=linux GOARCH=${ARCH} \
    go build -buildmode=plugin -ldflags "${LDFLAGS}" \
    -o demo-provisioner.so ./pkg/provisioner/demo/plugin/

# Runtime image. Uses distroless/base (not static) because Go plugins need glibc.
FROM $BASE_IMAGE

# image.version is set during image build by automation
LABEL org.opencontainers.image.authors="metal3-dev@googlegroups.com"
LABEL org.opencontainers.image.description="This is the image for the Metal3 BareMetal Operator"
LABEL org.opencontainers.image.documentation="https://book.metal3.io/bmo/introduction"
LABEL org.opencontainers.image.licenses="Apache License 2.0"
LABEL org.opencontainers.image.title="Metal3 BareMetal Operator"
LABEL org.opencontainers.image.url="https://github.com/metal3-io/baremetal-operator"
LABEL org.opencontainers.image.vendor="Metal3-io"

WORKDIR /
COPY --from=builder /workspace/baremetal-operator .
COPY --from=ironic-plugin-builder /workspace/ironic-provisioner.so /plugins/ironic-provisioner.so
COPY --from=demo-plugin-builder /workspace/demo-provisioner.so /plugins/demo-provisioner.so
USER nonroot:nonroot
ENTRYPOINT ["/baremetal-operator"]
