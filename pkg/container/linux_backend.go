//go:build linux

package container

import (
	"fmt"
	"os"
	"path/filepath"
)

// LinuxBackend implements the container backend for native Linux systems.
type LinuxBackend struct {
	rootDir string
	Runtime RuntimeService
	Image   ImageService
	Volume  VolumeService
}

func NewBackend() Backend {
	if os.Geteuid() != 0 {
		fmt.Println("Warning: PocketLinx on Linux requires root privileges (for unshare/mount). Please run with sudo.")
	}
	return NewLinuxBackend()
}

func NewLinuxBackend() *LinuxBackend {
	rootDir := "/var/lib/pocketlinx"
	return &LinuxBackend{
		rootDir: rootDir,
		Runtime: NewLinuxRuntimeService(rootDir),
		Image:   NewLinuxImageService(rootDir),
		Volume:  NewLinuxVolumeService(rootDir),
	}
}

func (b *LinuxBackend) Install() error {
	// On Linux, we assume 'plx' is placed in PATH by the user or package manager.
	// We could copy it to /usr/local/bin, but for now we skip.
	return nil
}

func (b *LinuxBackend) Setup() error {
	fmt.Println("Setting up PocketLinx environment (Linux)...")

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Join(b.rootDir, "images"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(b.rootDir, "containers"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(b.rootDir, "builds"), 0755); err != nil {
		return err
	}

	shimContent := `#!/bin/sh
# container-shim
# Arguments: rootfs_dir mounts_str [command...]

ROOTFS=$1
shift
MOUNTS=$1
shift

# 1. Mount setup
# Remount rootfs as private to avoid propagation (ignore failure)
mount --make-rprivate / || true

# Bind mount rootfs to itself to start clean
mount --bind $ROOTFS $ROOTFS
cd $ROOTFS

# Mount proc, sys, dev (Do this BEFORE pivot/chroot so they are available)
# But for pivot_root, we usually do it after. For chroot, we need them inside.
# Let's mount them now.
mount -t proc proc proc/
mount -t sysfs sys sys/
mount -t devtmpfs dev dev/

# Custom Mounts
if [ "$MOUNTS" != "none" ] && [ -n "$MOUNTS" ]; then
    IFS=','
    for M in $MOUNTS; do
        SRC=${M%%:*}
        DST=${M#*:}
		# Remove any leading / from DST to make it relative to current root
		REL_DST=$(echo $DST | sed 's|^/||')
        mkdir -p $REL_DST
        mount --bind /old_root/$SRC $REL_DST || mount --bind $SRC $REL_DST
    done
    unset IFS
fi

# Try pivot_root
mkdir -p .old_root
if pivot_root . .old_root; then
    # Success: Unmount old root
    umount -l /old_root
    rmdir /old_root
    
    # Exec
    exec "$@"
else
    # Fallback: chroot
    echo "Note: pivot_root failed, using chroot instead."
    exec chroot . "$@"
fi

`
	// Write shim to /usr/local/bin/container-shim
	shimPath := "/usr/local/bin/container-shim"
	if err := os.WriteFile(shimPath, []byte(shimContent), 0755); err != nil {
		fmt.Printf("Warning: Failed to install container-shim to %s: %v. \n", shimPath, err)
	}

	// Pull default image
	return b.Pull("alpine")
}

// Delegation

// Runtime
func (b *LinuxBackend) Run(opts RunOptions) error      { return b.Runtime.Run(opts) }
func (b *LinuxBackend) Start(id string) error          { return b.Runtime.Start(id) }
func (b *LinuxBackend) List() ([]Container, error)     { return b.Runtime.List() }
func (b *LinuxBackend) Stop(id string) error           { return b.Runtime.Stop(id) }
func (b *LinuxBackend) Logs(id string) (string, error) { return b.Runtime.Logs(id) }
func (b *LinuxBackend) Remove(id string) error         { return b.Runtime.Remove(id) }

// Image
func (b *LinuxBackend) Pull(image string) error   { return b.Image.Pull(image) }
func (b *LinuxBackend) Images() ([]string, error) { return b.Image.Images() }
func (b *LinuxBackend) Build(ctxDir string, dockerfile string, tag string) (string, error) {
	return b.Image.Build(ctxDir, dockerfile, tag)
}
func (b *LinuxBackend) Prune() error { return b.Image.Prune() }

func (b *LinuxBackend) Diff(image1, image2 string) (string, error) {
	return b.Image.Diff(image1, image2)
}

func (b *LinuxBackend) ExportDiff(baseImage, targetImage, outputPath string) error {
	return b.Image.ExportDiff(baseImage, targetImage, outputPath)
}

// Volume
func (b *LinuxBackend) CreateVolume(name string) error { return b.Volume.Create(name) }
func (b *LinuxBackend) RemoveVolume(name string) error { return b.Volume.Remove(name) }
func (b *LinuxBackend) ListVolumes() ([]string, error) { return b.Volume.List() }

func (b *LinuxBackend) GetIP(id string) (string, error)         { return b.Runtime.GetIP(id) }
func (b *LinuxBackend) Update(id string, opts RunOptions) error { return b.Runtime.Update(id, opts) }
func (b *LinuxBackend) Exec(id string, cmd []string, interactive bool) error {
	return b.Runtime.Exec(id, cmd, interactive)
}
