Bare Metal Operator
===================

## Setup with minishift

1. Install and launch minishift

   https://docs.okd.io/latest/minishift/getting-started/index.html

2. Ensure you're logged in to the correct context and login as a normal user called developer.

    ```
    oc config use-context minishift
    oc login
    Username: developer
    ```

3. Create a project to host the operator

    ```
    oc new-project bmo-project
    ```

4. Install operator-sdk

    ```
    go get github.com/metalkube/baremetal-operator
    cd ~/go/src/github.com/metalkube/baremetal-operator
    oc --as system:admin apply -f deploy/service_account.yaml
    oc --as system:admin apply -f deploy/role.yaml
    oc --as system:admin apply -f deploy/role_binding.yaml
    oc --as system:admin apply -f deploy/crds/metalkube_v1alpha1_baremetalhost_crd.yaml
    ```

5. Launch the operator locally

    ```
    export OPERATOR_NAME=baremetal-operator
    operator-sdk up local --namespace=bmo-project
    ```

6. Create the CR

    ```
    oc apply -f deploy/crds/metalkube_v1alpha1_baremetalhost_cr.yaml
    ```

## Running end-to-end tests

### Setup

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
