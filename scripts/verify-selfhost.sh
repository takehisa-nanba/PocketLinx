#!/bin/bash
# Dedicated trusted self-development fixture, directly on an Ubuntu VM.
set -euo pipefail
scratch=${1:?new dedicated absolute directory required}
[[ "$scratch" = /tmp/plx-selfhost-* ]] || exit 2
mkdir -m 755 "$scratch"
mkdir "$scratch/evidence"
exec > >(tee "$scratch/evidence/host.log") 2>&1
finish() {
 status=$?
 if [ "$status" -ne 0 ]; then
  for stage in development restored; do
   if [ -d "$scratch/$stage/source/out" ]; then cp -a "$scratch/$stage/source/out" "$scratch/evidence/failed-$stage"; fi
  done
 fi
 echo "EXIT=$status"
 chmod -R a+rX "$scratch/evidence"
 exit "$status"
}
trap finish EXIT
repo=$PWD
commit=$(git rev-parse HEAD)
printf '%s\n' "$commit" > "$scratch/evidence/source-commit.txt"
uname -a > "$scratch/evidence/platform.txt"
cat /etc/os-release >> "$scratch/evidence/platform.txt"
findmnt -T "$scratch" >> "$scratch/evidence/platform.txt"
mkdir "$scratch/bootstrap-cache" "$scratch/bootstrap-modules"
export GOCACHE="$scratch/bootstrap-cache" GOMODCACHE="$scratch/bootstrap-modules" CGO_ENABLED=0 GOTOOLCHAIN=local
go build -o "$scratch/plx-env" ./cmd/plx-env
go build -o "$scratch/check" ./scripts/roundtrip-check
go build -o "$scratch/fixture" ./scripts/wsl-fixture
curl -fL --retry 3 https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/x86_64/alpine-minirootfs-3.22.1-x86_64.tar.gz -o "$scratch/alpine.tar.gz"
curl -fL --retry 3 https://go.dev/dl/go1.23.12.linux-amd64.tar.gz -o "$scratch/go.tar.gz"
printf '%s  %s\n' 0e5cc5702ad72a4e151f219976ba946d50161c3acce210ef3b122a529aba1270 "$scratch/alpine.tar.gz" d3847fef834e9db11bf64e3fb34db9c04db14e068eeb064f49af747010454f90 "$scratch/go.tar.gz" | sha256sum -c -
sha256sum "$scratch/"*.tar.gz > "$scratch/evidence/materials.sha256"
mkdir "$scratch/alpine"
tar -xzf "$scratch/alpine.tar.gz" -C "$scratch/alpine"
root="$scratch/development"
mkdir -p "$root/rootfs/"{bin,lib,usr/local,workspace,cache,tmp,proc,dev} "$root/source" "$root/volumes/"{cache,tmp}
cp "$scratch/alpine/bin/busybox" "$root/rootfs/bin/"
cp "$scratch/alpine/lib/ld-musl-x86_64.so.1" "$root/rootfs/lib/"
for applet in sh mkdir cat sha256sum id; do ln -s busybox "$root/rootfs/bin/$applet"; done
tar -xzf "$scratch/go.tar.gz" -C "$root/rootfs/usr/local"
cp examples/selfhost/environment.json "$root/environment.json"
# Only committed tracked files; no .git, host keys, local edits or personal files.
git archive "$commit" | tar -x -C "$root/source"
printf '%s\n' "$commit" > "$root/source/SOURCE_COMMIT"
(cd "$root/source"; find . -type f ! -name SOURCE_FILES.sha256 -print0 | sort -z | xargs -0 sha256sum > SOURCE_FILES.sha256)
mkdir -p "$root/volumes/cache/"{build,modules,gopath,home}
# Online preparation only. Keep downloaded transitive checksums outside the source checkout.
mkdir "$scratch/module-preparation"
cp "$root/source/go.mod" "$root/source/go.sum" "$scratch/module-preparation/"
(cd "$scratch/module-preparation"; GOMODCACHE="$root/volumes/cache/modules" "$root/rootfs/usr/local/go/bin/go" mod download -json all) > "$scratch/evidence/modules.json"
(cd "$scratch/module-preparation"; GOMODCACHE="$root/volumes/cache/modules" "$root/rootfs/usr/local/go/bin/go" mod verify) > "$scratch/evidence/modules-verify.txt"
cp "$scratch/module-preparation/go.sum" "$scratch/evidence/prepared-go.sum"
chown -R 1000:1001 "$root/source" "$root/volumes"
measure() {
 local label=$1; shift
 /usr/bin/time -f "$label elapsed_s=%e max_rss_kib=%M" -a -o "$scratch/evidence/resources.txt" "$@"
}
# No external networking during execution, including the post-restore run.
measure first-run unshare --net "$scratch/plx-env" run "$root" | tee "$scratch/evidence/first-run.txt"
cp -a "$root/source/out" "$scratch/evidence/first-build"
"$scratch/check" snapshot "$root" > "$scratch/evidence/development.files.json"
du -sb "$root/rootfs" "$root/rootfs/usr/local/go" "$root/source" "$root/volumes/cache" "$root/volumes/tmp" > "$scratch/evidence/sizes.txt"
measure save "$scratch/plx-env" save --stopped "$root" "$scratch/development.plxenv"
sha256sum "$scratch/development.plxenv" > "$scratch/evidence/package.sha256"
stat -c 'package_bytes=%s' "$scratch/development.plxenv" >> "$scratch/evidence/sizes.txt"
measure restore "$scratch/plx-env" restore "$scratch/development.plxenv" "$scratch/restored"
"$scratch/check" check "$scratch/restored" "$scratch/evidence/development.files.json"
measure restored-run unshare --net "$scratch/plx-env" run "$scratch/restored" | tee "$scratch/evidence/restored-run.txt"
cp -a "$scratch/restored/source/out" "$scratch/evidence/restored-build"
cmp "$scratch/evidence/first-build/binaries.sha256" "$scratch/evidence/restored-build/binaries.sha256"
# Exercise the self-built binary from the host, not nested inside the chroot.
export PLX_FIXTURE_BASE="$scratch/alpine"
"$scratch/fixture" create "$scratch/sample" "$repo/examples/environment"
newcli="$scratch/restored/source/out/plx-env"
"$scratch/check" snapshot "$scratch/sample" > "$scratch/evidence/sample.files.json"
"$newcli" save --stopped "$scratch/sample" "$scratch/sample.plxenv"
"$newcli" restore "$scratch/sample.plxenv" "$scratch/sample-restored"
"$scratch/check" check "$scratch/sample-restored" "$scratch/evidence/sample.files.json"
"$newcli" run "$scratch/sample-restored" > "$scratch/evidence/self-built-sample.txt"
du -sb "$scratch" >> "$scratch/evidence/sizes.txt"
printf 'PASS self-development, offline tests, builds, restore, binary hashes, self-built CLI\n' > "$scratch/evidence/result.txt"
