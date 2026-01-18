package shim

// Content is the shell script used to bootstrap the container inside WSL
const Content = `#!/bin/sh
ROOTFS=$1
MOUNTS=$2
shift 2

if [ ! -d "$ROOTFS" ]; then
  echo "Error: Rootfs $ROOTFS not found"
  exit 1
fi

# 1. Mount proc
mkdir -p "$ROOTFS/proc"
mount -t proc proc "$ROOTFS/proc"

# 2. Dynamic Bind Mounts (Volumes)
# Format: src1:dst1,src2:dst2
if [ "$MOUNTS" != "none" ]; then
  echo "$MOUNTS" | tr ',' '\n' | while read -r m; do
    SRC=$(echo "$m" | cut -d: -f1)
    DST=$(echo "$m" | cut -d: -f2)
    if [ -n "$SRC" ] && [ -n "$DST" ]; then
      mkdir -p "$ROOTFS/$DST"
      mount --bind "$SRC" "$ROOTFS/$DST"
    fi
  done
fi

# 3. Execution
exec chroot "$ROOTFS" "$@"
`
