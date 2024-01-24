# How to Contribute

Metal3 projects are [Apache 2.0 licensed](LICENSE) and accept contributions via
GitHub pull requests.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Certificate of Origin](#certificate-of-origin)
   - [Git commit Sign-off](#git-commit-sign-off)
- [Finding Things That Need Help](#finding-things-that-need-help)
- [Versioning](#versioning)
   - [Codebase](#codebase)
      - [Backporting](#backporting)
- [Branches](#branches)
   - [CI Support and Branch Maintenance](#ci-support-and-branch-maintenance)
- [Contributing a Patch](#contributing-a-patch)
- [Backporting a Patch](#backporting-a-patch)
- [Breaking Changes](#breaking-changes)
   - [Merge Approval](#merge-approval)
   - [Google Doc Viewing Permissions](#google-doc-viewing-permissions)
   - [Issue and Pull Request Management](#issue-and-pull-request-management)
   - [Commands and Workflow](#commands-and-workflow)
- [Release Process](#release-process)
   - [Exact Steps](#exact-steps)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Certificate of Origin

By contributing to this project you agree to the Developer Certificate of
Origin (DCO). This document was created by the Linux Kernel community and is a
simple statement that you, as a contributor, have the legal right to make the
contribution. See the [DCO](DCO) file for details.

### Git commit Sign-off

Commit message should contain signed off section with full name and email. For example:

 ```text
  Signed-off-by: John Doe <jdoe@example.com>
 ```

When making commits, include the `-s` flag and `Signed-off-by` section
will be automatically added to your commit message. If you want GPG
signing too, add the `-S` flag alongside `-s`.

```bash
  # Signing off commit
  git commit -s

  # Signing off commit and also additional signing with GPG
  git commit -s -S
```

## Finding Things That Need Help

If you're new to the project and want to help, but don't know where to start, we
have a semi-curated list of issues that
should not need deep knowledge of the system. [Have a look and see if anything
sounds interesting](https://github.com/metal3-io/baremetal-operator/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22).
Alternatively, read some of the docs on other controllers and try to write your
own, file and fix any/all issues that come up, including gaps in documentation!

## Versioning

### Codebase

Baremetal Operator doesn't follow release cadence and versioning of upstream
Kubernetes semantic versioning. This codebase guarantees the following:

- A (*minor*) release CAN include:
   - Introduction of new API versions, or new Kinds.
   - Compatible API changes like field additions, deprecation notices, etc.
   - Breaking API changes for deprecated APIs, fields, or code.
   - Features, promotion or removal of feature gates.
   - And more!

- A (*patch*) release SHOULD only include backwards compatible set of bugfixes.

These guarantees extend to all code exposed in our Go Module, including
*types from dependencies in public APIs*.
Types and functions not in public APIs are not considered part of the guarantee.

#### Backporting

We only accept backports of critical bugs, security issues, or bugs without
easy workarounds, any
backport MUST not be breaking for either API or behavioral changes.
We generally do not accept PRs against older release branches.

## Branches

The *main* branch is where development happens. All the latest and
greatest code, including breaking changes, happens on main.

The *release-X.Y* branches will contain stable, backwards compatible code. On
every major or minor release, a new branch will be created. It is from these
branches that minor and patch releases will be tagged. In some cases, it may
be necessary to open PRs for bugfixes directly against stable branches, but
this should generally not be the case.

### CI Support and Branch Maintenance

Baremetal-operator maintains the most recent release/releases for all supported
APIs and contract versions. Support for this section refers to CI support and
the ability to backport and release patch versions;
[backport policy](#backporting) is defined above.

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

**Note**: Currently, BMO release branches are tested with CAPM3 integration/e2e
   tests, so dropping a test for BMO release version is dependant on CAPM3
   release. As such, we might test a BMO release until we drop the test for
   CAPM3. Once BMO e2e tests take over, we can follow the above policy for
   dropping tests.

| Minor Release | API Version  | Maintained Until                              |
| ------------- | ------------ | --------------------------------------------- |
| v0.4.x        | **v1alpha1** | when v0.6.0 will be released                  |
| v0.3.x        | **v1alpha1** | when v0.5.0 will be released                  |
| v0.2.x        | **v1alpha1** | EOL since 2023-08-30 (*)                      |
| v0.1.x        | **v1alpha1** | EOL since 2023-04-26 (*)                      |

(*) Previous support policy applies, older minor releases were immediately
unsupported when a new major/minor release was available

- Exceptions can be filed with maintainers and taken into consideration on a
  case-by-case basis.

## Contributing a Patch

1. If you haven't already done so, sign a Contributor License Agreement (see
   details [here](https://github.com/kubernetes/community/blob/master/CLA.md)).
1. Fork the desired repo, develop and test your code changes.
1. Submit a pull request.

All code PR should be labeled with one of

- ⚠️ (`:warning:`, major or breaking changes)
- ✨ (`:sparkles:`, feature additions)
- 🐛 (`:bug:`, patch and bugfixes)
- 📖 (`:book:`, documentation or proposals)
- 🌱 (`:seedling:`, minor or other)

Individual commits should not be tagged separately, but will generally be
assumed to match the PR. For instance, if you have a bugfix in with
a breaking change, it's generally encouraged to submit the bugfix
separately, but if you must put them in one PR, mark the commit
separately.

All changes must be code reviewed. Coding conventions and standards are
explained in the official [developer
docs](https://github.com/kubernetes/community/tree/master/contributors/devel).
Expect reviewers to request that you
avoid common [go style
mistakes](https://github.com/golang/go/wiki/CodeReviewComments) in your PRs.

## Backporting a Patch

Baremetal Operator will maintain older versions through `release-X.Y` branches.
We accept backports of bug fixes to the most recent
release branch. For example, if the most recent branch is `release-0.3`, and the
`main` branch is under active
development for v0.4.0, a bug fix that merged to `main` that also affects
`v0.3.x` may be considered for backporting
to `release-0.3`. We generally do not accept PRs against older release branches.

## Breaking Changes

Breaking changes are generally allowed in the `main` branch.

There may be times, however, when `main` is closed for breaking changes. This
is likely to happen as we are close to release a new minor version.

Breaking changes are not allowed in release branches, as these represent minor
versions that have already been released.
These versions have consumers who expect the APIs, behaviors, etc. to remain
stable during the life time of the patch stream for the minor release.

Examples of breaking changes include:

- Removing or renaming a field in a CRD
- Removing or renaming a CRD
- Removing or renaming an exported constant, variable, type, or function
- Updating the version of critical libraries such as controller-runtime,
  client-go, apimachinery, etc. (patch versions of these libraries are not
  considered as breaking change)
- Some version updates may be acceptable, for picking up bug fixes, but
  maintainers must exercise caution when reviewing.

There is possibility to have exceptions where breaking changes are allowed in
release branches. These are at the discretion of the project's maintainers, and
must be carefully considered before merging. An example of an allowed
breaking change might be a fix for a behavioral bug that was released in an
initial minor version (such as `v0.3.0`).

### Merge Approval

Please see the [Kubernetes community document on pull
requests](https://git.k8s.io/community/contributors/guide/pull-requests.md) for
more information about the merge process.

### Google Doc Viewing Permissions

To gain viewing permissions to google docs in this project, please join the
[metal3-dev](https://groups.google.com/forum/#!forum/metal3-dev) google
group.

### Issue and Pull Request Management

Anyone may comment on issues and submit reviews for pull requests. However, in
order to be assigned an issue or pull request, you must be a member of the
[Metal3-io organization](https://github.com/metal3-io) GitHub organization.

Metal3 maintainers can assign you an issue or pull request by leaving a
`/assign <your Github ID>` comment on the issue or pull request.

### Commands and Workflow

Baremetal Operator follows the standard Kubernetes workflow: any PR
needs `lgtm` and `approved` labels, and PRs must pass the tests before being
merged. See
[the contributor docs](https://github.com/kubernetes/community/blob/master/contributors/guide/pull-requests.md#the-testing-and-merge-workflow)
for more info.

We use the same priority and kind labels as Kubernetes. See the labels
tab in GitHub for the full list.

The standard Kubernetes comment commands should work in
Baremetal Operator. See [Prow](https://prow.apps.test.metal3.io/command-help)
for a command reference.

## Release Process

Minor and patch releases are generally done immediately after a feature or
bugfix is landed, or sometimes a series of features tied together.

Minor releases will only be tagged on the *most recent* major release
branch, except in exceptional circumstances. Patches will be backported
to maintained stable versions, as needed.

Major releases will be done shortly after a breaking change is merged -- once
a breaking change is merged, the next release *must* be a major revision.
We don't intend to have a lot of these, so we may put off merging breaking
PRs until a later date.

### Exact Steps

Refer to the [releasing document](./docs/releasing.md) for the exact steps.
