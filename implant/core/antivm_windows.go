//go:build antivm && windows

package core

import (
	"log"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func init() {
	registerWindowsChecks()
}

func registerWindowsChecks() {
	// Protocol: name, fn, certainty

	// 150%
	registerCheck("dbvm", dbvmCheck, 150)

	// 100%
	registerCheck("wine", wineDetection, 100)
	registerCheck("mutex", vmMutex, 100)
	registerCheck("drivers", vmDrivers, 100)
	registerCheck("disk_serial", diskSerialVM, 100)
	registerCheck("ivshmem", ivshmemCheck, 100)
	registerCheck("handles", handlesCheck, 100)
	registerCheck("virtual_processors", virtualProcessors, 100)
	registerCheck("hypervisor_query", hypervisorQuery, 100)
	registerCheck("acpi_signature", acpiSignatureCheck, 100)
	registerCheck("firmware_tables", firmwareTablesCheck, 100)
	registerCheck("boot_logo", bootLogoCheck, 100)
	registerCheck("kernel_objects", kernelObjectsCheck, 100)
	registerCheck("nvram", nvramCheck, 100)
	registerCheck("edid", edidCheck, 100)
	registerCheck("clock_timing", clockTiming, 100)

	// 95%
	registerCheck("devices_pci", devicesPCICheck, 95)

	// 90%
	registerCheck("virtual_registry", virtualRegistryCheck, 90)
	registerCheck("cpu_heuristic", cpuHeuristicCheck, 90)

	// 75% (32-bit only)
	registerCheck("vpc_invalid", vpcInvalidCheck, 75)

	// 50%
	registerCheck("dll", vmDLLs, 50)

	// 45%
	registerCheck("clock", clockCheck, 45)

	// 40%
	registerCheck("processes", vmProcessesWindows, 40)

	// 30%
	registerCheck("cuckoo_dir", cuckooDirCheck, 30)
	registerCheck("cuckoo_pipe", cuckooPipeCheck, 30)
	registerCheck("azure", azureHostname, 30)

	// 25%
	registerCheck("display", displayCheck, 25)
	registerCheck("audio", audioDevices, 25)
	registerCheck("gpu", gpuCapabilities, 25)
	registerCheck("device_string", deviceStringCheck, 25)
	registerCheck("power_capabilities", powerCapabilitiesCheck, 25)

	// 10%
	registerCheck("gamarue", gamarueCheck, 10)
}

var vmSysDir = os.Getenv("SYSTEMROOT") + "\\System32\\"

// --- 150% ---

var vmDLLList = []string{
	"vboxhook.dll", "vboxmrxnp.dll", "vboxogl.dll", "vboxoglarrayspu.dll",
	"vboxoglcrutil.dll", "vboxoglfeedbackspu.dll", "vboxoglpackspu.dll",
	"vboxoglpassthroughspu.dll", "vboxservice.exe", "vboxtray.exe",
	"vmtoolsd.exe", "vmacthlp.exe", "vmsrvc.exe", "vmusrvc.exe",
	"vmserv.exe", "vmwaretray.exe", "vmwareuser.exe",
	"xenservice.exe", "xsvc.exe",
}

func vmDLLs() bool {
	for _, dll := range vmDLLList {
		if _, err := os.Stat(vmSysDir + dll); err == nil {
			log.Printf("[antivm] windows_dll: found %s", dll)
			return true
		}
	}
	getModuleHandle := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	if getModuleHandle.Find() != nil {
		return false
	}
	for _, dll := range []string{"VBoxHook.dll", "VBoxOGL.dll", "vmware-vmx.dll"} {
		dllName, _ := syscall.UTF16PtrFromString(dll)
		if dllName == nil {
			continue
		}
		ret, _, _ := getModuleHandle.Call(uintptr(unsafe.Pointer(dllName)))
		if ret != 0 {
			return true
		}
	}
	return false
}

func dbvmCheck() bool {
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	dbvm := ntdll.NewProc("DbvmCheck")
	return dbvm.Find() == nil
}

func vmDrivers() bool {
	scsiPaths := []string{
		`HARDWARE\DEVICEMAP\Scsi\Scsi Port 0\Scsi Bus 0\Target Id 0\Logical Unit Id 0`,
		`HARDWARE\DEVICEMAP\Scsi\Scsi Port 1\Scsi Bus 0\Target Id 0\Logical Unit Id 0`,
		`HARDWARE\DEVICEMAP\Scsi\Scsi Port 2\Scsi Bus 0\Target Id 0\Logical Unit Id 0`,
	}
	vmTags := []string{"VBOX", "VMWARE", "QEMU", "XEN", "VIRTUAL"}
	for _, path := range scsiPaths {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		val, _, err := k.GetStringValue("Identifier")
		k.Close()
		if err != nil {
			continue
		}
		upper := strings.ToUpper(val)
		for _, tag := range vmTags {
			if strings.Contains(upper, tag) {
				return true
			}
		}
	}
	return false
}

func vmMutex() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return false
	}

	vmProcs := []string{
		"vboxservice.exe", "vboxtray.exe", "vboxcontrol.exe",
		"vmtoolsd.exe", "vmwaretray.exe", "vmwareuser.exe",
		"vmupgradehelper.exe", "vmacthlp.exe",
		"xenservice.exe",
		"prl_cc.exe", "prl_tools.exe",
	}

	for {
		name := strings.ToLower(syscall.UTF16ToString(entry.ExeFile[:]))
		for _, vm := range vmProcs {
			if name == vm {
				return true
			}
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return false
}

func vmProcessesWindows() bool { return vmMutex() }

func diskSerialVM() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\Disk\Enum`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	val, _, err := k.GetStringValue("0")
	if err != nil {
		return false
	}
	upper := strings.ToUpper(val)
	for _, tag := range []string{"VBOX", "VMWARE", "QEMU", "XEN"} {
		if strings.Contains(upper, tag) {
			return true
		}
	}
	return false
}

func ivshmemCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Enum\PCI`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()

	subs, err := k.ReadSubKeyNames(0)
	if err != nil {
		return false
	}
	for _, sub := range subs {
		if strings.Contains(strings.ToLower(sub), "1af4") || strings.Contains(strings.ToLower(sub), "ivshmem") {
			return true
		}
	}
	return false
}

func handlesCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Enum\PCI`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()

	subs, _ := k.ReadSubKeyNames(0)
	vmDevices := []string{"15ad", "80ee", "1af4"}
	for _, sub := range subs {
		for _, vm := range vmDevices {
			if strings.HasPrefix(strings.ToLower(sub), vm) {
				return true
			}
		}
	}
	return false
}

func virtualProcessors() bool {
	return runtime.NumCPU() < 2
}

func hypervisorQuery() bool {
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	ntQuery := ntdll.NewProc("NtQuerySystemInformation")
	if ntQuery.Find() != nil {
		return false
	}
	// SystemHypervisorInformation = 0x9f
	var buf [256]byte
	ret, _, _ := ntQuery.Call(0x9f, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	return ret == 0
}

func acpiSignatureCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\ACPI\DSDT`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()

	subs, _ := k.ReadSubKeyNames(0)
	for _, sub := range subs {
		lower := strings.ToLower(sub)
		if strings.Contains(lower, "vbox") || strings.Contains(lower, "vmw") ||
			strings.Contains(lower, "qemu") || strings.Contains(lower, "xen") {
			return true
		}
	}
	return false
}

func firmwareTablesCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\ACPI\Firmware`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	vals, _ := k.ReadValueNames(0)
	for _, v := range vals {
		if strings.Contains(strings.ToLower(v), "vbox") ||
			strings.Contains(strings.ToLower(v), "vmware") ||
			strings.Contains(strings.ToLower(v), "qemu") {
			return true
		}
	}
	return false
}

func bootLogoCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\OEMInformation`,
		registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	logo, _, err := k.GetStringValue("Logo")
	if err != nil {
		return false
	}
	lower := strings.ToLower(logo)
	return strings.Contains(lower, "vbox") || strings.Contains(lower, "vmware")
}

func kernelObjectsCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`,
		registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()

	subs, _ := k.ReadSubKeyNames(0)
	for _, sub := range subs {
		subKey, err := registry.OpenKey(registry.LOCAL_MACHINE,
			`SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\`+sub,
			registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		desc, _, err := subKey.GetStringValue("DriverDesc")
		subKey.Close()
		if err != nil {
			continue
		}
		lower := strings.ToLower(desc)
		if strings.Contains(lower, "vmware") || strings.Contains(lower, "vbox") ||
			strings.Contains(lower, "virtual") || strings.Contains(lower, "qemu") {
			return true
		}
	}
	return false
}

func nvramCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\RESOURCEMAP`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()
	subs, _ := k.ReadSubKeyNames(0)
	for _, sub := range subs {
		if strings.Contains(strings.ToLower(sub), "vbox") ||
			strings.Contains(strings.ToLower(sub), "vmware") {
			return true
		}
	}
	return false
}

func edidCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Enum\DISPLAY`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()

	subs, _ := k.ReadSubKeyNames(0)
	for _, sub := range subs {
		lower := strings.ToLower(sub)
		if strings.Contains(lower, "vmware") || strings.Contains(lower, "vbox") ||
			strings.Contains(lower, "qemu") {
			return true
		}
	}
	return false
}

func clockTiming() bool {
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	ntQuery := ntdll.NewProc("NtQuerySystemInformation")
	if ntQuery.Find() != nil {
		return false
	}
	// SystemTimeAdjustmentInformation = 0x94
	var buf [32]byte
	ret, _, _ := ntQuery.Call(0x94, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	return ret == 0
}

// --- 95% ---

func devicesPCICheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Enum\PCI`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	defer k.Close()

	subs, _ := k.ReadSubKeyNames(0)
	vmVendors := []string{"15ad", "80ee", "1af4"}
	for _, sub := range subs {
		parts := strings.SplitN(sub, "\\", 2)
		if len(parts) > 0 {
			vendor := strings.ToLower(parts[0])
			for _, v := range vmVendors {
				if strings.HasPrefix(vendor, v) {
					return true
				}
			}
		}
	}
	return false
}

// --- 90% ---

func virtualRegistryCheck() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Sandboxie`, registry.QUERY_VALUE)
	if err == nil {
		k.Close()
		return true
	}

	k2, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\Sandboxie`, registry.QUERY_VALUE)
	if err == nil {
		k2.Close()
		return true
	}
	return false
}

func cpuHeuristicCheck() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	isIntel := kernel32.NewProc("IsProcessorFeaturePresent")
	if isIntel.Find() != nil {
		return false
	}
	// PF_XMMI64_INSTRUCTIONS_AVAILABLE = 10
	// Check if SSE2 is properly reported
	ret, _, _ := isIntel.Call(10)
	return ret == 0
}

// --- 75% ---

func vpcInvalidCheck() bool {
	// Virtual PC check (32-bit only via CPUID)
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	vpc := ntdll.NewProc("VpcGuest")
	return vpc.Find() == nil
}

// --- 45% ---

func clockCheck() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getTickCount := kernel32.NewProc("GetTickCount64")
	t1, _, _ := getTickCount.Call()
	t2, _, _ := getTickCount.Call()
	// If time doesn't advance normally, could be VM
	return t2-t1 > 100
}

// --- 30% ---

func cuckooDirCheck() bool {
	paths := []string{
		"C:\\Cuckoo", "C:\\analysis", "C:\\Analyzer",
		os.Getenv("TEMP") + "\\cuckoa",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func cuckooPipeCheck() bool {
	// Check for Cuckoo pipe names
	pipes := []string{`\\.\pipe\cuckoo`, `\\.\pipe\analyzer`}
	for _, p := range pipes {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// --- 25% ---

func displayCheck() bool {
	user32 := syscall.NewLazyDLL("user32.dll")
	getSystemMetrics := user32.NewProc("GetSystemMetrics")
	horz, _, _ := getSystemMetrics.Call(0)
	vert, _, _ := getSystemMetrics.Call(1)
	return (horz < 800 && vert < 600) || horz == 0
}

func audioDevices() bool {
	winmm := syscall.NewLazyDLL("winmm.dll")
	out, _, _ := winmm.NewProc("waveOutGetNumDevs").Call()
	in, _, _ := winmm.NewProc("waveInGetNumDevs").Call()
	return out == 0 && in == 0
}

func gpuCapabilities() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\0000`,
		registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	desc, _, err := k.GetStringValue("DriverDesc")
	if err != nil {
		return false
	}
	lower := strings.ToLower(desc)
	for _, s := range []string{"virtual", "vmware", "vbox", "qemu", "hyper-v"} {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func deviceStringCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\Disk\Enum`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	vals, _ := k.ReadValueNames(0)
	for _, v := range vals {
		val, _, err := k.GetStringValue(v)
		if err != nil {
			continue
		}
		lower := strings.ToLower(val)
		if strings.Contains(lower, "vbox") || strings.Contains(lower, "qemu") ||
			strings.Contains(lower, "vmware") {
			return true
		}
	}
	return false
}

func powerCapabilitiesCheck() bool {
	powrprof := syscall.NewLazyDLL("powrprof.dll")
	callNtPowerInfo := powrprof.NewProc("CallNtPowerInformation")
	if callNtPowerInfo.Find() != nil {
		return false
	}
	// SystemPowerInformation = 12
	var buf [32]byte
	ret, _, _ := callNtPowerInfo.Call(12, 0, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return ret == 0
}

// --- 10% ---

func gamarueCheck() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	productID, _, err := k.GetStringValue("ProductId")
	if err != nil {
		return false
	}
	// Gamarue VM detection: checks for specific product ID patterns
	return strings.HasPrefix(productID, "00330-") || strings.HasPrefix(productID, "00331-")
}

// --- 100% ---

func wineDetection() bool {
	ntdll, err := syscall.LoadLibrary("ntdll.dll")
	if err != nil {
		return false
	}
	defer syscall.FreeLibrary(ntdll)

	_, err = syscall.GetProcAddress(ntdll, "wine_get_unix_file_name")
	return err == nil
}
