# Running the tests

## Setup

The user running the tests must have permission on the cluster to
create CRDs. An example role binding setting for configuring the
"developer" user is provided in test/e2e/role_binding.yaml

```bash
oc --as system:admin apply -f test/e2e/role_binding.yaml
```

### Run the unit tests

The tests can be run via `make`

```bash
make test
```

Run linters test before pushing your commit.

```bash
make lint
```

## Using the Hack scripts

The repository contains a ``hack`` directory which has some very useful scripts
for setting up containerized testing environments. It also has Dockerfiles to
allow you to generate images for your own use.

All the scripts accept the ``CONTAINER_RUNTIME`` environment variable with default
of ``podman``. This you can edit to ``docker`` as well.

### Example of running ``golint`` in a Docker container

1. Start by setting the environment variable appropriately

    ```bash
    export CONTAINER_RUNTIME=docker
    ```

2. From the operator parent dir, you can invoke the hack scripts

    ```bash
    ./hack/golint.sh
    ```

    **Note** It's important to be in the operator parent dir because it
    contains the ``Makefile`` that is used in running the tests for the
    operator. Otherwise, you'll see the following error

    ```bash
    sh: 0: Can't open /go/src/github.com/metal3-io/baremetal-operator/hack/golint.sh
    ```

3. Upon successful execution, you should see the following output. I already
    had all the images available from a previous run, you might see the images
    getting downloaded if you're running for the very first time.

    ```bash
    + IS_CONTAINER=false
    + CONTAINER_RUNTIME=docker
    + [ false != false ]
    + docker run --rm --env IS_CONTAINER=TRUE --volume /home/noor/go/src/github.com/metal3-io/baremetal-operator:/go/src/github.com/metal3-io/baremetal-operator:ro,z\
    --entrypoint sh --workdir /go/src/github.com/metal3-io/baremetal-operator\
    quay.io/metal3-io/golint:latest /go/src/github.com/metal3-io/baremetal-operator/hack/golint.sh
    + IS_CONTAINER=TRUE
    + CONTAINER_RUNTIME=podman
    + [ TRUE != false ]
    + export XDG_CACHE_HOME=/tmp/.cache
    + make lint
    which golint 2>&1 >/dev/null || make OPATH/bin/golint
    find ./pkg ./cmd -type f -name \*.go  |grep -v zz_ | xargs -L1 golint -set_exit_status
    ```

## Getting the latest tooling

For testing without the hack scripts, make sure you install the latest
development tools using ``go get``, e-g

```bash
go get golang.org/x/lint/golint
```

This ensures that the linters you have locally and the ones running in the CI
are matched and helps avoid inconsistencies. Happy coding!
