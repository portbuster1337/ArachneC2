package core

import (
	"bytes"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	cpb "github.com/portbuster1337/ArachneC2/protobuf/cpb"
)

func listProcesses() []*cpb.Process {
	switch runtime.GOOS {
	case "linux":
		return listProcessesLinux()
	case "windows":
		return listProcessesWindows()
	default:
		return listProcessesDummy()
	}
}

func listProcessesWindows() []*cpb.Process {
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return listProcessesDummy()
	}
	r := csv.NewReader(bytes.NewReader(out))
	records, err := r.ReadAll()
	if err != nil {
		return listProcessesDummy()
	}
	var procs []*cpb.Process
	for _, parts := range records {
		if len(parts) < 2 {
			continue
		}
		name := strings.Trim(parts[0], `"`)
		pidStr := strings.Trim(parts[1], `"`)
		pid, _ := strconv.Atoi(pidStr)
		owner := ""
		if len(parts) >= 7 {
			owner = strings.Trim(parts[6], `"`)
		}
		procs = append(procs, &cpb.Process{Pid: int32(pid), Name: name, Owner: owner})
	}
	return procs
}

func listProcessesLinux() []*cpb.Process {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return listProcessesDummy()
	}

	var procs []*cpb.Process
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		p := &cpb.Process{Pid: int32(pid)}

		stat, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if len(stat) == 0 {
			continue
		}
		parenStart := bytes.IndexByte(stat, '(')
		parenEnd := bytes.LastIndexByte(stat, ')')
		if parenStart >= 0 && parenEnd > parenStart {
			p.Name = string(stat[parenStart+1 : parenEnd])
		}

		status, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "status"))
		if len(status) > 0 {
			for _, line := range strings.Split(string(status), "\n") {
				if strings.HasPrefix(line, "Name:") {
					val := strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
					if p.Name == "" {
						p.Name = val
					}
			} else if strings.HasPrefix(line, "Uid:") {
				uidFields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "Uid:")))
				if len(uidFields) > 0 {
					p.Owner = uidFields[0]
				}
				}
			}
		}

		procs = append(procs, p)
	}
	return procs
}

func listProcessesDummy() []*cpb.Process {
	return nil
}
