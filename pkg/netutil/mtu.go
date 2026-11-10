package netutil

const (
	// Tunnel encapsulation overhead in bytes.
	VXLANOverhead = 50 // 8 VXLAN + 8 UDP + 20 IP + 14 Ethernet
	IPIPOverhead  = 20 // 20 IP outer header
	GREOverhead   = 24 // 4 GRE + 20 IP

	DefaultMTU     = 1500
	JumboMTU       = 9000
	MinMTU         = 1280
	MaxMTU         = 9216
)

// InnerMTU calculates the inner MTU after tunnel encapsulation overhead.
func InnerMTU(physicalMTU int, tunnelType string) int {
	overhead := tunnelOverhead(tunnelType)
	inner := physicalMTU - overhead
	if inner < MinMTU {
		return MinMTU
	}
	return inner
}

// RequiredPhysicalMTU calculates the physical MTU needed for a given inner MTU.
func RequiredPhysicalMTU(innerMTU int, tunnelType string) int {
	return innerMTU + tunnelOverhead(tunnelType)
}

// ValidateMTUChain validates that the physical MTU can support the desired inner MTU.
func ValidateMTUChain(physicalMTU, desiredInnerMTU int, tunnelType string) (bool, int) {
	required := RequiredPhysicalMTU(desiredInnerMTU, tunnelType)
	return physicalMTU >= required, required
}

func tunnelOverhead(tunnelType string) int {
	switch tunnelType {
	case "VXLAN":
		return VXLANOverhead
	case "IPIP":
		return IPIPOverhead
	case "GRE":
		return GREOverhead
	default:
		return VXLANOverhead
	}
}
