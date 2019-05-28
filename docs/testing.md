# Running end-to-end-tests

## Setup

The user running the tests must have permission on the cluster to
create CRDs. An example role binding setting for configuring the
"developer" user is provided in test/e2e/role_binding.yaml

```
oc --as system:admin apply -f test/e2e/role_binding.yaml
```

### Run the e2e tests

First, create the namespace to be used for the test:

```
$ kubectl create namespace operator-test
```

Next, delete hosts created by metal3-dev-env and set environment
variables with the URL and credentials for its master node:

```
$ kubectl delete -n metal3 -f ../metal3-dev-env/bmhosts_crs.yaml
$ export TEST_HOST_URL=$(yq -r '. |
    select(.metadata.name == "master-0") |
   .spec.bmc.address' \
    ../metal3-dev-env/bmhosts_crs.yaml)
$ export TEST_HOST_CREDS=$(yq  -r '. |
    select(.metadata.name == "master-0-bmc-secret") |
    .data.username + ":" + .data.password' \
    ../metal3-dev-env/bmhosts_crs.yaml)
```

Run the tests using the operator-sdk command line tool

```
operator-sdk test local ./test/e2e --namespace operator-test --up-local --debug
```

If the setup steps above have already been run, causing "X already
exists" errors, use the --no-setup option to skip that step in the test.

```
operator-sdk test local ./test/e2e --namespace operator-test --up-local --debug --no-setup
```

The tests can also be run via `make`

```
make test
```
