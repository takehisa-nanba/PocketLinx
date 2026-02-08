package container

import (
	"fmt"
	"os"
	"sync"
)

// CommandRunner defines how to run shell commands (local or wsl)
type CommandRunner func(cmd string) (string, error)

// BridgeNetworkManager implements NetworkService using Linux bridge/veth
type BridgeNetworkManager struct {
	bridgeName string
	subnet     string
	gatewayIP  string
	ipRange    []string
	usedIPs    map[string]bool
	runner     CommandRunner
	mu         sync.Mutex
}

func NewBridgeNetworkManager(runner CommandRunner) *BridgeNetworkManager {
	// Subnet: 10.10.0.0/24
	// Gateway/Bridge: 10.10.0.1
	// Containers: 10.10.0.2 - 10.10.0.254
	var ips []string
	for i := 2; i < 255; i++ {
		ips = append(ips, fmt.Sprintf("10.10.0.%d", i))
	}

	return &BridgeNetworkManager{
		bridgeName: "plx0",
		subnet:     "10.10.0.0/24",
		gatewayIP:  "10.10.0.1",
		ipRange:    ips,
		usedIPs:    make(map[string]bool),
		runner:     runner, // injected runner (e.g., wslClient.Run)
	}
}

func (m *BridgeNetworkManager) SetupBridge() error {
	// Check if bridge exists
	_, err := m.runner(fmt.Sprintf("/sbin/ip link show %s", m.bridgeName))
	if err == nil {
		if os.Getenv("PLX_VERBOSE") != "" {
			fmt.Println("[DEBUG] Bridge plx0 already exists. Skipping re-init.")
		}
		m.runner("echo 1 > /proc/sys/net/ipv4/ip_forward")
		return nil
	}

	fmt.Println("Initializing Network Bridge plx0...")

	// 1. Create Bridge
	if _, err := m.runner(fmt.Sprintf("/sbin/ip link add name %s type bridge", m.bridgeName)); err != nil {
		return fmt.Errorf("failed to create bridge: %w", err)
	}

	// 2. Assign Gateway IP
	if _, err := m.runner(fmt.Sprintf("/sbin/ip addr add %s/24 dev %s", m.gatewayIP, m.bridgeName)); err != nil {
		return fmt.Errorf("failed to assign ip to bridge: %w", err)
	}

	// 3. Up Bridge
	if _, err := m.runner(fmt.Sprintf("/sbin/ip link set %s up", m.bridgeName)); err != nil {
		return fmt.Errorf("failed to up bridge: %w", err)
	}

	// Ensure IP Forwarding is ON (Crucial for NAT)
	m.runner("echo 1 > /proc/sys/net/ipv4/ip_forward")
	m.runner("sysctl -w net.ipv4.ip_forward=1")

	natRule := fmt.Sprintf("POSTROUTING -s %s ! -d %s -j MASQUERADE", m.subnet, m.subnet)
	m.runner(fmt.Sprintf("iptables -t nat -A %s", natRule))

	return nil
}

func (m *BridgeNetworkManager) AllocateIP() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ip := range m.ipRange {
		if !m.usedIPs[ip] {
			m.usedIPs[ip] = true
			return ip, nil
		}
	}
	return "", fmt.Errorf("no available IPs in range")
}

func (m *BridgeNetworkManager) ReleaseIP(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.usedIPs, ip)
}

func (m *BridgeNetworkManager) MarkIPUsed(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ip == "" {
		return
	}
	m.usedIPs[ip] = true
}

// CreateVethPair creates a veth pair and attaches one end to the bridge.
// Returns the name of the container-side interface.
func (m *BridgeNetworkManager) CreateVethPair(containerID string) (string, string, error) {
	// veth names are limited to 15 chars.
	// ID is based on timestamp (high bits static). Use SUFFIX for variance.
	shortID := containerID
	if len(shortID) > 8 {
		shortID = shortID[len(shortID)-8:] // Use last 8 chars
	}

	hostVeth := fmt.Sprintf("veth%s", shortID)
	contVeth := fmt.Sprintf("ceth%s", shortID) // Peer name before moving

	// 1. Create Pair
	cmd := fmt.Sprintf("/sbin/ip link add %s type veth peer name %s", hostVeth, contVeth)
	if _, err := m.runner(cmd); err != nil {
		return "", "", fmt.Errorf("failed to create veth pair: %w", err)
	}

	// 2. Attach host side to Bridge
	if _, err := m.runner(fmt.Sprintf("/sbin/ip link set %s master %s", hostVeth, m.bridgeName)); err != nil {
		// Clean up if attach fails
		m.runner(fmt.Sprintf("/sbin/ip link delete %s", hostVeth))
		return "", "", fmt.Errorf("failed to attach veth to bridge: %w", err)
	}

	// 3. Up host side
	m.runner(fmt.Sprintf("/sbin/ip link set %s up", hostVeth))

	return hostVeth, contVeth, nil
}

// GetSetupScript generates a shell script that performs all network setup in one go.
func (m *BridgeNetworkManager) GetSetupScript(containerID, ip string) (string, string, error) {
	// 1. Prepare Veth names
	shortID := containerID
	if len(shortID) > 8 {
		shortID = shortID[len(shortID)-8:]
	}
	hostVeth := fmt.Sprintf("veth%s", shortID)
	contVeth := fmt.Sprintf("ceth%s", shortID)

	// 2. Generate the Script
	script := fmt.Sprintf(`
set -e
mkdir -p /var/run/netns
if ! ip netns list | grep -q "^%s$"; then
  ip netns add %s
fi

# Create Veth pair if host side doesn't exist
if ! ip link show %s >/dev/null 2>&1; then
  ip link add %s type veth peer name %s
  ip link set %s master %s
  ip link set %s up
fi

# Move to netns (ignore error if already there)
ip link set %s netns %s 2>/dev/null || true

# Rename and Configure inside netns
ip netns exec %s sh -c "
  ip link set lo up 2>/dev/null || true
  if ip link show %s >/dev/null 2>&1; then
    ip link set %s name eth0
  fi
  if ! ip addr show eth0 | grep -q "%s"; then
    ip addr add %s/24 dev eth0
  fi
  ip link set eth0 up
  ip route add default via %s 2>/dev/null || true
  ethtool -K eth0 tx off 2>/dev/null || true
"
`, containerID, containerID, hostVeth, hostVeth, contVeth, hostVeth, m.bridgeName, hostVeth, contVeth, containerID, containerID, contVeth, contVeth, ip, ip, m.gatewayIP)

	return script, hostVeth, nil
}

// CleanupContainerNetwork removes the netns and releases IP.
// The veth pair is automatically destroyed when netns is removed.
func (m *BridgeNetworkManager) CleanupContainerNetwork(containerID, ip string) error {
	m.ReleaseIP(ip)
	// Deleting netns also cleans up veth pair usually.
	_, err := m.runner(fmt.Sprintf("/sbin/ip netns del %s", containerID))
	return err
}

// GenerateNetworkScript is deprecated in favor of SetupContainerNetwork
func (m *BridgeNetworkManager) GenerateNetworkConfig(contVeth, ip string) string {
	return ""
}
