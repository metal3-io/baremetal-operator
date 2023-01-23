# Kustomizations for Ironic

This folder contains kustomizations for Ironic.
They are mainly used through the [deploy.sh](../tools/deploy.sh) script, which takes care of generating the necessary config for basic-auth and TLS.

- **base** - This is the kustomize base that we start from.
- **components** - In here you will find re-usable kustomize components for running Ironic with TLS, basic-auth or keepalived. Note that the basic-auth component is missing the actual credentials. This is on purpose, to make sure that the user is setting the password. Similarly the TLS component needs to have the proper IP/SAN set for the certificates.
- **default** - A minimal, fully working, Ironic kustomization including configmap and password. Use only for development! The DB password is hard coded in the repo and there is no TLS or basic-auth.
- **overlays** - Here you will find ready made overlays that use the above mentioned components.
