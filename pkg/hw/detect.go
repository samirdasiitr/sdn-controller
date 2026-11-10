package hw

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	MellanoxVendorID = "0x15b3"
	CX6DeviceID      = "0x101d"
	CX6DXDeviceID    = "0x101f"
	CX7DeviceID      = "0x1021"

	sysClassNet = "/sys/class/net"
)

// NICInfo contains discovered NIC hardware information.
type NICInfo struct {
	Device         string
	Vendor         string
	VendorID       string
	Model          string
	DeviceID       string
	Driver         string
	OffloadCapable bool
	SwitchdevMode  bool
	NumVFs         int
	Speed          string
	FirmwareVer    string
	PCIAddr        string
}

// DiscoverNICs scans the system for network interfaces and their hardware capabilities.
func DiscoverNICs() ([]NICInfo, error) {
	entries, err := os.ReadDir(sysClassNet)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sysClassNet, err)
	}

	var nics []NICInfo
	for _, entry := range entries {
		devPath := filepath.Join(sysClassNet, entry.Name(), "device")
		if _, err := os.Stat(devPath); os.IsNotExist(err) {
			continue // Skip virtual interfaces without PCI backing
		}

		nic := NICInfo{Device: entry.Name()}
		nic.VendorID = readSysfs(filepath.Join(devPath, "vendor"))
		nic.DeviceID = readSysfs(filepath.Join(devPath, "device"))
		nic.Driver = readDriverName(devPath)
		nic.Speed = readSysfs(filepath.Join(sysClassNet, entry.Name(), "speed"))
		nic.PCIAddr = readPCIAddr(devPath)

		// Identify vendor/model
		switch nic.VendorID {
		case MellanoxVendorID:
			nic.Vendor = "Mellanox"
			nic.OffloadCapable = true
			switch nic.DeviceID {
			case CX6DeviceID:
				nic.Model = "ConnectX-6"
			case CX6DXDeviceID:
				nic.Model = "ConnectX-6 Dx"
			case CX7DeviceID:
				nic.Model = "ConnectX-7"
			default:
				nic.Model = "ConnectX-Unknown"
				nic.OffloadCapable = false
			}
		default:
			nic.Vendor = "Unknown"
		}

		// Check switchdev mode
		nic.SwitchdevMode = checkSwitchdevMode(nic.PCIAddr)
		nic.NumVFs = countVFs(devPath)
		nic.FirmwareVer = readSysfs(filepath.Join(sysClassNet, entry.Name(), "device", "firmware_version"))

		nics = append(nics, nic)
	}

	return nics, nil
}

// IsMellanoxOffloadCapable returns true if the NIC is a Mellanox CX6/7 with offload support.
func IsMellanoxOffloadCapable(nic NICInfo) bool {
	if nic.VendorID != MellanoxVendorID {
		return false
	}
	switch nic.DeviceID {
	case CX6DeviceID, CX6DXDeviceID, CX7DeviceID:
		return true
	}
	return false
}

// DetectDPU checks if a BlueField DPU is present on the system.
func DetectDPU() (present bool, model string) {
	entries, err := os.ReadDir("/sys/bus/pci/devices")
	if err != nil {
		return false, ""
	}
	for _, entry := range entries {
		vendorPath := filepath.Join("/sys/bus/pci/devices", entry.Name(), "vendor")
		devicePath := filepath.Join("/sys/bus/pci/devices", entry.Name(), "device")
		vendor := readSysfs(vendorPath)
		device := readSysfs(devicePath)
		if vendor == MellanoxVendorID {
			switch device {
			case "0xa2d6":
				return true, "BlueField-2"
			case "0xa2dc":
				return true, "BlueField-3"
			}
		}
	}
	return false, ""
}

func readSysfs(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readDriverName(devPath string) string {
	driverLink, err := os.Readlink(filepath.Join(devPath, "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(driverLink)
}

func readPCIAddr(devPath string) string {
	realPath, err := filepath.EvalSymlinks(devPath)
	if err != nil {
		return ""
	}
	return filepath.Base(realPath)
}

func checkSwitchdevMode(pciAddr string) bool {
	if pciAddr == "" {
		return false
	}
	// Check via devlink (simplified - real impl would use netlink)
	modePath := filepath.Join("/sys/bus/pci/devices", pciAddr, "sriov_drivers_autoprobe")
	return readSysfs(modePath) != ""
}

func countVFs(devPath string) int {
	numVFs := readSysfs(filepath.Join(devPath, "sriov_numvfs"))
	if numVFs == "" {
		return 0
	}
	var n int
	fmt.Sscanf(numVFs, "%d", &n)
	return n
}
