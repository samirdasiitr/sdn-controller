package ipam

import (
	"fmt"
	"net"
	"sync"

	"github.com/sdncontroller/sdncontroller/pkg/netutil"
)

// Allocator manages IP address allocation for a subnet.
type Allocator struct {
	mu        sync.Mutex
	network   *net.IPNet
	gateway   net.IP
	allocated map[uint32]string // IP (as uint32) -> owner identifier
	start     uint32            // First allocatable IP
	end       uint32            // Last allocatable IP
}

// NewAllocator creates a new IP allocator for a subnet CIDR.
func NewAllocator(cidr, gateway string) (*Allocator, error) {
	network, err := netutil.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	gw := net.ParseIP(gateway)
	if gw == nil {
		gw = netutil.FirstUsableIP(network)
	}

	startIP := netutil.FirstUsableIP(network)
	endIP := netutil.LastUsableIP(network)

	return &Allocator{
		network:   network,
		gateway:   gw,
		allocated: make(map[uint32]string),
		start:     netutil.IPToUint32(startIP),
		end:       netutil.IPToUint32(endIP),
	}, nil
}

// Allocate assigns the next available IP address.
func (a *Allocator) Allocate(owner string) (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	gwUint := netutil.IPToUint32(a.gateway.To4())

	for ip := a.start; ip <= a.end; ip++ {
		if ip == gwUint {
			continue
		}
		if _, exists := a.allocated[ip]; !exists {
			a.allocated[ip] = owner
			return netutil.Uint32ToIP(ip), nil
		}
	}
	return nil, fmt.Errorf("no available IPs in subnet %s", a.network.String())
}

// AllocateSpecific assigns a specific IP address.
func (a *Allocator) AllocateSpecific(ip net.IP, owner string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	ipUint := netutil.IPToUint32(ip.To4())
	if ipUint < a.start || ipUint > a.end {
		return fmt.Errorf("IP %s is out of subnet range", ip.String())
	}
	if existing, exists := a.allocated[ipUint]; exists {
		if existing == owner {
			return nil
		}
		return fmt.Errorf("IP %s already allocated to %q", ip.String(), existing)
	}
	a.allocated[ipUint] = owner
	return nil
}

// Release frees an IP address.
func (a *Allocator) Release(ip net.IP) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.allocated, netutil.IPToUint32(ip.To4()))
}

// AllocatedCount returns the number of allocated IPs.
func (a *Allocator) AllocatedCount() int32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return int32(len(a.allocated))
}

// AvailableCount returns the number of available IPs.
func (a *Allocator) AvailableCount() int32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	total := netutil.CountUsableIPs(a.network)
	return int32(total) - int32(len(a.allocated)) - 1 // -1 for gateway
}

// Gateway returns the subnet gateway IP.
func (a *Allocator) Gateway() net.IP {
	return a.gateway
}

// IPAMManager manages allocators for multiple subnets.
type IPAMManager struct {
	mu         sync.RWMutex
	allocators map[string]*Allocator // subnet name -> allocator
}

// NewIPAMManager creates a new IPAM manager.
func NewIPAMManager() *IPAMManager {
	return &IPAMManager{
		allocators: make(map[string]*Allocator),
	}
}

// CreatePool creates an IP pool for a subnet.
func (m *IPAMManager) CreatePool(subnetName, cidr, gateway string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	alloc, err := NewAllocator(cidr, gateway)
	if err != nil {
		return fmt.Errorf("creating pool for subnet %q: %w", subnetName, err)
	}
	m.allocators[subnetName] = alloc
	return nil
}

// DeletePool removes an IP pool.
func (m *IPAMManager) DeletePool(subnetName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.allocators, subnetName)
}

// GetAllocator returns the allocator for a subnet.
func (m *IPAMManager) GetAllocator(subnetName string) (*Allocator, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	alloc, exists := m.allocators[subnetName]
	if !exists {
		return nil, fmt.Errorf("no IPAM pool for subnet %q", subnetName)
	}
	return alloc, nil
}
