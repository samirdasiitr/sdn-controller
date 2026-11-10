package netutil

import (
	"fmt"
	"sync"
)

const (
	MinVNI = 1
	MaxVNI = 16777215 // 2^24 - 1
)

// VNIAllocator manages VXLAN Network Identifier allocation.
type VNIAllocator struct {
	mu        sync.Mutex
	allocated map[int32]string // VNI -> VPC name
	nextVNI   int32
}

// NewVNIAllocator creates a new VNI allocator.
func NewVNIAllocator() *VNIAllocator {
	return &VNIAllocator{
		allocated: make(map[int32]string),
		nextVNI:   1000, // Start from 1000 to avoid reserved range
	}
}

// Allocate assigns a VNI to a VPC. If requestedVNI > 0, it tries to use that specific VNI.
func (a *VNIAllocator) Allocate(vpcName string, requestedVNI int32) (int32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if requestedVNI > 0 {
		if owner, exists := a.allocated[requestedVNI]; exists {
			if owner == vpcName {
				return requestedVNI, nil
			}
			return 0, fmt.Errorf("VNI %d already allocated to VPC %q", requestedVNI, owner)
		}
		a.allocated[requestedVNI] = vpcName
		return requestedVNI, nil
	}

	for i := a.nextVNI; i <= MaxVNI; i++ {
		if _, exists := a.allocated[i]; !exists {
			a.allocated[i] = vpcName
			a.nextVNI = i + 1
			return i, nil
		}
	}
	return 0, fmt.Errorf("VNI pool exhausted")
}

// Release frees a VNI.
func (a *VNIAllocator) Release(vni int32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.allocated, vni)
}

// IsAllocated checks if a VNI is in use.
func (a *VNIAllocator) IsAllocated(vni int32) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, exists := a.allocated[vni]
	return exists
}

// Restore loads an existing VNI allocation (used during controller restart).
func (a *VNIAllocator) Restore(vni int32, vpcName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allocated[vni] = vpcName
	if vni >= a.nextVNI {
		a.nextVNI = vni + 1
	}
}
