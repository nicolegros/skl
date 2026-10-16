#!/usr/bin/env bash
# Renders the skl formula for a release and prints it to stdout.
# Usage: render-formula.sh <version> <source.tar.gz>
set -euo pipefail

die() {
  echo "render-formula: $*" >&2
  exit 1
}

[[ $# -eq 2 ]] || die "usage: $0 <version> <source.tar.gz>"
version="$1"
tarball="$2"

[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] ||
  die "version must be MAJOR.MINOR.PATCH without a leading v, got '$version'"
[[ -s "$tarball" ]] || die "source tarball missing or empty: $tarball"

sha="$(shasum -a 256 "$tarball" | cut -d ' ' -f 1)"

template="$(dirname "$0")/skl.rb.tmpl"
rendered="$(sed -e "s/@VERSION@/$version/" -e "s/@SHA256@/$sha/" "$template")"
if grep -q '@[A-Z0-9_]*@' <<<"$rendered"; then
  die "unrendered placeholder left in $template"
fi
printf '%s\n' "$rendered"
