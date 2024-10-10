# Kustomize Namespace Annotator for Roles and Rolebindings

This tool provides a container that dynamically assigns namespaces to
Kubernetes roles. It is particularly useful when building a namespace-scoped
operator.

## Usage Instructions

Before running the `make manifests-generate-namespaced` command, you need to
build the Docker container. The container will be executed during the
generation process to modify roles. Note that this step always occurs after
any patches applied in the overlay.

You can identify the tag used for this process from the annotator definition
in `config/overlays/namespaced/roles-ns-annotator.yaml`.

To build the Docker container, run:

```bash
docker build -t bmo/roleannotator:1.0.0 .
```

Once the container is built, you can execute the command for the namespaced
operator:

```bash
make manifests-namespaced
```
