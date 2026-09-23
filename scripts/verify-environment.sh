#!/bin/sh
# Disposable Alpine/amd64 ONLY. Requires root, CAP_SYS_ADMIN, CAP_SYS_CHROOT,
# CAP_SETUID/CAP_SETGID. The test backend is not a production security boundary.
set -eu
[ "$(uname -m)" = x86_64 ] || { echo 'requires amd64' >&2; exit 1; }
[ "$(id -u)" = 0 ] || { echo 'test namespaces/chroot require root' >&2; exit 1; }
[ -f /etc/alpine-release ] || { echo 'requires Alpine test host' >&2; exit 1; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
scratch=$(mktemp -d /tmp/plx-verify.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT HUP INT TERM
cd "$repo"
go build -o "$scratch/plx-env" ./cmd/plx-env
export PLX_ENV_INTEGRATION=1
export PLX_ENV_BINARY="$scratch/plx-env"
if [ "${PLX_ENV_EXPECT_CREDENTIAL_DENIED:-}" = 1 ]; then
    go test -count=1 -v ./pkg/envrun -run '^TestIntegrationCredentialDenied$'
else
    /usr/bin/time -f 'integration-suite wall_s=%e peak_rss_kib=%M' \
        go test -count=1 -v ./pkg/envrun -run '^TestIntegration'
fi
wc -c "$scratch/plx-env"
