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

## Testing

Refer to [testing](docs/testing.md) for details about running the tests.

## API

Refer to [the API documentation](docs/api.md) for details about the
custom resources defined by this operator.
