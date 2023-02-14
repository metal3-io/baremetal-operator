# Support FROM override
ARG BUILD_IMAGE=docker.io/golang:1.19.5@sha256:572f68065ea605e0bd7ab42aa036462318e680a15db0f41a0cadcd06affdabdb
ARG BASE_IMAGE=gcr.io/distroless/base:latest

# Build the manager binary
FROM $BUILD_IMAGE AS builder

WORKDIR /workspace

# Bring in the go dependencies before anything else so we can take
# advantage of caching these layers in future builds.
COPY go.mod go.mod
COPY go.sum go.sum
COPY apis/go.mod apis/go.mod
COPY apis/go.sum apis/go.sum
COPY hack/tools/go.mod hack/tools/go.mod
COPY hack/tools/go.sum hack/tools/go.sum
COPY pkg/hardwareutils/go.mod pkg/hardwareutils/go.mod
COPY pkg/hardwareutils/go.sum pkg/hardwareutils/go.sum
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GO111MODULE=on go build -a -o baremetal-operator main.go

# Copy the controller-manager into a thin image
# BMO has a dependency preventing us to use the static one,
# using the base one instead
FROM $BASE_IMAGE
WORKDIR /
COPY --from=builder /workspace/baremetal-operator .
USER nonroot:nonroot
ENTRYPOINT ["/baremetal-operator"]

LABEL io.k8s.display-name="Metal3 BareMetal Operator" \
      io.k8s.description="This is the image for the Metal3 BareMetal Operator."
