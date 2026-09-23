#!/bin/sh
set -eu
read -r message < /data/message.txt
printf '%s|%s|%s\n' "$GREETING" "$PWD" "$message"
