#!/bin/sh
# Stage-based offline exchange. No transport service and no runtime overrides.
set -eu
[ "$#" -ge 5 ] || { echo 'usage: STAGE ROLE ENV_DIR ARTIFACT_DIR SAMPLE_DIR [INPUT_BUNDLE]' >&2; exit 1; }
stage=$1 role=$2 root=$3 artifacts=$4 sample=$5 input=${6:-}
cli=${PLX_CLI:-/usr/local/bin/plx-env}
check=${PLX_CHECK:-/usr/local/bin/plx-roundtrip-check}
fixture=${PLX_FIXTURE:-/usr/local/bin/plx-wsl-fixture}
# Must be a new directory. Keep evidence and packages for review.
mkdir "$artifacts"
"$check" host "$role" > "$artifacts/host.txt"
case "$stage" in
 start)
  "$fixture" create "$root" "$sample"
  "$check" seed "$root"
  state=original
  ;;
 exchange|finish)
  [ -n "$input" ] || exit 1
  expected=$(cat "$input.sha256")
  actual=$(sha256sum "$input"); actual=${actual%% *}
  [ "$actual" = "$expected" ] || { echo 'package transfer hash mismatch' >&2; exit 1; }
  expected=$(cat "$input.files.sha256")
  actual=$(sha256sum "$input.files.json"); actual=${actual%% *}
  [ "$actual" = "$expected" ] || { echo 'evidence transfer hash mismatch' >&2; exit 1; }
  "$cli" restore "$input" "$root"
  if "$cli" restore "$input" "$root"; then echo 'environment overwrite accepted' >&2; exit 1; fi
  "$check" check "$root" "$input.files.json"
  state=$(cat "$input.state")
  case "$stage:$state" in exchange:original|finish:returned) ;; *) echo 'incorrect exchange stage' >&2; exit 1;; esac
  ;;
 *) echo 'unknown stage' >&2; exit 1;;
esac
"$check" execute "$root" "$cli" "$state" > "$artifacts/output.txt"
"$check" denied "$root" "$cli" > "$artifacts/permission-error.txt"
if [ "$stage" = exchange ]; then
 "$fixture" edit "$root" "$sample"
 "$check" delete "$root"
 "$check" execute "$root" "$cli" updated >> "$artifacts/output.txt"
 state=returned
fi
if [ "$stage" != finish ]; then
 out="$artifacts/package.plxenv"
 "$check" snapshot "$root" > "$out.files.json"
 "$cli" save --stopped "$root" "$out"
 hash=$(sha256sum "$out"); printf '%s\n' "${hash%% *}" > "$out.sha256"
 hash=$(sha256sum "$out.files.json"); printf '%s\n' "${hash%% *}" > "$out.files.sha256"
 printf '%s\n' "$state" > "$out.state"
 # Existing output must fail without changing the original package.
 if "$cli" save --stopped "$root" "$out"; then echo 'overwrite accepted' >&2; exit 1; fi
 hash=$(sha256sum "$out"); [ "${hash%% *}" = "$(cat "$out.sha256")" ]
fi
printf '%s\n' "PASS stage=$stage role=$role (host evidence must establish independence)" | tee "$artifacts/result.txt"
