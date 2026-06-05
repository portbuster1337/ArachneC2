//go:build antivm && darwin

package core

import (
	"os/exec"
	"strings"
)

func init() {
	registerDarwinChecks()
}

func registerDarwinChecks() {
	// 100%
	registerCheck("hwmodel", hwModelMac, 100)
	registerCheck("mac_iokit", macIOKit, 100)
	registerCheck("ioreg_grep", ioregGrep, 100)
	registerCheck("mac_sip", macSIP, 100)
	registerCheck("mac_sys_profiler", macSysProfiler, 100)

	// 50%
	registerCheck("amd_sev_msr_darwin", amdSEVMSRDarwin, 50)

	// 15%
	registerCheck("mac_memsize", macMemSize, 15)
}

func runCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func hwModelMac() bool {
	model := runCmd("sysctl", "-n", "hw.model")
	return !strings.HasPrefix(strings.ToLower(model), "mac")
}

func macIOKit() bool {
	out := runCmd("ioreg", "-l")
	lower := strings.ToLower(out)
	for _, s := range []string{"virtualbox", "vmware", "qemu", "kvm", "vbox", "parallel"} {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func ioregGrep() bool {
	out := runCmd("ioreg", "-l")
	if out == "" {
		return false
	}
	lower := strings.ToLower(out)
	return strings.Contains(lower, "virtualbox") || strings.Contains(lower, "vmware") ||
		strings.Contains(lower, "qemu") || strings.Contains(lower, "parallels")
}

func macSIP() bool {
	// Check System Integrity Protection status
	sip := runCmd("csrutil", "status")
	if strings.Contains(sip, "disabled") || strings.Contains(sip, "unknown") {
		return false
	}
	// Check if hv is present
	hv := runCmd("sysctl", "-n", "kern.hv_support")
	return hv == "1"
}

func macSysProfiler() bool {
	out := runCmd("system_profiler", "SPHardwareDataType")
	lower := strings.ToLower(out)
	return strings.Contains(lower, "virtualbox") || strings.Contains(lower, "vmware") ||
		strings.Contains(lower, "qemu") || strings.Contains(lower, "parallels")
}

func amdSEVMSRDarwin() bool {
	// macOS doesn't have /dev/cpu, so check via sysctl
	model := runCmd("sysctl", "-n", "machdep.cpu.brand_string")
	return strings.Contains(model, "AMD EPYC") || strings.Contains(model, "AMD Ryzen")
}

func macMemSize() bool {
	// Check if memory is too low (< 2GB would be suspicious for macOS)
	out := runCmd("sysctl", "-n", "hw.memsize")
	if out == "" {
		return false
	}

	// Parse bytes to GB
	var bytes uint64
	for _, c := range out {
		bytes = bytes*10 + uint64(c-'0')
	}
	gb := bytes / 1024 / 1024 / 1024
	return gb < 2
}


