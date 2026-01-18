package shim

// Content is the shell script used to bootstrap the container inside WSL
const Content = `#!/bin/sh
ROOTFS=$1
shift
if [ -d "$ROOTFS" ]; then
  exec chroot "$ROOTFS" "$@"
else
  echo "Error: Rootfs $ROOTFS not found"
  exit 1
fi
`
