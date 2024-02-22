# Kustomizations for Baremetal Operator

This folder contains kustomizations for the Baremetal Operator. They have
traditionally been used through the [deploy.sh](../tools/deploy.sh) script,
which takes care of generating the necessary config for basic-auth and TLS.
However, a more GitOps friendly way would be to create your own static overlay.
Check the `overlays/e2e` for an example that is used in the e2e tests.
In the CI system we generate the necessary credentials before starting the test
in `hack/ci-e2e.sh`, and put them directly in the `e2e` overlays.

**NOTE** that you will need to supply the necessary secrets and config! This can
be done in many ways, e.g. through the
[external secrets operator](https://external-secrets.io/latest/) or directly in
your overlay.
In the CI system we generate the necessary credentials before starting the test
in `hack/ci-e2e.sh`, and put them directly in the `e2e` overlays.

- **base** - This is the kustomize base that we start from.
- **components** - In here you will find re-usable kustomize components for TLS
  and basic-auth.
   - **basic-auth** - Enable basic authentication. Note that the basic-auth
      component is missing the actual credentials. This is on purpose, to make
      sure that the user is setting the password instead of using sample
      credentials. The required secret is called `ironic-credentials`.
   - **tls** - Enable TLS. A CA certificate is needed here to verify the
      connection to Ironic. If you deploy BMO together with Ironic in a
      Kubernetes cluster, they can share the secret created for Ironic. The CA
      should be in a secret `ironic-cacert`.
- **default** - A minimal, fully working, BMO kustomization including configmap.
   Use only for development! There is no TLS or basic-auth.
- **overlays** - Here you will find ready made overlays that use the above
   mentioned components. These can be used as examples.
