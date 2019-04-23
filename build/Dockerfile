FROM registry.svc.ci.openshift.org/openshift/release:golang-1.10 AS builder
WORKDIR /go/src/github.com/metal3-io/baremetal-operator
COPY . .
RUN go build -o build/_output/bin/baremetal-operator cmd/manager/main.go

FROM quay.io/metalkube/base-image

COPY --from=builder /go/src/github.com/metal3-io/baremetal-operator/build/_output/bin/baremetal-operator /

RUN if ! rpm -q genisoimage; \
    then yum install -y genisoimage && \
    yum clean all && \
    rm -rf /var/cache/yum/*; \
    fi

LABEL io.k8s.display-name="MetalKube BareMetal Operator" \
      io.k8s.description="This is the image for the MetalKube BareMetal Operator."
