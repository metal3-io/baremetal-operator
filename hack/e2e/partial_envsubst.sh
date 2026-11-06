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
	# Use target directory as the location for tmp_dir, so that we remain in the
	# same filesystem and can atomically move the output.
	local tmp_dir
	tmp_dir=$(mktemp -d --tmpdir="$(dirname "${target_file}")")
	local template="${tmp_dir}/template"
	if [[ -e "${target_file}" ]]; then
		cp "${target_file}" "${template}"
	elif [[ -e "${target_file}.tmpl" ]]; then
		cp "${target_file}.tmpl" "${template}"
	else
		echo "No template can be found for the target file ${target_file}"
		return 1
	fi

	envsubst "${subst_vars}" < "${template}" > "${tmp_dir}/output"
	# Moving is atomic in the same filesystem.
	mv -f "${tmp_dir}/output" "${target_file}"
	echo "Injected variables into file ${target_file}"
	rm -rf "${tmp_dir}"
}
