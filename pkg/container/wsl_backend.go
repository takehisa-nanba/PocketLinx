//go:build windows

package container

import (
	"PocketLinx/pkg/wsl"
	"fmt"
)

// WSLBackend is the composite backend that delegates to specific services
type WSLBackend struct {
	wslClient *wsl.Client
	Runtime   RuntimeService
	Image     ImageService
	Volume    VolumeService
}

func NewBackend() Backend {
	return NewWSLBackend()
}

// NewWSLBackend initializes the backend with decomposed services
func NewWSLBackend() *WSLBackend {
	client := wsl.NewClient(DistroName)

	// Dependency Injection style
	return &WSLBackend{
		wslClient: client,
		Runtime:   NewWSLRuntimeService(client),
		Image:     NewWSLImageService(client),
		Volume:    NewWSLVolumeService(client),
	}
}

func (b *WSLBackend) Install() error {
	return InstallBinary()
}

func (b *WSLBackend) Setup() error {
	// Setup logic remains here as it initializes the environment for ALL services
	fmt.Println("Setting up PocketLinx environment (WSL2)...")

	// 1. Host side directories
	_ = GetImagesDir()
	_ = GetDistroDir()

	// 2. Distro side initialization
	fmt.Println("Fixing filesystem and network in pocketlinx distro... (PATCHED)")
	initCmds := `
		# A. Fix WSL Configuration to prevent automatic overwrites
		cat <<EOF > /etc/wsl.conf
[network]
generateResolvConf = false
[interop]
enabled = true
appendWindowsPath = true
EOF

		# B. Fix DNS first
		echo "nameserver 8.8.8.8" > /etc/resolv.conf
		echo "nameserver 1.1.1.1" >> /etc/resolv.conf

		# C. Update and Install core tools
		apk update
		apk add --no-cache tzdata util-linux socat iproute2 iptables

		# D. Set Timezone (Copy instead of link for early boot stability)
		if [ -f /usr/share/zoneinfo/Asia/Tokyo ]; then
			cp /usr/share/zoneinfo/Asia/Tokyo /etc/localtime
			echo "Asia/Tokyo" > /etc/timezone
		fi

		# E. Satisfy WSL init process (Fix ldconfig failed error)
		# 最後に実行することで、apkによる上書きを確実に防ぐ
		mkdir -p /etc/ld.so.conf.d
		rm -f /sbin/ldconfig /usr/sbin/ldconfig
		printf "#!/bin/sh\nexit 0\n" | tr -d '\r' > /sbin/ldconfig
		chmod +x /sbin/ldconfig
		cp /sbin/ldconfig /usr/sbin/ldconfig

		# F. Flush legacy NAT rules (from previous implementations)
		# Ensure iptables is installed first
		command -v iptables >/dev/null || apk add --no-cache iptables
		iptables -t nat -F

		# 設定を確実に反映
		sync
	`
	if err := b.wslClient.RunDistroCommand("sh", "-c", initCmds); err != nil {
		return fmt.Errorf("failed to initialize pocketlinx distro: %w", err)
	}

	fmt.Println("Environment is now healthy.")
	return nil
}

// Delegation Methods

// Runtime
func (b *WSLBackend) Run(opts RunOptions) error      { return b.Runtime.Run(opts) }
func (b *WSLBackend) Start(id string) error          { return b.Runtime.Start(id) }
func (b *WSLBackend) List() ([]Container, error)     { return b.Runtime.List() }
func (b *WSLBackend) Stop(id string) error           { return b.Runtime.Stop(id) }
func (b *WSLBackend) Logs(id string) (string, error) { return b.Runtime.Logs(id) }
func (b *WSLBackend) Remove(id string) error         { return b.Runtime.Remove(id) }

// Image
func (b *WSLBackend) Pull(image string) error   { return b.Image.Pull(image) }
func (b *WSLBackend) Images() ([]string, error) { return b.Image.Images() }
func (b *WSLBackend) Build(ctxDir string, tag string) (string, error) {
	return b.Image.Build(ctxDir, tag)
}
func (b *WSLBackend) Prune() error { return b.Image.Prune() }

// Volume
func (b *WSLBackend) CreateVolume(name string) error { return b.Volume.Create(name) }
func (b *WSLBackend) RemoveVolume(name string) error { return b.Volume.Remove(name) }
func (b *WSLBackend) ListVolumes() ([]string, error) { return b.Volume.List() }

func (b *WSLBackend) GetIP(id string) (string, error)         { return b.Runtime.GetIP(id) }
func (b *WSLBackend) Update(id string, opts RunOptions) error { return b.Runtime.Update(id, opts) }
