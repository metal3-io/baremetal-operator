# Build the manager binary
FROM registry.hub.docker.com/library/golang:1.14 AS builder

WORKDIR /workspace

# Bring in the go dependencies before anything else so we can take
# advantage of caching these layers in future builds.
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o baremetal-operator cmd/manager/main.go

# Copy the controller-manager into a thin image
# BMO has a dependency preventing us to use the static one,
# using the base one instead
FROM gcr.io/distroless/base:latest
WORKDIR /
COPY --from=builder /workspace/baremetal-operator .
USER nonroot:nonroot
ENTRYPOINT ["/baremetal-operator"]

LABEL io.k8s.display-name="Metal3 BareMetal Operator" \
      io.k8s.description="This is the image for the Metal3 BareMetal Operator."
