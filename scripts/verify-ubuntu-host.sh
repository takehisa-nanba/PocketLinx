#!/bin/bash
# Run directly on a standard GitHub Ubuntu VM. No Docker command is used.
set -euo pipefail
scratch=${1:?dedicated temporary directory required}
mkdir -m 755 "$scratch"
mkdir "$scratch/evidence"
mkdir "$scratch/tmp" "$scratch/go-cache" "$scratch/go-modules"
export TMPDIR="$scratch/tmp" GOCACHE="$scratch/go-cache" GOMODCACHE="$scratch/go-modules"
exec > >(tee "$scratch/evidence/host-run.log") 2>&1
trap 'status=$?; if [ "$status" -ne 0 ]; then echo "FAILED status=$status line=$LINENO command=$BASH_COMMAND"; fi; chmod -R a+rX "$scratch/evidence"; exit "$status"' EXIT
{
 uname -a
 cat /etc/os-release
 lscpu
 findmnt -T "$scratch" -o TARGET,SOURCE,FSTYPE,OPTIONS
 id
 grep '^Cap' /proc/self/status
 systemd-detect-virt || true
} > "$scratch/evidence/platform.txt"
export PLX_CLI="$scratch/plx-env" PLX_CHECK="$scratch/check" PLX_FIXTURE="$scratch/fixture"
go build -o "$PLX_CLI" ./cmd/plx-env
go build -o "$PLX_CHECK" ./scripts/roundtrip-check
go build -o "$PLX_FIXTURE" ./scripts/wsl-fixture
"$PLX_CHECK" host linux-vm
# The whole archive is checksum-pinned; fixture copies only BusyBox and musl.
archive="$scratch/alpine.tar.gz"
curl --fail --location --retry 3 --output "$archive" https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/x86_64/alpine-minirootfs-3.22.1-x86_64.tar.gz
echo "0e5cc5702ad72a4e151f219976ba946d50161c3acce210ef3b122a529aba1270  $archive" | sha256sum -c -
export PLX_FIXTURE_BASE="$scratch/alpine"
mkdir "$PLX_FIXTURE_BASE"
tar -xzf "$archive" -C "$PLX_FIXTURE_BASE"
sha256sum "$archive" "$PLX_FIXTURE_BASE/bin/busybox" "$PLX_FIXTURE_BASE/lib/ld-musl-x86_64.so.1" > "$scratch/evidence/materials.sha256"
cp "$PLX_FIXTURE_BASE/lib/apk/db/installed" "$scratch/evidence/alpine-packages.txt"
# Isolated host namespace probe: root mount changes must not enter the runner namespace.
unshare --mount --pid --fork bash -euxc '
 mount --make-rprivate /
 mkdir "$1/probe-a" "$1/probe-b"
 mount --bind "$1/probe-a" "$1/probe-b"
 umount "$1/probe-b"
' probe "$scratch"
sample="$PWD/examples/environment"
sh scripts/roundtrip-linux.sh start linux-vm "$scratch/original" "$scratch/evidence/start" "$sample"
sh scripts/roundtrip-linux.sh exchange linux-vm "$scratch/restored" "$scratch/evidence/changed" "$sample" "$scratch/evidence/start/package.plxenv"
sh scripts/roundtrip-linux.sh finish linux-vm "$scratch/back" "$scratch/evidence/finish" "$sample" "$scratch/evidence/changed/package.plxenv"
echo 'PASS: Ubuntu VM host direct execution, not a container or physical host'
