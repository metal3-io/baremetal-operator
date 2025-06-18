# Kustomize NamespaceAnnotator plugin for BMO operator in namespaced mode

This Kustomize plugin provides a container that dynamically assigns namespaces to
Kubernetes roles. It is particularly useful when building a namespace-scoped
operator.

## Usage Instructions

Before running the `make manifests-namespaced` command, you need to
build plugin binary which located hack/tools/kustomize-namespace-annotator
folder. The binary will be executed during the generation process to modify
roles. Note that this step always occurs after any patches applied in the
overlay.

You can identify the tag used for this process from the annotator definition
in `config/overlays/namespaced/roles-ns-annotator.yaml`.

To build NamespaceAnnotator plugin binary, run:

```bash
go build -build -o NamespaceAnnotator main.go
```

Once the binary  is ready, you can execute the command for the namespaced
operator:

```bash
make manifests-namespaced
```
