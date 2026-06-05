//go:build antivm && linux

package core

import (
	"os"
	"path/filepath"
	"strings"
)

func init() {
	registerLinuxChecks()
}

func registerLinuxChecks() {
	// 100% certainty
	registerCheck("firmware_dmi", firmwareDMI, 100)

	// 95% certainty
	registerCheck("devices_pci", devicesPCI, 95)

	// 80% certainty
	registerCheck("uml_cpu", umlCPU, 80)
	registerCheck("kgt_signature", kgtSignature, 80)

	// 75% certainty
	registerCheck("nsjail_pid", nsjailPID, 75)

	// 70% certainty
	registerCheck("vmware_ioports", vmwareIOPorts, 70)
	registerCheck("cgroup", cgroupVM, 70)
	registerCheck("qemu_fw_cfg", qemuFWCfg, 70)

	// 65% certainty
	registerCheck("dmi_product", dmiProduct, 65)
	registerCheck("dmi_sys_vendor", dmiSysVendor, 65)
	registerCheck("dmi_chassis_vendor", dmiChassisVendor, 65)
	registerCheck("vmware_iomem", vmwareIOMEM, 65)
	registerCheck("vmware_dmesg", vmwareDMESG, 65)

	// 55% certainty
	registerCheck("dmidecode", dmidecodeCheck, 55)
	registerCheck("dmesg", dmesgCheck, 55)

	// 50% certainty
	registerCheck("dmi_scan", dmiScan, 50)
	registerCheck("smbios_vm_bit", smbiosVMBit, 50)
	registerCheck("thread_mismatch_linux", threadMismatchLinux, 50)
	registerCheck("amd_sev_msr", amdSEVMSR, 50)

	// 40% certainty
	registerCheck("vmware_scsi", vmwareSCSI, 40)
	registerCheck("qemu_virtual_dmi", qemuVirtualDMI, 40)
	registerCheck("processes_linux", processesLinux, 40)

	// 35% certainty
	registerCheck("systemd_virt", systemdVirt, 35)
	registerCheck("hwmon", hwmonMissing, 35)

	// 30% certainty
	registerCheck("wsl_proc", wslProc, 30)

	// 20% certainty
	registerCheck("hypervisor_dir", hypervisorDir, 20)
	registerCheck("temperature", temperatureCheck, 20)
	registerCheck("qemu_usb", qemuUSB, 20)
	registerCheck("ctype", dmiChassisType, 20)

	// 15% certainty
	registerCheck("vbox_module", vboxModule, 15)
	registerCheck("sysinfo_proc", sysinfoProc, 15)
	registerCheck("file_access_history", fileAccessHistory, 15)

	// 10% certainty
	registerCheck("linux_user_host", linuxUserHost, 10)

	// 5% certainty
	registerCheck("podman_file", podmanFile, 5)
	registerCheck("bluestacks", bluestacksFolders, 5)
	registerCheck("kmsg", kmsgCheck, 5)
}

// --- Linux 100% ---

func firmwareDMI() bool {
	// Check all DMI entries for VM signatures
	dirs := []string{
		"/sys/class/dmi/id",
		"/sys/devices/virtual/dmi/id",
	}
	vmBrands := []string{"virtualbox", "vmware", "qemu", "kvm", "innotek",
		"xen", "bochs", "hyper-v", "microsoft", "oracle", "oem"}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(d, e.Name()))
			if err != nil {
				continue
			}
			content := strings.ToLower(string(data))
			for _, brand := range vmBrands {
				if strings.Contains(content, brand) {
					return true
				}
			}
		}
	}
	return false
}

// --- Linux 95% ---

func devicesPCI() bool {
	entries, err := os.ReadDir("/sys/bus/pci/devices")
	if err != nil {
		return false
	}
	// VM vendors: 8086 (Intel), 15ad (VMware), 80ee (VirtualBox),
	// 1af4 (Red Hat/QEMU), 10de (NVIDIA in passthrough)
	vmVendors := []string{"15ad", "80ee", "1af4"}
	for _, e := range entries {
		vendorFile := filepath.Join("/sys/bus/pci/devices", e.Name(), "vendor")
		data, err := os.ReadFile(vendorFile)
		if err != nil {
			continue
		}
		vendor := strings.TrimSpace(string(data))
		for _, v := range vmVendors {
			if strings.Contains(strings.ToLower(vendor), v) {
				return true
			}
		}
	}
	return false
}

// --- Linux 80% ---

func umlCPU() bool {
	content := readFile("/proc/cpuinfo")
	return strings.Contains(content, "UML")
}

func kgtSignature() bool {
	content := readFile("/proc/cpuinfo")
	return strings.Contains(content, "Intel KGT") || strings.Contains(content, "TGKIntel")
}

// --- Linux 75% ---

func nsjailPID() bool {
	content := readFile("/proc/self/status")
	if content == "" {
		return false
	}
	// nsjail makes processes see PID 1 in the namespace
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "Pid:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[1] == "1" {
				// Check if nsjail env vars are present
				if os.Getenv("NSJAIL") != "" {
					return true
				}
				// Check cgroup for nsjail
				cg := readFile("/proc/self/cgroup")
				return strings.Contains(cg, "nsjail")
			}
		}
	}
	return false
}

// --- Linux 70% ---

func vmwareIOPorts() bool {
	return strings.Contains(readFile("/proc/ioports"), "VMware")
}

func cgroupVM() bool {
	content := readFile("/proc/1/cgroup")
	if content == "" {
		return false
	}
	for _, s := range []string{"docker", "lxc", "kubepods"} {
		if strings.Contains(content, s) {
			return true
		}
	}
	return false
}

func qemuFWCfg() bool {
	if _, err := os.Stat("/sys/firmware/qemu_fw_cfg"); err == nil {
		return true
	}
	if _, err := os.Stat("/proc/device-tree/fw-cfg"); err == nil {
		return true
	}
	return false
}

// --- Linux 65% ---

func dmiProduct() bool {
	name := readFile("/sys/class/dmi/id/product_name")
	switch {
	case strings.Contains(name, "VirtualBox"):
		return true
	case strings.Contains(name, "VMware"):
		return true
	case name == "KVM" || name == "QEMU":
		return true
	case strings.Contains(name, "Bochs"):
		return true
	case strings.Contains(name, "Hyper-V"):
		return true
	}
	return false
}

func dmiSysVendor() bool {
	vendor := readFile("/sys/class/dmi/id/sys_vendor")
	switch {
	case strings.Contains(vendor, "innotek"):
		return true
	case strings.Contains(vendor, "Xen"):
		return true
	case strings.Contains(vendor, "Microsoft"):
		return true
	case strings.Contains(vendor, "QEMU"):
		return true
	}
	return false
}

func dmiChassisVendor() bool {
	vendor := readFile("/sys/class/dmi/id/chassis_vendor")
	return strings.Contains(vendor, "Oracle") || strings.Contains(vendor, "innotek")
}

func vmwareIOMEM() bool {
	return strings.Contains(readFile("/proc/iomem"), "VMware")
}

func vmwareDMESG() bool {
	content := readFile("/var/log/dmesg")
	if content == "" {
		content = readFile("/var/log/kern.log")
	}
	return strings.Contains(content, "VMware") || strings.Contains(content, "vmware")
}

// --- Linux 55% ---

func dmidecodeCheck() bool {
	// Check all DMI sysfs files for VM brands
	entries, err := os.ReadDir("/sys/class/dmi/id")
	if err != nil {
		return false
	}
	patterns := []string{"virtualbox", "vmware", "qemu", "kvm", "innotek", "xen", "bochs"}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/sys/class/dmi/id", e.Name()))
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))
		for _, p := range patterns {
			if strings.Contains(content, p) {
				return true
			}
		}
	}
	return false
}

func dmesgCheck() bool {
	content := readFile("/var/log/dmesg")
	if content == "" {
		return false
	}
	for _, s := range []string{"Hypervisor", "VMware", "VirtualBox", "QEMU", "KVM", "Xen"} {
		if strings.Contains(content, s) {
			return true
		}
	}
	return false
}

// --- Linux 50% ---

func dmiScan() bool {
	return firmwareDMI()
}

func smbiosVMBit() bool {
	// Check /sys/firmware/dmi/tables for SMBIOS entry point
	data, err := os.ReadFile("/sys/firmware/dmi/tables/smbios_entry_point")
	if err != nil {
		return false
	}
	// SMBIOS v3+ has a VM bit at offset 0x09
	if len(data) >= 0x1F {
		if data[0x1E]&0x01 != 0 {
			return true
		}
	}
	return false
}

func threadMismatchLinux() bool {
	cpuContent := readFile("/proc/cpuinfo")
	if cpuContent == "" {
		return false
	}
	n := 0
	for _, line := range strings.Split(cpuContent, "\n") {
		if strings.HasPrefix(line, "processor") {
			n++
		}
	}
	// If we see CPU brand but low core count, likely VM
	if n < 2 && strings.Contains(cpuContent, "Intel") || strings.Contains(cpuContent, "AMD") {
		return true
	}
	return false
}

func amdSEVMSR() bool {
	// Check /sys/kernel/security/sev for AMD SEV/SEV-ES/SEV-SNP
	if data, err := os.ReadFile("/sys/kernel/security/sev/version"); err == nil {
		return strings.TrimSpace(string(data)) != ""
	}
	// Also check MSR via /dev/cpu (requires root)
	data, err := os.ReadFile("/dev/cpu/0/msr")
	if err != nil {
		return false
	}
	if len(data) > 0x800 {
		_ = data[0x800]
	}
	return false
}

// --- Linux 40% ---

func vmwareSCSI() bool {
	return strings.Contains(readFile("/proc/scsi/scsi"), "VMware")
}

func qemuVirtualDMI() bool {
	content := readFile("/sys/devices/virtual/dmi/id/product_name")
	if strings.Contains(content, "QEMU") || strings.Contains(content, "KVM") || strings.Contains(content, "VirtualBox") {
		return true
	}
	vendor := readFile("/sys/devices/virtual/dmi/id/sys_vendor")
	return strings.Contains(vendor, "QEMU")
}

func processesLinux() bool {
	vmProcs := []string{"vbox", "vmware", "VBox", "VMware", "qemu", "QEMU", "xen", "Xen"}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only check numeric directories (process PIDs)
		if e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(cmdline))
		for _, p := range vmProcs {
			if strings.EqualFold(comm, p) {
				return true
			}
		}
	}
	return false
}

// --- Linux 35% ---

func systemdVirt() bool {
	// Check /sys/devices/virtual/dmi/id for VM indicators
	// In containers, /sys might not have DMI info
	vendor := readFile("/sys/devices/virtual/dmi/id/sys_vendor")
	if vendor == "" {
		// No DMI info — likely container
		return cgroupVM() || dockerEnv()
	}
	return strings.Contains(vendor, "QEMU") || strings.Contains(vendor, "Xen")
}

func hwmonMissing() bool {
	_, err := os.ReadDir("/sys/class/hwmon")
	return err != nil
}

// --- Linux 30% ---

func wslProc() bool {
	version := readFile("/proc/version")
	if strings.Contains(version, "Microsoft") || strings.Contains(version, "WSL") || strings.Contains(version, "microsoft") {
		return true
	}
	osrelease := readFile("/proc/sys/kernel/osrelease")
	return strings.Contains(osrelease, "Microsoft") || strings.Contains(osrelease, "WSL")
}

// --- Linux 20% ---

func hypervisorDir() bool {
	entries, err := os.ReadDir("/sys/hypervisor")
	if err != nil {
		return false
	}
	return len(entries) > 0
}

func temperatureCheck() bool {
	entries, err := os.ReadDir("/sys/class/thermal")
	if err != nil {
		return true // no thermal info = likely VM
	}
	return len(entries) < 2
}

func qemuUSB() bool {
	data, err := os.ReadFile("/sys/kernel/debug/usb/devices")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "QEMU")
}

func dmiChassisType() bool {
	typ := readFile("/sys/class/dmi/id/chassis_type")
	return typ == "" || typ == "0"
}

// --- Linux 15% ---

func vboxModule() bool {
	content := readFile("/proc/modules")
	return strings.Contains(content, "vbox")
}

func sysinfoProc() bool {
	content := readFile("/proc/sysinfo")
	return strings.Contains(content, "QEMU") || strings.Contains(content, "KVM") || strings.Contains(content, "Xen")
}

func fileAccessHistory() bool {
	// Check if there's very low file access history (common in fresh VMs)
	_, err := os.Stat("/root/.bash_history")
	hasHistory := err == nil
	_, err2 := os.Stat("/home")
	hasHome := err2 == nil
	return hasHome && !hasHistory
}

// --- Linux 10% ---

func linuxUserHost() bool {
	hostname := readFile("/proc/sys/kernel/hostname")
	if hostname == "" {
		return false
	}
	lower := strings.ToLower(hostname)
	for _, s := range []string{"ubuntu", "debian", "localhost"} {
		if lower == s || strings.HasPrefix(lower, s+"-") {
			return true
		}
	}
	return false
}

// --- Linux 5% ---

func podmanFile() bool {
	_, err := os.Stat("/run/.containerenv")
	return err == nil
}

func bluestacksFolders() bool {
	entries, err := os.ReadDir("/mnt")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name()), "bluestacks") {
			return true
		}
	}
	return false
}

func kmsgCheck() bool {
	f, err := os.OpenFile("/dev/kmsg", os.O_RDONLY|os.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return false
	}
	content := strings.ToLower(string(buf[:n]))
	return strings.Contains(content, "hypervisor") || strings.Contains(content, "kvm")
}
