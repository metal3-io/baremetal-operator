# Out of tree Starlark provisioner plugin image.
# Builds a pinned BMO, builds the plugin against that source, layers it in.

ARG BUILD_IMAGE=docker.io/golang:1.26.5@sha256:3aff6657219a4d9c14e27fb1d8976c49c29fddb70ba835014f477e1c70636647
ARG BASE_IMAGE=gcr.io/distroless/base-debian13:nonroot@sha256:a557d784ac275c287d2bdf3172f47bece8d2a0ef3c0fdefb712e95084a04a562
ARG BMO_VERSION=v0.14.0-beta.0
ARG BMO_REPO=https://github.com/metal3-io/baremetal-operator.git

# Fetch the pinned BMO source once. Both the operator build and the plugin build
# derive from this so they resolve the identical module graph.
FROM $BUILD_IMAGE AS bmo-src
ARG BMO_VERSION
ARG BMO_REPO
RUN git clone --depth 1 --branch ${BMO_VERSION} ${BMO_REPO} /bmo 2>/dev/null \
 || (git clone ${BMO_REPO} /bmo && git -C /bmo checkout ${BMO_VERSION})

# Build the stock operator binary and the plugin load tester from the pin.
FROM bmo-src AS bmo
ARG ARCH=amd64
ARG LDFLAGS=-s -w
ENV CGO_ENABLED=1
ENV GO111MODULE=on
WORKDIR /bmo
RUN GOOS=linux GOARCH=${ARCH} go build -a -ldflags "${LDFLAGS}" -o /out/baremetal-operator main.go \
 && GOOS=linux GOARCH=${ARCH} go build -a -ldflags "${LDFLAGS}" -o /out/plugin-load-test ./cmd/plugin-load-test

# Build the plugin against the pinned BMO source. The replaces make it inherit
# BMO's exact module resolution and add only the Starlark dependencies.
FROM bmo-src AS plugin
ARG ARCH=amd64
ARG LDFLAGS=-s -w
ARG BMO_VERSION
ARG BMO_REPO
ENV CGO_ENABLED=1
ENV GO111MODULE=on
WORKDIR /plugin
COPY . .
RUN bash ./hack/retarget-bmo.sh "${BMO_VERSION}" "${BMO_REPO}" /bmo \
 && GOOS=linux GOARCH=${ARCH} \
    go build -buildmode=plugin -ldflags "${LDFLAGS}" -o /out/starlark-provisioner.so ./plugin

# Fail the build unless the .so loads into the pinned operator. This is the
# contract and toolchain gate. The final image copies from here so it always runs.
FROM bmo AS verify
COPY --from=plugin /out/starlark-provisioner.so /plugins/starlark-provisioner.so
COPY --from=plugin /plugin/scripts/efi-vmedia.star /scripts/efi-vmedia.star
RUN STARLARK_PROVISIONER_SCRIPT=/scripts/efi-vmedia.star \
    /out/plugin-load-test /plugins/starlark-provisioner.so starlark

# Runtime image. Stock BMO at the pin plus the layered Starlark plugin. Uses
# distroless/base (not static) because Go plugins need glibc.
FROM $BASE_IMAGE
WORKDIR /
COPY --from=bmo /out/baremetal-operator .
COPY --from=verify /plugins/starlark-provisioner.so /plugins/starlark-provisioner.so
COPY --from=plugin /plugin/scripts/efi-vmedia.star /scripts/efi-vmedia.star
ENV STARLARK_PROVISIONER_SCRIPT=/scripts/efi-vmedia.star
USER nonroot:nonroot
ENTRYPOINT ["/baremetal-operator"]
CMD ["--provisioner", "starlark"]
