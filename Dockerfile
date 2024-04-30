# Support FROM override
ARG BUILD_IMAGE=docker.io/golang:1.21.9@sha256:7d0dcbe5807b1ad7272a598fbf9d7af15b5e2bed4fd6c4c2b5b3684df0b317dd
ARG BASE_IMAGE=gcr.io/distroless/static:nonroot@sha256:9ecc53c269509f63c69a266168e4a687c7eb8c0cfd753bd8bfcaa4f58a90876f

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
