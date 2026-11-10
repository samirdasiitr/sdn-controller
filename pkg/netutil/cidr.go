package netutil

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
)

// ParseCIDR parses a CIDR string and returns the network.
func ParseCIDR(cidr string) (*net.IPNet, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	return network, nil
}

// ContainsCIDR returns true if outer fully contains inner.
func ContainsCIDR(outer, inner *net.IPNet) bool {
	outerOnes, _ := outer.Mask.Size()
	innerOnes, _ := inner.Mask.Size()
	return outer.Contains(inner.IP) && outerOnes <= innerOnes
}

// OverlapsCIDR returns true if two CIDRs overlap.
func OverlapsCIDR(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

// FirstUsableIP returns the first usable IP (network+1) in a subnet.
func FirstUsableIP(network *net.IPNet) net.IP {
	ip := make(net.IP, len(network.IP))
	copy(ip, network.IP)
	return incrementIP(ip)
}

// LastUsableIP returns the last usable IP (broadcast-1) in a subnet.
func LastUsableIP(network *net.IPNet) net.IP {
	ip := make(net.IP, len(network.IP))
	copy(ip, network.IP)
	for i := range ip {
		ip[i] |= ^network.Mask[i]
	}
	return decrementIP(ip)
}

// CountUsableIPs returns the number of usable IPs in a subnet (excluding network and broadcast).
func CountUsableIPs(network *net.IPNet) int64 {
	ones, bits := network.Mask.Size()
	total := int64(math.Pow(2, float64(bits-ones)))
	if total <= 2 {
		return 0
	}
	return total - 2
}

// IPToUint32 converts an IPv4 address to a uint32.
func IPToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip)
}

// Uint32ToIP converts a uint32 to an IPv4 address.
func Uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}

func incrementIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)
	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			break
		}
	}
	return result
}

func decrementIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)
	for i := len(result) - 1; i >= 0; i-- {
		result[i]--
		if result[i] != 0xFF {
			break
		}
	}
	return result
}
