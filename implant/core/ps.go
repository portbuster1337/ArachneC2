package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	commonpb "github.com/portbuster1337/ArachneC2/protobuf/commonpb"
)

func listProcesses() []*commonpb.Process {
	switch runtime.GOOS {
	case "linux":
		return listProcessesLinux()
	default:
		return listProcessesDummy()
	}
}

func listProcessesLinux() []*commonpb.Process {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return listProcessesDummy()
	}

	var procs []*commonpb.Process
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		p := &commonpb.Process{Pid: int32(pid)}

		stat, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if len(stat) > 0 {
			var unused string
			fmt.Sscanf(string(stat), "%d %s %s", &unused, &p.Name, &unused)
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
					p.Owner = strings.TrimSpace(strings.TrimPrefix(line, "Uid:"))
				}
			}
		}

		procs = append(procs, p)
	}
	return procs
}

func listProcessesDummy() []*commonpb.Process {
	return []*commonpb.Process{
		{Pid: 1, Name: "init", Owner: "root"},
		{Pid: 2, Name: "kthreadd", Owner: "root"},
	}
}
