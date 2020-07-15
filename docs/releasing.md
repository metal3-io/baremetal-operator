# Releasing

## Prerequisites

### `docker`

You must have docker installed.

## Output

### Expected artifacts

1. A container image of the shared baremetal-operator
1. A git tag
1. A release on Github containing:
    - A manifest file - `infrastructure-components.yaml`

### Artifact locations

1. The container image is found in the registry `quay.io/metal3-io` with an image
   name of `baremetal-operator` and a tag that matches the release
   version. The image is automatically built once the release has been created.

## Creating a release for BMO

### Process

For version v0.x.y:

1. Create the release notes `make release-notes`. Copy the output and sort
   manually the items that need to be sorted.
1. Create an annotated tag `git tag -a v0.x.y -m v0.x.y`. To use your GPG
   signature when pushing the tag, use `git tag -s [...]` instead
1. Push the tag to the GitHub repository `git push origin v0.x.y`
   NB: `origin` should be the name of the remote pointing to
   `github.com/metal3-io/baremetal-operator`
1. [Create a release in GitHub](https://help.github.com/en/github/administering-a-repository/creating-releases)
   that contains the elements listed above that have been created in the `out`
   folder
1. Create a branch `release-0.x` for a minor release for backports and bug fixes.

### Permissions

Releasing requires a particular set of permissions.

- Tag push access to the GitHub repository
- GitHub Release creation access

## Impact on Metal3

Multiple additional actions are required in the Metal3 project

### Update the Jenkins jobs

For each minor or major release, two jobs need to be created :

- a master job that runs on a regular basis
- a PR verification job that is triggered by a keyword on a PR targeted for that
  release branch.

### Update Metal3-dev-env

Metal3-dev-env variables need to be modified. After a major or minor release,
the new minor version (that follows CAPI versioning) should point to master for
BMO and the released version should point to the release branch.

### Update the image of BMO in the release branch

If you just created a release branch (i.e. minor version release), you should
modify the image for BMO deployment in this branch to be tagged with the
branch name. The image will then follow the branch.
