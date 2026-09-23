#!/bin/sh
set -eu
read -r message < "$DATA_FILE"
printf '%s|%s|%s|uid=%s|gid=%s\n' "$GREETING" "$PWD" "$message" "$(/bin/busybox id -u)" "$(/bin/busybox id -g)"
