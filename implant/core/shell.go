package core

import (
	"os"
	"os/exec"
	"runtime"
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
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	return cmd
}
