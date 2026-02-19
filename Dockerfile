# Support FROM override
ARG BUILD_IMAGE=docker.io/golang:1.24.13@sha256:c29cdf32d47053ab0d914852d9c2ed2da12b3cf13079aaef1704ef21335e68a3
ARG BASE_IMAGE=gcr.io/distroless/static:nonroot@sha256:9ecc53c269509f63c69a266168e4a687c7eb8c0cfd753bd8bfcaa4f58a90876f

# Build the manager binary
FROM $BUILD_IMAGE AS builder

WORKDIR /workspace

# Bring in the go dependencies before anything else so we can take
# advantage of caching these layers in future builds.
COPY go.mod go.sum ./
COPY apis/go.mod apis/go.sum apis/
COPY hack/tools/go.mod hack/tools/go.sum hack/tools/
COPY pkg/hardwareutils/go.mod pkg/hardwareutils/go.sum pkg/hardwareutils/
RUN go mod download
ARG LDFLAGS=-s -w -extldflags=-static

COPY . .
ARG ARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} GO111MODULE=on go build -a -ldflags "${LDFLAGS}" -o baremetal-operator main.go

# Copy the controller-manager into a thin image
# BMO has a dependency preventing us to use the static one,
# using the base one instead
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
USER nonroot:nonroot
ENTRYPOINT ["/baremetal-operator"]
