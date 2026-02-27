#!/usr/bin/env bash
#
# Copyright 2023 The Metal3 Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# USAGE:
#
# This script aims to verify a release content per docs/releasing.md
# is all done, all images are built and release in general good to go.
# It can be executed before making a release tag to verify Go dependencies
# and vulnerabilities are already fixed.
#
# Git setup:
# This script expects to be executed in the root directory of BMO
# repository, with the release commit/tag in question checked out.
#
# Command line arguments:
# arg1: mandatory: version without leading v, eg. 0.6.0
#
# Environment variables:
# GITHUB_TOKEN: mandatory: your bearer token that has access to the release
# REMOTE: optional: use this git remote for tag checks: Default: autodetected
# CONTAINER_RUNTIME: optional: container runtime binary. Default: docker

set -eu
# we are using plenty of subshell pipes, and catch errors elsewhere
set +o pipefail

# enable support for **/go.mod, and make it ignore hack/tools/go.mod
shopt -s globstar
GLOBIGNORE=./hack/tools/go.mod

# user input
VERSION="${1:?release version missing, provide without leading v. Example: 0.6.0}"
GITHUB_TOKEN="${GITHUB_TOKEN:?export GITHUB_TOKEN with permissions to read unpublished release notes}"

# if CONTAINER_RUNTIME is set, we will use crane and osv-scanner from images.
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-}"
# correct remote will be autodetected, if empty
REMOTE="${REMOTE:-}"

# this repo
ORG="metal3-io"
PROJECT="${ORG}/baremetal-operator"
REGISTRY="quay.io"

# if the given tag doesn't exist, we run only pre-tag checks
TAG_EXISTS=""
# we skip some checks if we cannot download release information
RELEASE_EXISTS=""


#
# checklist configuration
#

# git tags
declare -a git_annotated_tags=(
    "v${VERSION}"
)

declare -a git_lightweight_tags=(
    "apis/v${VERSION}"
    "pkg/hardwareutils/v${VERSION}"
)

declare -a git_nonexisting_tags=(
    "hack/tools/v${VERSION}"
)

# release notes should have these strings
declare -a release_note_strings=(
    ":recycle:"
    "Changes since v"
)

# required strings that are postfixed with correct release number
declare -a release_note_tag_strings=(
    "The image for this release is: v${VERSION}"
)

# release artifacts
declare -a release_artifacts=(
)

# quay images
declare -a container_images=(
    "${ORG}/baremetal-operator:v${VERSION}"
)

# go mod bump checks - must match up to leading space before v
declare -A module_groups=(
    [capi]="
        sigs.k8s.io/cluster-api
        sigs.k8s.io/cluster-api/test
    "
    [k8s]="
        k8s.io/api
        k8s.io/apiextensions-apiserver
        k8s.io/apimachinery
        k8s.io/apiserver
        k8s.io/client-go
        k8s.io/cluster-bootstrap
        k8s.io/component-base
    "
)

# check these modules are using latest patch releases of their releases
# format: module name=github repo name
declare -A module_releases=(
    [sigs.k8s.io/cluster-api]="kubernetes-sigs/cluster-api"
)

# required tools
declare -a required_tools=(
    awk
    curl
    git
    jq
    sed
)

# we also require a container runtime, or pre-installed binaries
# for osv-scanner we have also version check implemented during tool check
if [[ -n "${CONTAINER_RUNTIME}" ]]; then
    required_tools+=(
        "${CONTAINER_RUNTIME}"
    )
    declare -a GCRANE_CMD=(
        "${CONTAINER_RUNTIME}" run --rm
        --pull always
        gcr.io/go-containerregistry/gcrane:latest
    )
    declare -a OSVSCANNER_CMD=(
        "${CONTAINER_RUNTIME}" run --rm
        -v "${PWD}":"/src:ro,z"
        -w /src
        ghcr.io/google/osv-scanner:v2.3.3@sha256:bf249317dcf838cf9e47f370cfd4dd4178d875bba14e3ce74d299c5bf1b129a1
    )
else
    # go install github.com/google/go-containerregistry/cmd/gcrane@latest
    # go install github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.3.3
    required_tools+=(
        gcrane
        osv-scanner
    )
    declare -a GCRANE_CMD=(gcrane)
    declare -a OSVSCANNER_CMD=(osv-scanner)
fi


#
# temporary files and cleanup trap
#
cleanup()
{
    rm -rf "${TMP_DIR}"
}

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/verify-release-XXXXX")"
RELEASE_JSON="${TMP_DIR}/release.json"
RELEASES_JSON="${TMP_DIR}/releases.json"
SCAN_LOG="${TMP_DIR}/scan.log"
TAG_LOG="${TMP_DIR}/tag.log"
mkdir -p "${TMP_DIR}"
trap cleanup EXIT


#
# pre-requisites
#
_version_check()
{
    # check version of the tool, return failure if smaller
    local min_version version

    min_version="$1"
    version="$2"

    [[ "${min_version}" == $(echo -e "${min_version}\n${version}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]
}

check_tools()
{
    # check that all tools are present, and pass version check too
    # TODO: if more tools need versioning, add the version info directly to the
    # array defining required tools
    local min_version version

    echo "Checking required tools ..."

    for tool in "${required_tools[@]}"; do
        if ! type "${tool}" &>/dev/null; then
            echo "FATAL: need ${tool} to be installed"
            if [[ "${tool}" = "osv-scanner" ]] || [[ "${tool}" = "gcrane" ]]; then
                echo "HINT: 'export CONTAINER_RUNTIME=<docker|podman>' to use containerized tools"
            fi
            exit 1
        fi

        case "${tool}" in
            osv-scanner)
                version=$("${OSVSCANNER_CMD[@]}" -v | grep version | cut -f3 -d" ")
                min_version="2.2.0"
                ;;
            *)
                # dummy values here for other tools
                version="1.0.0"
                min_version="1.0.0"
                ;;
        esac

        # shellcheck disable=SC2310
        if ! _version_check "${min_version}" "${version}"; then
            echo "WARNING: tool ${tool} is version ${version}, should be >= ${min_version}"
        fi
    done

    echo -e "Done\n"
}

detect_remote()
{
    # we support origin (default) and upstrea (if cloned with "gh" CLI tool)
    echo "Detecting remote ..."

    if [[ -z "${REMOTE}" ]]; then
        REMOTE="$(git remote -v | grep "${PROJECT}.* (fetch)" | awk '{print $1;}')"

        if ! [[ "${REMOTE}" =~ ^(origin|upstream)$ ]]; then
            echo "WARNING: detected remote '${REMOTE}' is not supported"
        fi
    else
        echo "INFO: Using supplied remote: ${REMOTE}"
    fi

    echo -e "Done\n"
}

check_input()
{
    echo "Checking input ..."

    # check version is input without leading v, since we have extra annotated
    # tags in history and it needs manually to be edited out
    if [[ "${VERSION}" =~ ^v\d+ ]]; then
        echo "FATAL: given version includes a leading v. Example: 0.6.0"
        exit 1
    fi

    # verify remote exists
    if ! git ls-remote --exit-code "${REMOTE}" &>/dev/null; then
        echo "FATAL: detected remote ${REMOTE} does not exist in repository"
        exit 1
    fi

    echo -e "Done\n"
}

check_tag()
{
    echo "Checking if tag exists ..."

    # is there even a tag
    if git rev-list -n0 "v${VERSION}" &>/dev/null; then
        echo "INFO: Tag v${VERSION} exists, running post-tag checks too"
        TAG_EXISTS="yes"
    else
        echo "INFO: Tag v${VERSION} does not exist, running only pre-tag checks"
    fi

    echo -e "Done\n"
}

check_commit()
{
    # check the tag commit and local commit are the same, and not dirty,
    # so we are verifying the right content
    local local_commit tag_commit repo_status

    echo "Checking local commit vs tag commit ..."

    # verify local HEAD is the same as TAG
    local_commit="$(git rev-list -n1 HEAD)"
    tag_commit="$(git rev-list -n1 "v${VERSION}" || echo)"
    if [[ "${local_commit}" != "${tag_commit}" ]]; then
        echo "WARNING: your local branch content does not match tag v${VERSION} content"
    fi

    repo_status="$(git diff --stat)"
    if [[ -n "${repo_status}" ]]; then
        echo "WARNING: your local repository is dirty"
    fi

    echo -e "Done\n"
}

download_release_information()
{
    # download release information json, requires GITHUB_TOKEN
    echo "Downloading release information ..."
    local release_id

    if ! curl -SsL --fail \
            -H "Accept: application/vnd.github+json" \
            -H "Authorization: Bearer ${GITHUB_TOKEN}" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            -o "${RELEASE_JSON}" \
            "https://api.github.com/repos/${PROJECT}/releases" >/dev/null; then
        echo "ERROR: could not download release information, check token and permissions"
        exit 1
    fi
    release_id=$(jq '.[] | select(.name == "v'"${VERSION}"'") | .id' "${RELEASE_JSON}")

    if [[ -z "${release_id}" ]] || ! curl -SsL --fail \
            -H "Accept: application/vnd.github+json" \
            -H "Authorization: Bearer ${GITHUB_TOKEN}" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            -o "${RELEASE_JSON}" \
            "https://api.github.com/repos/${PROJECT}/releases/${release_id}" >/dev/null; then
        echo "WARNING: could not download release information for tag v${VERSION} (id '${release_id}')"
        echo "WARNING: will skip all release note checks"
    fi
    RELEASE_EXISTS=true

    echo -e "Done\n"
}


#
# verification functions
#
verify_git_tags()
{
    # check tags exist in remote, ie. are not just local but pushed
    echo "Verifying Git tags ..."

    for tag in "${git_annotated_tags[@]}" "${git_lightweight_tags[@]}"; do
        if ! git ls-remote --exit-code --tags "${REMOTE}" "refs/tags/v${VERSION}" &>/dev/null; then
            echo "ERROR: tag ${tag} is not found in remote ${REMOTE}"
        fi
    done

    echo -e "Done\n"
}

verify_git_tag_types()
{
    # check tags are annotated or lightweight as expected
    # and also that no extra tags are pushed by accident
    echo "Verifying Git tag types ..."

    for annotated_tag in "${git_annotated_tags[@]}"; do
        if [[ "$(git cat-file -t "${annotated_tag}" 2>/dev/null)" != "tag" ]]; then
            echo "ERROR: ${annotated_tag} is not an annotated tag, or is missing"
        fi
    done

    for lightweight_tag in "${git_lightweight_tags[@]}"; do
        if [[ "$(git cat-file -t "${lightweight_tag}" 2>/dev/null)" != "commit" ]]; then
            echo "WARNING: ${lightweight_tag} is not a lightweight tag, or is missing"
        fi
    done

    for nonexist_tag in "${git_nonexisting_tags[@]}"; do
        if git cat-file -t "${nonexist_tag}" &>/dev/null; then
            echo "ERROR: ${nonexist_tag} is exists, while it should not"
        fi
    done

    echo -e "Done\n"
}

verify_release_notes()
{
    # check release note content
    echo "Verifying release notes ..."

    # check body if certain strings
    for string in "${release_note_tag_strings[@]}"; do
        # shellcheck disable=SC2076
        if ! [[ "$(jq .body "${RELEASE_JSON}")" =~ "${string}" ]]; then
            echo "ERROR: '${string}' not found in release note text, is tag correct?"
        fi
    done

    # check body for tagged images
    for string in "${release_note_strings[@]}"; do
        # shellcheck disable=SC2076
        if ! [[ "$(jq .body "${RELEASE_JSON}")" =~ "${string}" ]]; then
            echo "WARNING: '${string}' not found in release note text, recheck content"
        fi
    done

    echo -e "Done\n"
}

verify_release_artifacts()
{
    # check that the release json lists all artifacts as present
    echo "Verifying release artifacts ..."

    for artifact in "${release_artifacts[@]}"; do
        # shellcheck disable=SC2076
        if ! [[ "$(jq .assets[].name "${RELEASE_JSON}")" =~ "\"${artifact}\"" ]]; then
            echo "ERROR: release artifact '${artifact}' not found in release"
        fi
    done

    echo -e "Done\n"
}

verify_container_images()
{
    # check quay as built images successfully, and hence tag is present
    # if tag doesn't appear, the build trigger might've been disabled
    local image tag

    echo "Verifying container images are built and tagged ..."

    for image_and_tag in "${container_images[@]}"; do
        image="${image_and_tag/:*}"
        tag="${image_and_tag/*:}"

        # quay paginates 50 items at a time, so it is simpler to use gcrane
        # to list all the tags, than DIY parse the pagination logic
        if ! "${GCRANE_CMD[@]}" ls "${REGISTRY}/${image}" 2>/dev/null > "${TAG_LOG}"; then
            echo "ERROR: cannot list container image tags for ${REGISTRY}/${image}"
            continue
        fi
        if ! grep -E -q "${REGISTRY}/${image}:${tag}$" "${TAG_LOG}"; then
            echo "ERROR: container image tag ${image_and_tag} not found at ${REGISTRY}"
        fi
    done

    echo -e "Done\n"
}

verify_container_base_image()
{
    # check if the golang used for container image build is latest of its minor
    local image tag tag_minor

    echo "Verifying container base images are up to date ..."
    image="docker.io/golang"
    tag="$(make go-version)"
    tag_minor="${tag%.*}"

    # quay paginates 50 items at a time, so it is simpler to use gcrane
    # to list all the tags, than DIY parse the pagination logic
    if ! "${GCRANE_CMD[@]}" ls --platform "linux/amd64" "${image}" 2>/dev/null > "${TAG_LOG}"; then
        echo "ERROR: cannot list container tags for ${image}"
        return 1
    fi
    latest_minor="$(sort -rV < "${TAG_LOG}" | cut -f2 -d: | grep -E "^v?${tag_minor/./\\.}\.[[:digit:]]+$" | head -1)"

    if [[ -z "${latest_minor}" ]]; then
        echo "WARNING: could not find any minor releases of ${image}:${tag}"
    elif [[ "${latest_minor}" != "${tag}" ]]; then
        echo "WARNING: container base image ${image}:${tag} is not the latest minor"
        echo "WARNING: latest minor ${latest_minor} != ${tag}, needs a bump"
    fi

    echo -e "Done\n"
}


#
# helper functions for module related checks
#
_module_direct_dependencies()
{
    # get all required, direct dependencies, exclude hack/tools/go.mod
    sed -n '/^require (/,/^)/{/^require (/!{/^)/!p;};}' ./**/go.mod \
        | grep -v "//\s*indirect" | grep -v "^\s*$" \
        | awk '{print $1, $2;}' | sort | uniq
}

_module_counts_differ()
{
    # return true if module with and without version differ
    # ie. there is mismatch in versions, false otherwise
    local module="$1"
    local version="$2"

    # shellcheck disable=SC2126
    mod_count="$(grep "\b${module} v" ./**/go.mod | grep -v "//\s*indirect" | wc -l)"
    # shellcheck disable=SC2126
    ver_count="$(grep "\b${module} ${version}" ./**/go.mod | grep -v "//\s*indirect" | wc -l)"

    [[ "${mod_count}" -ne "${ver_count}" ]]
}

_module_get_version()
{
    # get a version of given module, pick first match
    local module="$1"

    grep -h "\b${module}\b" ./**/go.mod \
        | grep -v "//\s*indirect" | head -1 | awk '{print $2;}'
}

_module_get_latest_patch_release()
{
    # get latest patch release from given version
    # module needs to contain full module url
    # version is minor release prefix, like v1.4.
    local repo="$1"
    local version="$2"

    if ! curl -SsL --fail \
            -H "Accept: application/vnd.github+json" \
            -H "Authorization: Bearer ${GITHUB_TOKEN}" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            -o "${RELEASES_JSON}" \
            "https://api.github.com/repos/${repo}/releases" >/dev/null; then
        echo ""
    else
        # do simple filtering,
        jq ".[].name" "${RELEASES_JSON}" | tr -d '"' \
            | grep "^${version}" | grep -v -- "-(rc|alpha|beta)" | head -1
    fi
}


#
# pre-tag checks
#
verify_module_versions()
{
    # verify all dependencies are using the same version across all go.mod
    # in the repository. Ignore indirect ones.
    echo "Verify all go.mod direct dependencies are the same across go.mods ..."

    # shellcheck disable=SC2119
    _module_direct_dependencies | while read -r module version; do
        if [[ -z "${module}" ]] || [[ -z "${version}" ]]; then
            echo "WARNING: malformatted line found: module=${module} version=${version} ... skipping"
            continue
        fi

        # shellcheck disable=SC2310
        if _module_counts_differ "${module}" "${version}"; then
            echo "ERROR: module ${module} has version mismatch!"
            grep "\b${module} v" ./**/go.mod | grep -v "//\s*indirect"
            echo
        fi
    done


    echo -e "Done\n"
}

verify_module_group_versions()
{
    # verify certain important go.mod modules are correctly bumped
    # this checks all the modules are the same version per group
    local ver mod mod_count ver_count

    echo "Verifying go.mod bump module pairings ..."

    for name in "${!module_groups[@]}"; do
        mod=""
        ver=""

        for module in ${module_groups[${name}]}; do
            # all versions of modules in the array must be the same, so get
            # first one, and then verify they are all the same
            if [[ -z "${ver}" ]]; then
                # shellcheck disable=SC2311
                ver="$(_module_get_version "${module}")"
                mod="${module}"
            fi

            # shellcheck disable=SC2310
            if _module_counts_differ "${module}" "${ver}"; then
                echo "ERROR: module ${module} has version mismatch!"
                # print the mismatches
                grep -E "\b(${mod}|${module}) v" ./**/go.mod \
                    | grep -v "//\s*indirect" | sort | uniq
                echo
            fi
        done
    done

    echo -e "Done\n"
}

verify_module_releases()
{
    # verify certain modules are using latest patch versions of their respecive
    # releases, so we have remembered to bump them
    echo "Verify modules are using latest patch releases ..."

    for module in "${!module_releases[@]}"; do
        repo="${module_releases[${module}]}"
        # shellcheck disable=SC2311
        version="$(_module_get_version "${module}")"
        # shellcheck disable=SC2311
        latest="$(_module_get_latest_patch_release "${repo}" "${version:0:5}")"

        if [[ -z "${latest}" ]]; then
            echo "ERROR: failed to read release information for ${module} from ${repo}"
        elif [[ "${version}" != "${latest}" ]]; then
            echo "WARNING: module ${module} ${version} is not latest release ${latest}"
        fi
    done

    echo -e "Done\n"
}

verify_vulnerabilities()
{
    # run osv-scanner to verify if we have open vulnerabilities in deps
    local go_version config_file=".osv-scanner.toml"

    echo "Verifying vulnerabilities ..."

    go_version="$(make go-version)"
    echo "GoVersionOverride = \"${go_version}\"" > "${config_file}"
    "${OSVSCANNER_CMD[@]}" scan \
        --recursive \
        --config="${config_file}" \
        ./ > "${SCAN_LOG}" || true

    if ! grep -q "No vulnerabilities found" "${SCAN_LOG}"; then
        cat "${SCAN_LOG}"
    fi
    rm -f "${config_file}"

    echo -e "Done\n"
}


#
# check inputs and setup, then run verifications
#
check_tools
detect_remote
check_input
check_tag

# post-tag verifications
if [[ -n "${TAG_EXISTS}" ]]; then
    check_commit
    download_release_information
    verify_git_tags
    verify_git_tag_types
    if [[ -n "${RELEASE_EXISTS}" ]]; then
        verify_release_notes
        verify_release_artifacts
    fi
    verify_container_images
fi

# always verified
verify_container_base_image
verify_module_versions
verify_module_group_versions
verify_module_releases
verify_vulnerabilities
