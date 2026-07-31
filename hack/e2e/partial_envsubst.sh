#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Description: Call signature:
#
#					partial_envsubst <target file> <var1> <var2>, ...
#
#			   partial_envsubst checks if target file exists and if it does, it
#			   will use that as the template. If not, it looks for a template
#			   "<target file>.tmpl" for the target file, if no template is
#			   found, it returns -1. If template can be found, it will use the
#			   template and inject the specified variables var1, var2, ... into
#			   the template.
# Usage:       Source and call the function
# -----------------------------------------------------------------------------

set -eu

partial_envsubst() {
	local target_file="$1"
	shift
	local subst_vars="$*"
	local template
	if [[ -e "${target_file}" ]]; then
		template="/tmp/partial_envsubst_template"
		mv -f "${target_file}" "${template}"
	elif [[ -e "${target_file}.tmpl" ]]; then
		template="${target_file}.tmpl"
	else
		echo "No template can be found for the target file ${target_file}"
		return 1
	fi
	envsubst "${subst_vars}" < "${template}" > "${target_file}"
	echo "Injected variables into file ${target_file}"
}
