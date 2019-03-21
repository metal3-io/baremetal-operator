# Running end-to-end-tests

## Setup

The user running the tests must have permission on the cluster to
create CRDs. An example role binding setting for configuring the
"developer" user is provided in test/e2e/role_binding.yaml

```
oc --as system:admin apply -f test/e2e/role_binding.yaml
```

### Run the e2e tests

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
