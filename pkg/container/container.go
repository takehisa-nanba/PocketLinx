package container

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/takehisa-nanba/PocketLinx/pkg/shim"
	"github.com/takehisa-nanba/PocketLinx/pkg/wsl"
)

const (
	DistroName = "u-container"
	RootfsUrl  = "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.0-x86_64.tar.gz"
	RootfsFile = "alpine-rootfs.tar.gz"
)

// Setup initializes the PocketLinx environment (WSL distro)
func Setup() error {
	fmt.Println("Setting up PocketLinx environment...")
	wslClient := wsl.NewClient(DistroName)

	// 1. Download rootfs
	if _, err := os.Stat(RootfsFile); os.IsNotExist(err) {
		fmt.Printf("Downloading Alpine rootfs from %s...\n", RootfsUrl)
		err := wslClient.Run("powershell", "-Command", fmt.Sprintf("Invoke-WebRequest -Uri %s -OutFile %s", RootfsUrl, RootfsFile))
		if err != nil {
			return fmt.Errorf("error downloading rootfs: %w", err)
		}
	}

	// 2. Import to WSL
	installDir := "distro"
	absInstallDir, _ := filepath.Abs(installDir)
	absRootfsFile, _ := filepath.Abs(RootfsFile)

	fmt.Printf("Importing %s into WSL as '%s'...\n", absRootfsFile, DistroName)

	// Clean up existing distro and directory
	wslClient.Run("wsl.exe", "--unregister", DistroName)
	// Note: wslClient.Run calls "wsl.exe" internally?
	// The Run method in client.go takes args for wsl.exe.
	// So calling wslClient.Run("wsl.exe", ...) would execute "wsl.exe wsl.exe ..." which is wrong.
	// client.Run executes "wsl.exe <args>".
	// So we should pass "--unregister" directly.

	wslClient.Run("--unregister", DistroName) // Ignore error if not exists

	os.RemoveAll(absInstallDir)
	if err := os.MkdirAll(absInstallDir, 0755); err != nil {
		return fmt.Errorf("error creating install directory: %w", err)
	}

	// Import
	err := wslClient.Run("--import", DistroName, absInstallDir, absRootfsFile, "--version", "2")
	if err != nil {
		os.RemoveAll(absInstallDir)
		return fmt.Errorf("error importing distro: %w", err)
	}

	// 3. Create container-shim
	fmt.Println("Installing container-shim...")
	err = wslClient.RunDistroCommandWithInput(
		shim.Content,
		"sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim",
	)
	if err != nil {
		return fmt.Errorf("error installing shim: %w", err)
	}

	fmt.Println("Setup complete!")
	return nil
}

// Run executes a command in an isolated container
func Run(args []string) error {
	wslClient := wsl.NewClient(DistroName)
	containerId := fmt.Sprintf("c-%d", os.Getpid())

	fmt.Printf("Running %v in container %s...\n", args, containerId)

	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", containerId)
	rootfsDir := filepath.Join(containerDir, "rootfs")

	// Convert Windows path to WSL path for caching/copying strategy if needed
	// For now we assume rootfsFile is in CWD
	wslRootfsPath, err := wsl.WindowsToWslPath(RootfsFile)
	if err != nil {
		return fmt.Errorf("path resolving error: %w", err)
	}

	// Provisioning
	setupCmd := fmt.Sprintf("mkdir -p %s && tar -xf %s -C %s", rootfsDir, wslRootfsPath, rootfsDir)
	// fmt.Printf("Provisioning...\n")
	if err := wslClient.RunDistroCommand("sh", "-c", setupCmd); err != nil {
		return fmt.Errorf("provisioning failed: %w", err)
	}

	// Execution
	// Construct the user command
	userCmd := ""
	for _, arg := range args {
		userCmd += fmt.Sprintf(" %q", arg)
	}

	shimCmd := fmt.Sprintf("/bin/sh /usr/local/bin/container-shim %s %s", rootfsDir, userCmd)

	// Unshare arguments
	unshareArgs := []string{
		"unshare", "--pid", "--fork", "--mount-proc", "--mount", "--uts",
		"sh", "-c", shimCmd,
	}

	if err := wslClient.RunDistroCommand(unshareArgs...); err != nil {
		// Cleanup on error
		wslClient.RunDistroCommand("rm", "-rf", containerDir)
		return fmt.Errorf("execution failed: %w", err)
	}

	// Cleanup
	// fmt.Printf("Cleaning up...\n")
	wslClient.RunDistroCommand("rm", "-rf", containerDir)
	return nil
}
