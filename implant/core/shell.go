package core

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func shellPath() string {
	switch runtime.GOOS {
	case "windows":
		return "cmd.exe"
	default:
		if sh, ok := os.LookupEnv("SHELL"); ok && sh != "" {
			return sh
		}
		for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		return "/bin/sh"
	}
}

func shellCommand() *exec.Cmd {
	cmd := exec.Command(shellPath())
	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, "TERM=") {
			filtered = append(filtered, e)
		}
	}
	filtered = append(filtered, "TERM=xterm-256color")
	cmd.Env = filtered
	return cmd
}
