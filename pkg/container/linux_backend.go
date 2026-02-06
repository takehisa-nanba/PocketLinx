package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LinuxBackend implements the container backend for native Linux systems.
type LinuxBackend struct {
	rootDir string
}

func NewLinuxBackend() *LinuxBackend {
	return &LinuxBackend{
		rootDir: "/var/lib/pocketlinx",
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

	// Install shim (just like in WSL, we need a shim inside the container/distro context)
	// But here, we are the host. We need the shim to be available for 'unshare' calls.
	// We can place it in /usr/local/bin of the HOST (if we have permissions).
	// For self-hosting (nested), we are root, so this is fine.

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

func (b *LinuxBackend) Pull(image string) error {
	url, ok := SupportedImages[image]
	if !ok {
		return fmt.Errorf("image '%s' is not supported", image)
	}

	targetFile := filepath.Join(b.rootDir, "images", image+".tar.gz")
	if _, err := os.Stat(targetFile); err == nil {
		fmt.Printf("Image '%s' already exists.\n", image)
		return nil
	}

	fmt.Printf("Pulling image '%s' from %s...\n", image, url)
	if err := downloadFile(url, targetFile); err != nil {
		return fmt.Errorf("error executing download: %w", err)
	}

	return nil
}

func (b *LinuxBackend) Images() ([]string, error) {
	imagesDir := filepath.Join(b.rootDir, "images")
	files, err := os.ReadDir(imagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var images []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".tar.gz") {
			name := strings.TrimSuffix(f.Name(), ".tar.gz")
			images = append(images, name)
		}
	}
	return images, nil
}

func (b *LinuxBackend) Run(opts RunOptions) error {
	containerId := fmt.Sprintf("c-%d", os.Getpid())

	containerDir := filepath.Join(b.rootDir, "containers", containerId)
	rootfsDir := filepath.Join(containerDir, "rootfs")

	image := opts.Image
	if image == "" {
		image = "alpine"
	}

	imageFile := filepath.Join(b.rootDir, "images", image+".tar.gz")
	if _, err := os.Stat(imageFile); os.IsNotExist(err) {
		return fmt.Errorf("image '%s' not found.", image)
	}

	// 1. Provisioning
	// Create dirs
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return fmt.Errorf("failed to create container dir: %w", err)
	}

	// Extract tar
	fmt.Printf("Extracting %s to %s...\n", imageFile, rootfsDir)
	tarCmd := exec.Command("tar", "-xf", imageFile, "-C", rootfsDir)
	tarCmd.Stdout = os.Stdout
	tarCmd.Stderr = os.Stderr
	if err := tarCmd.Run(); err != nil {
		return fmt.Errorf("failed to extract rootfs: %w", err)
	}

	// 2. Metadata
	meta := Container{
		ID:      containerId,
		Command: strings.Join(opts.Args, " "),
		Created: time.Now(),
		Status:  "Running",
	}
	metaJSON, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(containerDir, "config.json"), metaJSON, 0644); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}

	// 3. Mounts
	mountsStr := "none"
	if len(opts.Mounts) > 0 {
		var mParts []string
		for _, m := range opts.Mounts {
			absSrc, _ := filepath.Abs(m.Source)
			mParts = append(mParts, fmt.Sprintf("%s:%s", absSrc, m.Target))
		}
		mountsStr = strings.Join(mParts, ",")
	}

	// 4. Execution
	// unshare setup
	cmdArgs := []string{"--mount", "--pid", "--fork", "--uts", "--propagation", "unchanged"}

	// Add shim and args
	// We call container-shim which sets up pivot_root and others
	cmdArgs = append(cmdArgs, "/usr/local/bin/container-shim", rootfsDir, mountsStr)
	cmdArgs = append(cmdArgs, opts.Args...)

	runCmd := exec.Command("unshare", cmdArgs...)

	// Env & IO
	runCmd.Stdin = os.Stdin
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	runCmd.Env = os.Environ() // Pass current env

	if opts.Interactive {
		// handle tty checks? For now just inherit
	}

	fmt.Printf("Running container %s (Linux)...\n", containerId)
	err := runCmd.Run()

	// Cleanup / Update status
	meta.Status = "Exited"
	metaJSON, _ = json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(containerDir, "config.json"), metaJSON, 0644)

	return err
}

func (b *LinuxBackend) List() ([]Container, error) {
	containersDir := filepath.Join(b.rootDir, "containers")
	entries, err := os.ReadDir(containersDir)
	if err != nil {
		return nil, nil
	}

	var containers []Container
	for _, e := range entries {
		if e.IsDir() {
			configPath := filepath.Join(containersDir, e.Name(), "config.json")
			data, err := os.ReadFile(configPath)
			if err == nil {
				var c Container
				if err := json.Unmarshal(data, &c); err == nil {
					containers = append(containers, c)
				}
			}
		}
	}
	return containers, nil
}

func (b *LinuxBackend) Stop(id string) error {
	// Simple pkill based on container ID regex in command line
	// BUT since we are using unshare directly, finding the pid is tricky without tracking PIDs.
	// For "Checking implementation" phase, we will just set status to exited.
	// A proper implementation needs to store the PID of the unshare process.
	fmt.Println("Stop not fully implemented for Linux Native yet (requires PID tracking).")
	return nil
}

func (b *LinuxBackend) Logs(id string) (string, error) {
	return "", fmt.Errorf("logs are not captured in native mode yet (stdout is attached)")
}

func (b *LinuxBackend) Remove(id string) error {
	containerDir := filepath.Join(b.rootDir, "containers", id)
	return os.RemoveAll(containerDir)
}

func (b *LinuxBackend) Prune() error {
	// Simple implementation for Linux: remove cache dir
	return os.RemoveAll(filepath.Join(b.rootDir, "cache"))
}

func (b *LinuxBackend) CreateVolume(name string) error {
	return os.MkdirAll(filepath.Join(b.rootDir, "volumes", name), 0755)
}

func (b *LinuxBackend) RemoveVolume(name string) error {
	return os.RemoveAll(filepath.Join(b.rootDir, "volumes", name))
}

func (b *LinuxBackend) ListVolumes() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(b.rootDir, "volumes"))
	if err != nil {
		return []string{}, nil
	}
	var vols []string
	for _, e := range entries {
		if e.IsDir() {
			vols = append(vols, e.Name())
		}
	}
	return vols, nil
}

func (b *LinuxBackend) Build(ctxDir string, tag string) (string, error) {
	// Re-implement simplified Build logic for Linux
	// (Skipping full build implementation for this specific turn to save space, will implement if requested
	// OR implementing a very basic version now)

	// Let's defer full Build implementation for Linux Native to keep it simple first?
	// User wants "plx run", but "plx build" is also useful.
	// Since I am in the "Self-Hosting" task, `plx build` is crucial.
	// I should implement Build.

	dockerfilePath := filepath.Join(ctxDir, "Dockerfile")
	df, err := ParseDockerfile(dockerfilePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse Dockerfile: %w", err)
	}

	imageName := tag
	if imageName == "" {
		imageName = strings.ToLower(filepath.Base(ctxDir))
		if imageName == "." {
			abs, _ := filepath.Abs(ctxDir)
			imageName = strings.ToLower(filepath.Base(abs))
		}
	}

	buildId := fmt.Sprintf("build-%d", os.Getpid())
	buildDir := filepath.Join(b.rootDir, "builds", buildId)
	rootfsDir := filepath.Join(buildDir, "rootfs")
	defer os.RemoveAll(buildDir)

	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return "", err
	}

	// Base image
	if err := b.Pull(df.Base); err != nil {
		return "", err
	}
	baseTar := filepath.Join(b.rootDir, "images", df.Base+".tar.gz")
	exec.Command("tar", "-xf", baseTar, "-C", rootfsDir).Run()

	// 2. Build Steps
	envPrefix := ""

	for _, instr := range df.Instructions {
		switch instr.Type {
		case "ENV":
			for i := 0; i < len(instr.Args); i += 2 {
				k := instr.Args[i]
				v := ""
				if i+1 < len(instr.Args) {
					v = instr.Args[i+1]
				}
				envPrefix += fmt.Sprintf("export %s=%q; ", k, v)
			}
		case "WORKDIR":
			// Skip for now in simplified linux build
		case "RUN":
			runCmd := instr.Raw
			fmt.Printf("STEP: RUN %s\n", runCmd)

			// Same simplified RUN logic as before
			resolvConfPath := filepath.Join(rootfsDir, "etc/resolv.conf")
			_ = os.MkdirAll(filepath.Dir(resolvConfPath), 0755)
			exec.Command("cp", "/etc/resolv.conf", resolvConfPath).Run()

			shimPath := "/usr/local/bin/container-shim"
			if _, err := os.Stat(shimPath); os.IsNotExist(err) {
				return "", fmt.Errorf("container-shim not found at %s. Please run 'plx setup' first", shimPath)
			}

			fullUserCmd := fmt.Sprintf("%s%s", envPrefix, runCmd)
			cmdArgs := []string{"--mount", "--pid", "--fork", "--uts", "--propagation", "unchanged"}
			cmdArgs = append(cmdArgs, shimPath, rootfsDir, "none", "/bin/sh", "-c", fullUserCmd)

			runExec := exec.Command("unshare", cmdArgs...)
			runExec.Stdin = os.Stdin
			runExec.Stdout = os.Stdout
			runExec.Stderr = os.Stderr

			if err := runExec.Run(); err != nil {
				return "", fmt.Errorf("RUN failed: %w", err)
			}

		case "COPY":
			src := filepath.Join(ctxDir, instr.Args[0])
			dst := filepath.Join(rootfsDir, instr.Args[1])
			_ = os.MkdirAll(filepath.Dir(dst), 0755)
			if err := exec.Command("cp", "-r", src, dst).Run(); err != nil {
				return "", fmt.Errorf("COPY failed: %w", err)
			}
		}
	}

	// 4. Save
	if err := os.MkdirAll(filepath.Join(b.rootDir, "images"), 0755); err != nil {
		return "", err
	}
	outTar := filepath.Join(b.rootDir, "images", imageName+".tar.gz")
	fmt.Printf("Saving image to %s...\n", outTar)
	if err := exec.Command("tar", "-czf", outTar, "-C", rootfsDir, ".").Run(); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	return imageName, nil
}
