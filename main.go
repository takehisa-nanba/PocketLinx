package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: u-container <command> <args>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		run()
	case "setup":
		setup()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

const (
	distroName = "u-container"
	rootfsUrl  = "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.0-x86_64.tar.gz"
	rootfsFile = "alpine-rootfs.tar.gz"
)

// setup: Alpine LinuxのrootfsをダウンロードしてWSLに登録する
func setup() {
	fmt.Println("Setting up lightweight Alpine environment...")

	// 1. Download rootfs
	if _, err := os.Stat(rootfsFile); os.IsNotExist(err) {
		fmt.Printf("Downloading Alpine rootfs from %s...\n", rootfsUrl)
		cmd := exec.Command("powershell", "-Command", fmt.Sprintf("Invoke-WebRequest -Uri %s -OutFile %s", rootfsUrl, rootfsFile))
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error downloading rootfs: %v\n", err)
			return
		}
	}

	// 2. Import to WSL
	installDir := "distro"
	absInstallDir, _ := filepath.Abs(installDir)
	absRootfsFile, _ := filepath.Abs(rootfsFile)

	fmt.Printf("Importing %s into WSL as '%s'...\n", absRootfsFile, distroName)
	fmt.Printf("Install location: %s\n", absInstallDir)

	// Clean up existing distro and directory
	exec.Command("wsl.exe", "--unregister", distroName).Run()
	os.RemoveAll(absInstallDir)
	if err := os.MkdirAll(absInstallDir, 0755); err != nil {
		fmt.Printf("Error creating install directory: %v\n", err)
		return
	}

	cmd := exec.Command("wsl.exe", "--import", distroName, absInstallDir, absRootfsFile, "--version", "2")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error importing distro: %v\n", err)
		// Clean up on failure
		os.RemoveAll(absInstallDir)
		return
	}

	// 3. Create container-shim (Robustly)
	fmt.Println("Installing container-shim...")
	shimContent := `#!/bin/sh
ROOTFS=$1
shift
if [ -d "$ROOTFS" ]; then
  exec chroot "$ROOTFS" "$@"
else
  echo "Error: Rootfs $ROOTFS not found"
  exit 1
fi
`
	// Use printf to avoid line ending issues. We escape the content for sh/printf.
	// Actually, easier to write a temp file in Windows then cp? No, that failed.
	// We can use 'wsl sh -c "cat > ..."' and pass content via Stdin!
	shimCmd := exec.Command("wsl.exe", "-d", distroName, "--", "sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim")
	shimCmd.Stdin = strings.NewReader(shimContent)
	shimCmd.Stdout = os.Stdout
	shimCmd.Stderr = os.Stderr
	if err := shimCmd.Run(); err != nil {
		fmt.Printf("Error checking/installing shim: %v\n", err)
		return
	}

	fmt.Println("Setup complete! You can now use 'run' command.")
}

// run: WSL内で指定されたコマンドを隔離された環境で実行する
func run() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: u-container run <command> <args>")
		os.Exit(1)
	}

	cmdArgs := os.Args[2:]
	fmt.Printf("Running %v in isolated container...\n", cmdArgs)

	// 1. Generate Container ID
	containerId := generateUUID()
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", containerId)
	rootfsDir := filepath.Join(containerDir, "rootfs")

	// Base image path (inside WSL)
	// For now, we reuse the host's rootfs or a prepared base.
	// To make it simple securely, let's copy the /bin, /lib, /usr etc from host to container rootfs?
	// No, that's slow.
	// Better approach for MVP:
	// We assume /var/lib/pocketlinx/images/base exists.
	// If not, we create it from current rootfs (dangerous?) or just download again?
	// Let's use a simpler approach: Extract the tarball again if needed or assume we have a clean copy.
	//
	// **Strategy for this step**:
	// We will instruct WSL to:
	// 1. mkdir -p <rootfs>
	// 2. tar -xf <original_tarball> -C <rootfs> (Slow but safe)
	//
	// Wait, the tarball is on Windows side. Accessing it from WSL /mnt/c/... is easy.

	// Convert Windows path to WSL path for the rootfs tarball
	// e.g. C:\Users\USER... -> /mnt/c/Users/USER...
	wslRootfsPath, err := windowsToWslPath(rootfsFile) // We need to resolve this path absolutely first
	if err != nil {
		fmt.Printf("Error resolving paths: %v\n", err)
		os.Exit(1)
	}

	// Prepare Container Rootfs command
	setupCmd := fmt.Sprintf(
		"mkdir -p %s && tar -xf %s -C %s",
		rootfsDir, wslRootfsPath, rootfsDir,
	)

	fmt.Printf("Provisioning container %s...\n", containerId)
	if err := exec.Command("wsl.exe", "-d", distroName, "--", "sh", "-c", setupCmd).Run(); err != nil {
		fmt.Printf("Error provisioning container: %v\n", err)
		os.Exit(1)
	}

	// 2. Execute with unshare
	// unshare flags:
	// -m (mount), -u (uts), -i (ipc), -n (net), -p (pid), -f (fork)
	// --mount-proc: Mount /proc automatically
	//
	// Command: unshare ... /usr/local/bin/container-shim <rootfs> <cmd>

	// Construct command string for shim
	// We need to join args properly.
	userCmd := ""
	for _, arg := range cmdArgs {
		userCmd += fmt.Sprintf(" %q", arg) // changing to quoted string
	}

	shimCmd := fmt.Sprintf("/bin/sh /usr/local/bin/container-shim %s %s", rootfsDir, userCmd)

	wslArgs := []string{
		"-d", distroName,
		"--",
		"unshare", "--pid", "--fork", "--mount-proc", "--mount", "--uts",
		"sh", "-c", shimCmd,
	}

	cmd := exec.Command("wsl.exe", wslArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running container: %v\n", err)
		// Cleanup (Optional for debug, but good for cleanliness)
		exec.Command("wsl.exe", "-d", distroName, "--", "rm", "-rf", containerDir).Run()
		os.Exit(1)
	}

	// Cleanup
	fmt.Printf("Cleaning up container %s...\n", containerId)
	exec.Command("wsl.exe", "-d", distroName, "--", "rm", "-rf", containerDir).Run()
}

// Helper to generate simple UUID (pseudo)
func generateUUID() string {

	// In real app use crypto/rand
	// For now simple fix string is enough to test, but let's do better.
	// Just use current time nano + random
	return fmt.Sprintf("c-%d", os.Getpid()) // Simple enough for single user
}

// Helper to convert Windows path to WSL path
func windowsToWslPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// C:\Users... -> /mnt/c/Users...
	// 1. Replace backslashes
	// 2. Lowercase drive letter
	// 3. Prepend /mnt/

	// Simple naive implementation
	drive := abs[0]
	driveLower := string(drive + 32) // 'C' -> 'c' roughly
	rest := abs[3:]
	rest = filepath.ToSlash(rest)

	return fmt.Sprintf("/mnt/%s/%s", driveLower, rest), nil
}
