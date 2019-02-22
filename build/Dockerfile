FROM registry.svc.ci.openshift.org/openshift/release:golang-1.10 AS builder
WORKDIR /go/src/github.com/metalkube/baremetal-operator
COPY . .
RUN go build -o build/_output/bin/baremetal-operator cmd/manager/main.go

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base
COPY --from=builder /go/src/github.com/metalkube/baremetal-operator/build/_output/bin/baremetal-operator /
