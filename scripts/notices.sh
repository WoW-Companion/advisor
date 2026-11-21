#!/usr/bin/env bash
# Writes the third-party notices for the Go modules linked into wowc-advisor to stdout:
# each module, its version and license, then the license text shipped with that module.
set -euo pipefail
cd "$(dirname "$0")/.."

license_file() {
	find "$1" -maxdepth 1 -type f \( -name 'LICENSE*' -o -name 'COPYING*' \) | sort | sed -n 1p
}

license_id() {
	local f=$1
	if grep -q "Apache License" "$f"; then echo "Apache-2.0"
	elif grep -q "Mozilla Public License" "$f"; then echo "MPL-2.0"
	elif grep -q "Redistributions of source code must retain" "$f"; then
		if grep -q "Neither the name" "$f"; then echo "BSD-3-Clause"; else echo "BSD-2-Clause"; fi
	elif grep -q "Permission is hereby granted, free of charge" "$f"; then echo "MIT"
	else echo "see text"; fi
}

echo "Third-party notices for wowc-advisor"
echo
echo "wowc-advisor itself is MIT licensed (see LICENSE). It links the Go modules below, each under"
echo "the license named next to it. Their license texts follow."
echo
mods=$(go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' ./cmd/wowc-advisor | sort -u)
while read -r path ver dir; do
	f=$(license_file "$dir")
	[ -n "$f" ] || { echo "notices: no license file in $path" >&2; exit 1; }
	echo "  $path $ver  $(license_id "$f")"
done <<<"$mods"
while read -r path ver dir; do
	f=$(license_file "$dir")
	echo
	echo "================================================================================"
	echo "$path $ver ($(license_id "$f"))"
	echo "================================================================================"
	cat "$f"
done <<<"$mods"
