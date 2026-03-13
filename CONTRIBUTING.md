# How to Contribute to Baremetal Operator

> **Note**: Please read the [common Metal3 contributing guidelines](https://github.com/metal3-io/community/blob/main/CONTRIBUTING.md)
> first. This document contains baremetal-operator-specific information.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Versioning](#versioning)
   - [Codebase](#codebase)
- [Branches](#branches)
   - [CI Support and Branch Maintenance](#ci-support-and-branch-maintenance)
- [Backporting Policy](#backporting-policy)
- [Release Process](#release-process)
   - [Exact Steps](#exact-steps)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Versioning

See the [common versioning and release semantics](https://github.com/metal3-io/community/blob/main/CONTRIBUTING.md#versioning-and-release-semantics)
in the Metal3 community contributing guide.

**Note**: The test module and experiments do not provide any backward
compatible guarantees.

### Codebase

Baremetal Operator doesn't follow the release cadence of upstream Kubernetes.
The versioning semantics follow the common Metal3 guidelines above.

## Branches

See the [common branch structure guidelines](https://github.com/metal3-io/community/blob/main/CONTRIBUTING.md#branches)
in the Metal3 community contributing guide.

### CI Support and Branch Maintenance

Baremetal Operator maintains the most recent release/releases for all supported
APIs and contract versions. Support for this section refers to CI support and
the ability to backport and release patch versions;
[backport policy](#backporting-policy) is defined below.

- The API version is determined from the GroupVersion defined in the top-level
  `apis/` package.

- The EOL date of each API Version is determined from the last release available
  once a new API version is published.

<!-- markdownlint-disable MD013 -->

| API Version  | Maintained Until                                              |
| ------------ | ------------------------------------------------------------- |
| **v1alpha1** | TBD (current latest)                                          |
| **v1beta1**  | Upcoming (Proposal [PR](https://github.com/metal3-io/metal3-docs/pull/332)) |

<!-- markdownlint-enable MD013 -->

- For the current stable API version (v1alpha1) we support the two most recent
  minor releases; older minor releases are immediately unsupported when a new
  major/minor release is available.

- Once we have v1beta1 API, we will determine support policies for older API
  versions.

- We will maintain test coverage for all supported minor releases and for one
  additional release for the current stable API version in case we have to do an
  emergency patch release. For example, if v0.4 and v0.3 are currently
  supported, we will also maintain test coverage for v0.2 for one additional
  release cycle. When v0.5 is released, tests for v0.2 will be removed.

| Minor Release | API Version  | Maintained Until                              |
| ------------- | ------------ | --------------------------------------------- |
| v0.10.x       | **v1alpha1** | when v0.12.0 will be released                 |
| v0.9.x        | **v1alpha1** | when v0.11.0 will be released                 |
| v0.8.x        | **v1alpha1** | EOL since 2025-04-29                          |
| v0.7.x        | **v1alpha1** | EOL since 2024-12-19                          |
| v0.6.x        | **v1alpha1** | EOL since 2024-09-03                          |
| v0.5.x        | **v1alpha1** | EOL since 2024-09-03                          |
| v0.4.x        | **v1alpha1** | EOL since 2024-04-17                          |
| v0.3.x        | **v1alpha1** | EOL since 2024-01-08                          |
| v0.2.x        | **v1alpha1** | EOL since 2023-08-30 (*)                      |
| v0.1.x        | **v1alpha1** | EOL since 2023-04-26 (*)                      |

(*) Previous support policy applies, older minor releases were immediately
unsupported when a new major/minor release was available

- Exceptions can be filed with maintainers and taken into consideration on a
  case-by-case basis.

## Backporting Policy

See the [common backporting guidelines](https://github.com/metal3-io/community/blob/main/CONTRIBUTING.md#backporting)
in the Metal3 community contributing guide.

Additionally, for baremetal-operator:

- We generally do not accept backports to BMO release branches that are EOL.
  Check the [Version support](https://github.com/metal3-io/metal3-docs/blob/main/docs/user-guide/src/version_support.md#baremetal-operator)
  guide for reference.

## Release Process

See the [common release process guidelines](https://github.com/metal3-io/community/blob/main/CONTRIBUTING.md#release-process)
in the Metal3 community contributing guide.

### Exact Steps

Refer to the [releasing document](./docs/releasing.md) for the exact steps.
