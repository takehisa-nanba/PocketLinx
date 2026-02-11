package container

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// CommandRunner defines how to run shell commands (local or wsl)
type CommandRunner interface {
	Run(cmd string) (string, error)
}

// FunctionRunner makes a function satisfy CommandRunner
type FunctionRunner func(cmd string) (string, error)

func (f FunctionRunner) Run(cmd string) (string, error) {
	return f(cmd)
}

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
	if _, err := m.runner.Run(fmt.Sprintf("/sbin/ip link show %s", m.bridgeName)); err == nil {
		if os.Getenv("PLX_VERBOSE") != "" {
			fmt.Printf("[DEBUG] Bridge %s already exists. Ensuring forwarding is ON.\n", m.bridgeName)
		}
		m.runner.Run("/sbin/sysctl -w net.ipv4.ip_forward=1")
		return nil
	}

	fmt.Printf("Initializing Network Bridge %s...\n", m.bridgeName)

	// 1. Create Bridge
	if _, err := m.runner.Run(fmt.Sprintf("/sbin/ip link add name %s type bridge", m.bridgeName)); err != nil {
		return fmt.Errorf("failed to create bridge: %w", err)
	}

	// 2. Assign Gateway IP
	if _, err := m.runner.Run(fmt.Sprintf("/sbin/ip addr add %s/24 dev %s", m.gatewayIP, m.bridgeName)); err != nil {
		return fmt.Errorf("failed to assign ip to bridge: %w", err)
	}

	// 3. Up Bridge
	if _, err := m.runner.Run(fmt.Sprintf("/sbin/ip link set %s up", m.bridgeName)); err != nil {
		return fmt.Errorf("failed to up bridge: %w", err)
	}

	// 4. IP Forwarding
	if _, err := m.runner.Run("/sbin/sysctl -w net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("failed to enable ip forwarding: %w", err)
	}

	// 5. NAT (MASQUERADE)
	natRule := fmt.Sprintf("POSTROUTING -s %s ! -d %s -j MASQUERADE", m.subnet, m.subnet)
	if _, err := m.runner.Run(fmt.Sprintf("/usr/sbin/iptables -t nat -C %s", natRule)); err != nil {
		if _, err := m.runner.Run(fmt.Sprintf("/usr/sbin/iptables -t nat -A %s", natRule)); err != nil {
			return fmt.Errorf("failed to add NAT rule: %w", err)
		}
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
	if _, err := m.runner.Run(cmd); err != nil {
		return "", "", fmt.Errorf("failed to create veth pair: %w", err)
	}

	// 2. Attach host side to Bridge
	if _, err := m.runner.Run(fmt.Sprintf("/sbin/ip link set %s master %s", hostVeth, m.bridgeName)); err != nil {
		// Clean up if attach fails
		m.runner.Run(fmt.Sprintf("/sbin/ip link delete %s", hostVeth))
		return "", "", fmt.Errorf("failed to attach veth to bridge: %w", err)
	}

	// 3. Up host side
	m.runner.Run(fmt.Sprintf("/sbin/ip link set %s up", hostVeth))

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
# Force cleanup stale namespace to prevent "File exists" error (v1.0.8: more aggressive)
ip netns del %s 2>/dev/null || true
ip link del %s 2>/dev/null || true
ip netns add %s

# Create Veth pair
ip link add %s type veth peer name %s
ip link set %s master %s
ip link set %s up

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
`, containerID, hostVeth, containerID, hostVeth, contVeth, hostVeth, m.bridgeName, hostVeth, contVeth, containerID, containerID, contVeth, contVeth, contVeth, ip, ip, m.gatewayIP)

	return script, hostVeth, nil
}

// CleanupContainerNetwork removes the netns and releases IP.
// The veth pair is automatically destroyed when netns is removed.
func (m *BridgeNetworkManager) CleanupContainerNetwork(containerID, ip string) error {
	m.ReleaseIP(ip)
	// Deleting netns also cleans up veth pair usually.
	_, err := m.runner.Run(fmt.Sprintf("/sbin/ip netns del %s", containerID))
	return err
}

// GenerateNetworkScript is deprecated in favor of SetupContainerNetwork
func (m *BridgeNetworkManager) GenerateNetworkConfig(contVeth, ip string) string {
	return ""
}
