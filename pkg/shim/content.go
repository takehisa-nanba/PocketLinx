package shim

// Content is the shell script used to bootstrap the container inside WSL
const Content = `#!/bin/sh
ROOTFS=$1
MOUNTS=$2
WORKDIR=$3
shift 3

if [ ! -d "$ROOTFS" ]; then
  echo "Error: Rootfs $ROOTFS not found"
  exit 1
fi

# 1. Mount system directories
mkdir -p "$ROOTFS/proc" "$ROOTFS/sys" "$ROOTFS/dev" "$ROOTFS/tmp" "$ROOTFS/etc"
mount -t proc proc "$ROOTFS/proc"
mount -t sysfs sysfs "$ROOTFS/sys"
mount --rbind /dev "$ROOTFS/dev"
mount -t devpts devpts "$ROOTFS/dev/pts" -o newinstance,ptmxmode=0666
mount -t tmpfs tmpfs "$ROOTFS/tmp"

# 2. Setup Network (DNS & Hosts)
rm -f "$ROOTFS/etc/resolv.conf" "$ROOTFS/etc/hosts"
if [ -f /etc/resolv.conf ]; then
  cat /etc/resolv.conf > "$ROOTFS/etc/resolv.conf"
fi
# Fallback/Append public DNS to ensure resolution
echo "nameserver 8.8.8.8" >> "$ROOTFS/etc/resolv.conf"
echo "nameserver 1.1.1.1" >> "$ROOTFS/etc/resolv.conf"

# Generate basic hosts file
echo "127.0.0.1 localhost" > "$ROOTFS/etc/hosts"
echo "::1       localhost ip6-localhost ip6-loopback" >> "$ROOTFS/etc/hosts"
echo "127.0.1.1 plx-container" >> "$ROOTFS/etc/hosts"

# 3. Dynamic Bind Mounts (Volumes)
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

# 4. Working Directory
if [ "$WORKDIR" != "none" ] && [ -n "$WORKDIR" ]; then
  mkdir -p "$ROOTFS/$WORKDIR"
  # Change directory using chroot logic (append cd to command or use --chdir if available)
  # But chroot usually doesn't take --chdir in older versions or busybox.
  # So we wrap the command.
  exec chroot "$ROOTFS" sh -c "cd \"$WORKDIR\" && exec \"\$@\"" -- "$@"
else
  # 5. Execution
  exec chroot "$ROOTFS" "$@"
fi
`
