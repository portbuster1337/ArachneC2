//go:build antivm

package core

import (
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
)

type vmCheck struct {
	name      string
	detect    func() bool
	certainty int
}

// allChecks populated by init() with platform-appropriate techniques
var allChecks []vmCheck

func init() {
	registerCheck("hypervisor_cpu", hypervisorCPU, 100)
	registerCheck("bochs_cpu", bochsCPU, 100)
	registerCheck("cpuid_signature", cpuidSignature, 95)
	registerCheck("cpu_brand_vm", cpuBrandVM, 95)
	registerCheck("thread_mismatch", threadMismatch, 50)
	registerCheck("thread_count", threadCount, 35)
	registerCheck("container_env", containerEnv, 30)
	registerCheck("dockerenv", dockerEnv, 30)
	registerCheck("azure_hostname", azureHostname, 30)

	registerCheck("mac_vmware", macVMware, 20)
	registerCheck("mac_virtualbox", macVirtualBox, 20)
	registerCheck("mac_qemu", macQEMU, 20)
	registerCheck("mac_xen", macXen, 20)
}

const detectionThreshold = 50

func registerCheck(name string, fn func() bool, certainty int) {
	allChecks = append(allChecks, vmCheck{name: name, detect: fn, certainty: certainty})
}

func DetectVM() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[antivm] FATAL: %v", r)
		}
	}()

	sort.Slice(allChecks, func(i, j int) bool {
		return allChecks[i].certainty > allChecks[j].certainty
	})

	var score int
	var hits []string
	for _, c := range allChecks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[antivm] panic in %s: %v", c.name, r)
				}
			}()
			if c.detect() {
				score += c.certainty
				hits = append(hits, fmt.Sprintf("%s(+%d)", c.name, c.certainty))
				log.Printf("[antivm] +%d%% %s (total: %d)", c.certainty, c.name, score)
			}
		}()
	}

	if score > 100 {
		score = 100
	}

	log.Printf("[antivm] score %d/%d (%d techniques)", score, detectionThreshold, len(hits))

	if score >= detectionThreshold {
		log.Printf("[antivm] threshold met, exiting")
		os.Exit(0)
	}
	log.Printf("[antivm] below threshold, continuing")
}

// Cross-platform checks

var vmwarePrefixes = []string{
	"00:50:56", "00:0c:29", "00:05:69", "00:1c:14",
	"00:3c:7d", "00:0f:4b",
}

var vboxPrefixes   = []string{"08:00:27"}
var qemuPrefixes   = []string{"52:54:00"}
var xenPrefixes    = []string{"00:16:3e"}

func macMatch(prefixes []string) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		mac := iface.HardwareAddr.String()
		for _, p := range prefixes {
			if strings.HasPrefix(strings.ToLower(mac), strings.ToLower(p)) {
				return true
			}
		}
	}
	return false
}

func macVMware() bool    { return macMatch(vmwarePrefixes) }
func macVirtualBox() bool { return macMatch(vboxPrefixes) }
func macQEMU() bool       { return macMatch(qemuPrefixes) }
func macXen() bool        { return macMatch(xenPrefixes) }

func threadCount() bool {
	return runtime.NumCPU() < 2
}

func threadMismatch() bool {
	n := runtime.NumCPU()
	// Most modern CPUs have >=4 threads
	return n < 4
}

func containerEnv() bool {
	for _, e := range []string{"container", "DOCKER", "KUBERNETES", "LXC"} {
		if os.Getenv(e) != "" {
			return true
		}
	}
	return false
}

func dockerEnv() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}

func hypervisorCPU() bool {
	if runtime.GOARCH != "amd64" {
		return false
	}
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/cpuinfo")
		if err != nil {
			return false
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "hypervisor") {
				return true
			}
		}
	}
	return false
}

func cpuBrandVM() bool {
	if runtime.GOARCH != "amd64" || runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	content := string(data)
	for _, s := range []string{"QEMU", "KVM", "VirtualBox", "VMware", "Bochs",
		"KVMKVMKVM", "Microsoft Hv", "VBoxVBoxVBox",
		"VMwareVMware", "XenVMMXenVMM", "ACRNACRNACRN"} {
		if strings.Contains(content, s) {
			return true
		}
	}
	return false
}

func bochsCPU() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	content := readFile("/proc/cpuinfo")
	return strings.Contains(content, "Bochs") || strings.Contains(content, "bochs")
}

func cpuidSignature() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	content := readFile("/proc/cpuinfo")
	for _, sig := range []string{
		"KVMKVMKVM", "Microsoft Hv", "prl hyperv",
		"VBoxVBoxVBox", "VMwareVMware", "XenVMMXenVMM",
		"ACRNACRNACRN",
	} {
		if strings.Contains(content, sig) {
			return true
		}
	}
	return false
}

func azureHostname() bool {
	hostname, err := os.Hostname()
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(hostname), "internal.cloudapp.net")
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

