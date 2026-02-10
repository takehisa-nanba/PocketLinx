package shim

// Content is the shell script used to bootstrap the container inside WSL
const Content = `#!/bin/sh
ROOTFS=$1
MOUNTS=$2
WORKDIR=$3
USER=$4
PID_FILE=$5
shift 5

if [ -z "$ROOTFS" ]; then
  echo "Error: ROOTFS is empty. Refusing to continue to protect host system."
  exit 1
fi

if [ -n "$PID_FILE" ]; then
  echo $$ > "$PID_FILE"
fi

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
if [ -f "/etc/resolv.conf" ]; then
  # Backup existing resolv.conf and try to use it (v0.8.1)
  cp /etc/resolv.conf "$ROOTFS/etc/resolv.conf.bak" 2>/dev/null
  cat /etc/resolv.conf > "$ROOTFS/etc/resolv.conf" 2>/dev/null
fi
# Fallback to Google DNS if empty (v0.8.1)
if [ ! -s "$ROOTFS/etc/resolv.conf" ]; then
  echo "nameserver 8.8.8.8" > "$ROOTFS/etc/resolv.conf"
fi
rm -f "$ROOTFS/etc/hosts"


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
if [ -n "$PLX_CONTAINER_PATH" ]; then
  export PATH="$PLX_CONTAINER_PATH"
else
  export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
fi

if [ -n "$USER" ] && [ "$USER" != "root" ] && [ "$USER" != "none" ]; then
  # Verify user exists inside chroot
  if ! /usr/sbin/chroot "$ROOTFS" id "$USER" >/dev/null 2>&1; then
    # Fallback to manual grep check for robustness (v0.7.16)
    # Use grep with absolute search or ensure it's in path
    if ! /bin/grep -q "^$USER:" "$ROOTFS/etc/passwd" 2>/dev/null && ! /usr/bin/grep -q "^$USER:" "$ROOTFS/etc/passwd" 2>/dev/null; then
      echo "Warning: user '$USER' not found in /etc/passwd, falling back to root"
      USER="root"
    fi
  fi
fi

if [ $# -eq 0 ]; then
  if [ -n "$USER" ] && [ "$USER" != "root" ] && [ "$USER" != "none" ]; then
    # Search for su (v0.7.17)
    SU_EXE="su"
    if [ -f "$ROOTFS/bin/su" ]; then SU_EXE="/bin/su"; elif [ -f "$ROOTFS/usr/bin/su" ]; then SU_EXE="/usr/bin/su"; fi
    exec /usr/sbin/chroot "$ROOTFS" "$SU_EXE" - "$USER"
  else
    exec /usr/sbin/chroot "$ROOTFS" /bin/sh
  fi
else
  # Use sh -c for command lookup and PATH injection inside chroot
  if [ -n "$USER" ] && [ "$USER" != "root" ] && [ "$USER" != "none" ]; then
    # su -m preserves environment (including our injected PATH)
    # Search for su (v0.7.17)
    SU_EXE="su"
    if [ -f "$ROOTFS/bin/su" ]; then SU_EXE="/bin/su"; elif [ -f "$ROOTFS/usr/bin/su" ]; then SU_EXE="/usr/bin/su"; fi
    # Use HOME and TERM from environment if available (v0.8.1)
    exec /usr/sbin/chroot "$ROOTFS" "$SU_EXE" -m "$USER" -c "export HOME=${HOME:-/home/$USER}; export TERM=${TERM:-xterm}; $CD_CMD exec \"\$@\"" sh "$@"
  else
    # Explicitly use /bin/sh for chroot exec (v0.7.17)
    exec /usr/sbin/chroot "$ROOTFS" /bin/sh -c "$CD_CMD exec \"\$@\"" sh "$@"
  fi
fi
`
