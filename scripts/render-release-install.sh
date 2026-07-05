#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: render-release-install.sh INPUT OUTPUT RELEASE" >&2
  exit 1
fi

input=$1
output=$2
version=$3
output_dir=$(dirname "$output")

mkdir -p "$output_dir"
sed "s/^RELEASE_VERSION=.*/RELEASE_VERSION=\"$version\"/" "$input" > "$output"
chmod 0755 "$output"
