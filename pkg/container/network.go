package container

import (
	"fmt"
	"os"
	"strings"
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

func NewBridgeNetworkManager(runner CommandRunner, bridgeName, subnet string) *BridgeNetworkManager {
	if bridgeName == "" {
		bridgeName = "plx0"
	}
	if subnet == "" {
		subnet = "10.10.0.0/24"
	}

	// Derive gateway and range from subnet (simple logic for now)
	prefix := subnet[:strings.LastIndex(subnet, ".")+1]
	gatewayIP := prefix + "1"

	var ips []string
	for i := 2; i < 255; i++ {
		ips = append(ips, fmt.Sprintf("%s%d", prefix, i))
	}

	return &BridgeNetworkManager{
		bridgeName: bridgeName,
		subnet:     subnet,
		gatewayIP:  gatewayIP,
		ipRange:    ips,
		usedIPs:    make(map[string]bool),
		runner:     runner,
	}
}

func (m *BridgeNetworkManager) SetupBridge() error {
	// Check if bridge exists
	_, err := m.runner(fmt.Sprintf("/sbin/ip link show %s", m.bridgeName))
	if err == nil {
		if os.Getenv("PLX_VERBOSE") != "" {
			fmt.Printf("[DEBUG] Bridge %s already exists. Skipping re-init.\n", m.bridgeName)
		}
		m.runner("echo 1 > /proc/sys/net/ipv4/ip_forward")
		return nil
	}

	fmt.Printf("Initializing Network Bridge %s...\n", m.bridgeName)

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
	// Check if rule exists before adding (v0.8.0)
	if _, err := m.runner(fmt.Sprintf("/sbin/iptables -t nat -C %s", natRule)); err != nil {
		m.runner(fmt.Sprintf("/sbin/iptables -t nat -A %s", natRule))
	}

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
# Force cleanup stale namespace to prevent "File exists" error (v0.7.4)
ip netns del %s 2>/dev/null || true
ip netns add %s

# Create Veth pair if host side doesn't exist
# Force cleanup host veth if it exists to prevent "File exists" error (v0.7.18)
ip link del %s 2>/dev/null || true
if ! ip link show %s >/dev/null 2>&1; then
  ip link add %s type veth peer name %s
  ip link set %s master %s
  ip link set %s up
fi

# Move to netns (v0.7.17: Fail fast to avoid ghost devices)
ip link set %s netns %s

# Rename and Configure inside netns
ip netns exec %s sh -c '
  set -e
  ip link set lo up 2>/dev/null || true
  # Wait for interface to appear in namespace (max 2s, v0.7.16)
  _i=0
  _found=0
  while [ "$_i" -lt 20 ]; do
    if ip link show %s >/dev/null 2>&1; then _found=1; break; fi
    sleep 0.1
    _i=$((_i+1))
  done
  if [ "$_found" -eq 0 ]; then
    echo "Error: Device %s failed to appear in netns" >&2
    exit 1
  fi
  ip link set %s name eth0
  
  if ! ip addr show eth0 | grep -q "%s"; then
    ip addr add %s/24 dev eth0
  fi
  ip link set eth0 up
  ip route add default via %s 2>/dev/null || true
  ethtool -K eth0 tx off 2>/dev/null || true
'
`, containerID, containerID, hostVeth, hostVeth, hostVeth, contVeth, hostVeth, m.bridgeName, hostVeth, contVeth, containerID, containerID, contVeth, contVeth, contVeth, ip, ip, m.gatewayIP)

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
