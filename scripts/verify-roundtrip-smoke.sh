#!/bin/sh
# CI plumbing test in a disposable Alpine container; NOT independent-host proof.
set -eu
scratch=$(mktemp -d /tmp/plx-offline.XXXXXX)
chmod 755 "$scratch"
trap 'rm -rf -- "$scratch"' EXIT HUP INT TERM
export PLX_CLI="$scratch/plx-env" PLX_CHECK="$scratch/check" PLX_FIXTURE="$scratch/fixture"
go build -o "$PLX_CLI" ./cmd/plx-env
go build -o "$PLX_CHECK" ./scripts/roundtrip-check
go build -o "$PLX_FIXTURE" ./scripts/wsl-fixture
sample="$PWD/examples/environment"
sh scripts/roundtrip-linux.sh start smoke "$scratch/a" "$scratch/send" "$sample"
sh scripts/roundtrip-linux.sh exchange smoke "$scratch/b" "$scratch/return" "$sample" "$scratch/send/package.plxenv"
sh scripts/roundtrip-linux.sh finish smoke "$scratch/c" "$scratch/finish" "$sample" "$scratch/return/package.plxenv"
