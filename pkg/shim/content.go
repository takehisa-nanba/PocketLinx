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
ip link set lo up 2>/dev/null || true # Setup loopback

# 2. Setup Network (DNS & Hosts)
rm -f "$ROOTFS/etc/resolv.conf" "$ROOTFS/etc/hosts"
if [ -f /etc/resolv.conf ]; then
  cat /etc/resolv.conf > "$ROOTFS/etc/resolv.conf"
fi
# Fallback/Append public DNS to ensure resolution
# Fallback removed: Rely on host's resolv.conf (copied above)
# This ensures we use the network's correct DNS servers (e.g. VPN/Intranet)


# Generate basic hosts file
echo "127.0.0.1 localhost" > "$ROOTFS/etc/hosts"
echo "::1       localhost ip6-localhost ip6-loopback" >> "$ROOTFS/etc/hosts"
echo "127.0.1.1 plx-container" >> "$ROOTFS/etc/hosts"

# Service Discovery Injection
if [ -f "$ROOTFS/etc/hosts-extra" ]; then
  cat "$ROOTFS/etc/hosts-extra" >> "$ROOTFS/etc/hosts"
fi

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
CD_CMD=""
if [ "$WORKDIR" != "none" ] && [ -n "$WORKDIR" ]; then
  mkdir -p "$ROOTFS/$WORKDIR"
  CD_CMD="cd \"$WORKDIR\" && "
fi

# 5. Execution
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

if [ $# -eq 0 ]; then
  exec chroot "$ROOTFS" /bin/sh
else
  # Use sh -c for command lookup and PATH injection inside chroot
  exec chroot "$ROOTFS" sh -c "export PATH=$PATH; $CD_CMD exec \"\$@\"" -- "$@"
fi
`
