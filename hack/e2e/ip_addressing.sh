#!/usr/bin/env bash

HELP_MSG="""
Description:

IPv4 and IPv6 specific setups including IP addresses. This file collects all
IP-addresses into one place and offers functionality to inject the addresses
into config files and cleaning the created config files.

Usage:

	Sourcing the file ONLY exports the IP variables
	. $(basename "${0}") 

	Using functionality through arguments:
	$(basename "${0}") [opts]

Options:

	-i, --inject	Inject IP-addresses to templates
	-c, --clean		Delete templated configuration files
	-h, --help		Show this help message
	<no argument>	Export variables, do nothing else.
"""

set -eu

REPO_ROOT="$(realpath "$(dirname "${BASH_SOURCE[0]}")/../..")"
. "${REPO_ROOT}/hack/e2e/partial_envsubst.sh"

if [[ "${USE_IPV6:-false}" = true ]]; then
	export IP_ADDRESS="fd55::1"
	export HOST_ADDRESS="[${IP_ADDRESS}]"
	export SUBNET_MASK="64"
	# IP address where Ironic is available
	export IRONIC_PROVISIONING_IP="fd55::2"
	export IRONIC_PROVISIONING_HOST="[${IRONIC_PROVISIONING_IP}]"
	export UPGRADE_IRONIC_PROVISIONING_IP="fd55::3"
	# Values for IrSO Ironic
	export IRONIC_DHCP_RANGE_BEGIN="fd55::100"
	export IRONIC_DHCP_RANGE_END="fd55::164" # allocate 100 addresses
	export IRONIC_NETWORK_CIDR="fd55::/64"
	# IP addresses for the VMs
	export VM_HOST1_IP="fd55::122"
	export VM_HOST2_IP="fd55::123"
	export VM_HOST1_HOST="[${VM_HOST1_IP}]"
	export VM_HOST2_HOST="[${VM_HOST2_IP}]"
	export IP_FAMILY="ipv6"

	# NOTE(nuhakala): This disables the e2e tests from SSH'ing into provisioned
	# nodes to check if they are running. We don't want to use this with IPv6
	# because dnsmasq identifies v6 clients using client ID (CID) instead of
	# MAC address like in v4. Using CID means that we cannot assign static IPv6
	# address with dnsmasq. To use static v6 address, we would need to use
	# pre-provisioning networkdata, but we don't want to do that because it has
	# its own test.
	export SSH_CHECK_PROVISIONED="false"

	# In IPv6 case we want to skip metrics test because DNS is not working with
	# IPv6 only clusters: https://github.com/kubernetes-sigs/kind/issues/4152
	export GINKGO_SKIP="Metrics ${GINKGO_SKIP:-}"
else
	# Sushy-tools / image server endpoint, not ironic
	export IP_ADDRESS="192.168.222.1"
	export HOST_ADDRESS="${IP_ADDRESS}"
	export SUBNET_MASK="24"
	# IP address where Ironic is available
	export IRONIC_PROVISIONING_IP="192.168.222.2"
	export IRONIC_PROVISIONING_HOST="${IRONIC_PROVISIONING_IP}"
	export UPGRADE_IRONIC_PROVISIONING_IP="192.168.222.3"
	# Values for IrSO Ironic
	export IRONIC_DHCP_RANGE_BEGIN="192.168.222.100"
	export IRONIC_DHCP_RANGE_END="192.168.222.200"
	export IRONIC_NETWORK_CIDR="192.168.222.0/24"
	# IP addresses for the VMs
	export VM_HOST1_IP="192.168.222.122"
	export VM_HOST2_IP="192.168.222.123"
	export VM_HOST1_HOST="${VM_HOST1_IP}"
	export VM_HOST2_HOST="${VM_HOST2_IP}"
	export IP_FAMILY="ipv4"
	export SSH_CHECK_PROVISIONED="true"
fi


_inject_templates() {
	# shellcheck disable=SC2016
	SUBST_VARS='${IP_ADDRESS},${HOST_ADDRESS},${SUBNET_MASK},${IRONIC_PROVISIONING_IP},${IRONIC_PROVISIONING_HOST},'
	# shellcheck disable=SC2016
	SUBST_VARS+='${UPGRADE_IRONIC_PROVISIONING_IP},${IRONIC_DHCP_RANGE_BEGIN},${IRONIC_DHCP_RANGE_END},${IRONIC_NETWORK_CIDR},'
	# shellcheck disable=SC2016
	SUBST_VARS+='${VM_HOST1_IP},${VM_HOST2_IP},${VM_HOST1_HOST},${VM_HOST2_HOST},${IP_FAMILY},${SSH_CHECK_PROVISIONED}'

	for tmpl in "${templates[@]}"; do
		partial_envsubst "${REPO_ROOT}/${tmpl}" "${SUBST_VARS}"
	done
}

_delete_templates() {
	for tmpl in "${templates[@]}"; do
		rm "${tmpl}" || true
	done
	echo "Templated config files deleted."
}

_main() {
	# NOTE(nuhakala): Create templates. Fixture tests don't execute ci-e2e.sh,
	# hence we want to do templating here, because this file can be executed with
	# fixture tests without running rest of the ci-e2e.sh setup.
	local templates=(
		test/e2e/config/ironic.yaml
		test/e2e/config/fixture.yaml
		test/e2e/data/ironic-standalone-operator/components/tls/certificate.yaml
		test/e2e/data/ironic-standalone-operator/operator/irso-v0.10/kustomization.yaml
		test/e2e/data/ironic-standalone-operator/operator/irso-v0.11/kustomization.yaml
		test/e2e/data/ironic-standalone-operator/ironic/base/ironic.yaml
		test/e2e/config/bmcs-fixture.yaml
		test/e2e/config/bmcs-ipmi.yaml
		test/e2e/config/bmcs-redfish-virtualmedia.yaml
		test/e2e/config/bmcs-redfish.yaml
		test/e2e/config/vbmctl.yaml
		config/overlays/e2e/ironic.env
	)
	OPTS=$(getopt \
		-o cih \
		--long clean,inject,help -- "$@")
	eval set -- "${OPTS}"
	while true; do
		case "$1" in
			-c|--clean)
				_delete_templates
				shift ;;
			-i|--inject)
				_inject_templates
				shift ;;
			-h|--help)
				echo -e "${HELP_MSG}"; exit 0 ;;
			*) break ;;
		esac
	done
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    # Script is being executed, execute main.
	_main "$@"
fi
