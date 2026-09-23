#!/bin/sh
# Test-only harness for Alpine Linux/amd64, with Go and chroot available.
# chroot here is NOT a production isolation boundary. Run in a disposable test
# environment as root; never use this harness for untrusted rootfs/scripts.
set -eu
[ "$(uname -m)" = x86_64 ] || { echo 'requires amd64' >&2; exit 1; }
[ "$(id -u)" = 0 ] || { echo 'test chroot requires root' >&2; exit 1; }
[ -f /etc/alpine-release ] || { echo 'requires Alpine test host' >&2; exit 1; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
scratch=$(mktemp -d /tmp/plx-verify.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT HUP INT TERM
cd "$repo"
go build -o "$scratch/plx-env" ./cmd/plx-env
go build -o "$scratch/plx-before" ./cmd/plx
src="$scratch/original"
mkdir -p "$src/rootfs/bin" "$src/rootfs/lib" "$src/source" "$src/volumes/data"
# Minimal Alpine userland, not a full distro export. No external downloads.
cp /bin/busybox "$src/rootfs/bin/busybox"
cp /lib/ld-musl-x86_64.so.1 "$src/rootfs/lib/ld-musl-x86_64.so.1"
ln -s busybox "$src/rootfs/bin/sh"
cp "$repo/examples/environment/environment.json" "$src/environment.json"
cp "$repo/examples/environment/hello.sh" "$src/source/hello.sh"
chmod 755 "$src/source/hello.sh"
printf 'persistent-data\n' > "$src/volumes/data/message.txt"

execute_copy() {
    # Materialize source/data in a disposable execution copy only. This is test
    # setup, not production mount/runtime integration.
    envdir=$1
    runroot=$2
    cp -a "$envdir/rootfs" "$runroot"
    mkdir -p "$runroot/workspace" "$runroot/data"
    cp -a "$envdir/source/." "$runroot/workspace/"
    cp -a "$envdir/volumes/data/." "$runroot/data/"
    GREETING=hello chroot "$runroot" /bin/sh -c 'cd /workspace; exec /bin/sh ./hello.sh'
}

before=$(execute_copy "$src" "$scratch/run-before")
[ "$before" = 'hello|/workspace|persistent-data' ]
/usr/bin/time -f 'save wall_s=%e peak_rss_kib=%M' "$scratch/plx-env" save --stopped "$src" "$scratch/first.plxenv"
/usr/bin/time -f 'restore wall_s=%e peak_rss_kib=%M' "$scratch/plx-env" restore "$scratch/first.plxenv" "$scratch/restored"
after=$(execute_copy "$scratch/restored" "$scratch/run-after")
[ "$before" = "$after" ]
cmp "$src/environment.json" "$scratch/restored/environment.json"
cmp "$src/rootfs/bin/busybox" "$scratch/restored/rootfs/bin/busybox"
cmp "$src/volumes/data/message.txt" "$scratch/restored/volumes/data/message.txt"

printf '\nprintf "edited\\n"\n' >> "$scratch/restored/source/hello.sh"
printf 'updated-data\n' > "$scratch/restored/volumes/data/message.txt"
"$scratch/plx-env" save --stopped "$scratch/restored" "$scratch/second.plxenv"
"$scratch/plx-env" restore "$scratch/second.plxenv" "$scratch/back"
expected=$(printf 'hello|/workspace|updated-data\nedited')
[ "$(execute_copy "$scratch/back" "$scratch/run-back")" = "$expected" ]
# Confirm the original was not modified by the operation or the harness.
[ "$(cat "$src/volumes/data/message.txt")" = persistent-data ]
cmp "$src/source/hello.sh" "$repo/examples/environment/hello.sh"

echo 'Disk bytes (binary sizes and uncompressed bundle; not VHDX size):'
wc -c "$scratch/plx-before" "$scratch/plx-env" "$scratch/first.plxenv"
du -sk "$src" "$scratch/restored"
echo 'Legacy version command only (not container startup):'
/usr/bin/time -f 'legacy-version wall_s=%e peak_rss_kib=%M' "$scratch/plx-before" version
echo 'PASS: Alpine shell execution before/after restore and local changed resave'
