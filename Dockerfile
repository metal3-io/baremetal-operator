# Build the manager binary
FROM golang:1.14 as builder

WORKDIR /workspace

# Copy the go source
COPY . .

# Copy the Go Modules manifests
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o baremetal-operator main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/baremetal-operator .
USER nonroot:nonroot

ENTRYPOINT ["/baremetal-operator"]
