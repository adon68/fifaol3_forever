//go:build windows

package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const apiHost = "APSEODESK172"

var useBranchPatch bool

const (
	markerVA = 0x1530668
	callVA   = 0x5EFC2E
	callOrig = "\xe8\x5d\x80\xff\xff"
	callNew  = "\x90\x90\x90\x90\x90"

	branchVA   = 0x5E7F2A
	branchOrig = "\x0f\x84\xc5\x00\x00\x00"
	branchNew  = "\xe9\xc6\x00\x00\x00\x90"
)

var (
	modkernel32               = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess           = modkernel32.NewProc("OpenProcess")
	procReadProcessMemory     = modkernel32.NewProc("ReadProcessMemory")
	procWriteProcessMemory    = modkernel32.NewProc("WriteProcessMemory")
	procVirtualProtectEx      = modkernel32.NewProc("VirtualProtectEx")
	procVirtualQueryEx        = modkernel32.NewProc("VirtualQueryEx")
	procFlushInstructionCache = modkernel32.NewProc("FlushInstructionCache")
	procCloseHandle           = modkernel32.NewProc("CloseHandle")
)

const (
	processAllAccess = 0x1F0FFF
	memCommit        = 0x1000
	pageNoAccess     = 0x01
	pageGuard        = 0x100
	pageReadWrite    = 0x04
	memImage         = 0x1000000
)

type memoryBasicInformation struct {
	BaseAddress       uintptr
	AllocationBase    uintptr
	AllocationProtect uint32
	RegionSize        uintptr
	State             uint32
	Protect           uint32
	Type              uint32
}

var marker = []byte("XignCode System Enter Error")

func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags = 0x08000000
}

func openProcess(pid int) (uintptr, error) {
	h, _, err := procOpenProcess.Call(processAllAccess, 0, uintptr(pid))
	if h == 0 {
		return 0, fmt.Errorf("cannot open the client process (%v); try running the launcher as administrator", err)
	}
	return h, nil
}

func readMemory(h uintptr, addr uintptr, n int) ([]byte, error) {
	buf := make([]byte, n)
	var got uintptr
	ok, _, err := procReadProcessMemory.Call(h, addr,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(n), uintptr(unsafe.Pointer(&got)))
	if ok == 0 {
		return nil, err
	}
	return buf[:got], nil
}

func writeMemory(h uintptr, addr uintptr, data []byte) error {
	page := addr &^ 0xFFF
	span := (addr + uintptr(len(data)) - page + 0xFFF) &^ 0xFFF
	var old uint32
	procVirtualProtectEx.Call(h, page, span, pageReadWrite, uintptr(unsafe.Pointer(&old)))
	var written uintptr
	ok, _, err := procWriteProcessMemory.Call(h, addr,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written)))
	var back uint32
	procVirtualProtectEx.Call(h, page, span, uintptr(old), uintptr(unsafe.Pointer(&back)))
	if ok == 0 {
		return err
	}
	procFlushInstructionCache.Call(h, addr, uintptr(len(data)))
	return nil
}

func findInRegion(h uintptr, start, size uintptr, needle []byte) []uintptr {
	if size == 0 || len(needle) == 0 {
		return nil
	}
	const chunk = 1 << 20
	overlap := uintptr(len(needle) - 1)
	var hits []uintptr
	var off uintptr
	for off < size {
		n := uintptr(chunk)
		if off+n > size {
			n = size - off
		}
		data, err := readMemory(h, start+off, int(n))
		if err != nil || len(data) == 0 {
			off += n
			continue
		}
		for i := 0; ; {
			j := bytes.Index(data[i:], needle)
			if j < 0 {
				break
			}
			hits = append(hits, start+off+uintptr(i+j))
			i += j + 1
		}
		if n <= overlap {
			break
		}
		off += n - overlap
	}
	return hits
}

func imageRange(h uintptr) (uintptr, uintptr) {
	const base = 0x400000
	head, err := readMemory(h, base, 0x400)
	if err != nil || len(head) < 0x400 || head[0] != 'M' || head[1] != 'Z' {
		return 0, 0
	}
	e := *(*uint32)(unsafe.Pointer(&head[0x3C]))
	if uintptr(e)+0x58 > uintptr(len(head)) {
		return 0, 0
	}
	size := *(*uint32)(unsafe.Pointer(&head[e+24+0x38]))
	if size == 0 || size > 0x40000000 {
		return 0, 0
	}
	return base, uintptr(size)
}

func readableRegions(h uintptr, mainImageOnly bool) [][2]uintptr {
	var out [][2]uintptr
	if mainImageOnly {
		base, size := imageRange(h)
		if size == 0 {
			return nil
		}
		return [][2]uintptr{{base, size}}
	}
	var mbi memoryBasicInformation
	addr := uintptr(0x10000)
	for {
		n, _, _ := procVirtualQueryEx.Call(h, addr, uintptr(unsafe.Pointer(&mbi)), unsafe.Sizeof(mbi))
		if n == 0 || mbi.RegionSize == 0 {
			break
		}
		if mbi.State == memCommit && mbi.Protect&(pageNoAccess|pageGuard) == 0 && mbi.Type != memImage {
			out = append(out, [2]uintptr{mbi.BaseAddress, mbi.RegionSize})
		}
		next := mbi.BaseAddress + mbi.RegionSize
		if next <= addr {
			break
		}
		addr = next
	}
	return out
}

func needsHostPatch() bool {
	addrs, err := net.LookupHost(apiHost)
	if err != nil {
		return true
	}
	for _, a := range addrs {
		if a == "127.0.0.1" || a == "::1" {
			return false
		}
	}
	return true
}

func utf16le(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

func patchHostInImage(h uintptr) (patched, found int) {
	ascii := [2][]byte{[]byte(apiHost), append([]byte("127.0.0.1"), 0, 0, 0)}
	wide := [2][]byte{utf16le(apiHost), append(utf16le("127.0.0.1"), 0, 0, 0, 0, 0, 0)}
	targets := [][2][]byte{ascii, wide}

	for _, onlyImage := range []bool{true, false} {
		regions := readableRegions(h, onlyImage)
		for _, rg := range regions {
			for _, t := range targets {
				for _, addr := range findInRegion(h, rg[0], rg[1], t[0]) {
					found++
					if err := writeMemory(h, addr, t[1]); err == nil {
						patched++
					}
				}
			}
		}
		if patched > 0 {
			return patched, found
		}
	}
	return patched, found
}

func patchShield(h uintptr) (bool, error) {
	va, orig, neu := uintptr(callVA), callOrig, callNew
	if useBranchPatch {
		va, orig, neu = uintptr(branchVA), branchOrig, branchNew
	}
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		cur, err := readMemory(h, markerVA, len(marker))
		if err != nil || !bytes.Equal(cur, marker) {
			time.Sleep(40 * time.Millisecond)
			continue
		}
		at, err := readMemory(h, va, len(orig))
		if err != nil {
			return false, err
		}
		switch string(at) {
		case neu:
			return true, nil
		case orig:
			return true, writeMemory(h, va, []byte(neu))
		default:
			return false, fmt.Errorf("patch site mismatch: 0x%X holds % x (this client is not build 654420)", va, at)
		}
	}
	return false, fmt.Errorf("timed out waiting for the client to unpack (25s)")
}

func shieldVerifyBytes() (uintptr, []byte) {
	if useBranchPatch {
		return uintptr(branchVA), []byte(branchNew)
	}
	return uintptr(callVA), []byte(callNew)
}

func (a *app) stubPath() string {
	return filepath.Join(a.dataDir, "stub", "xcorona.xem")
}

func (a *app) ensureShieldStub() bool {
	src, err := os.ReadFile(a.stubPath())
	if err != nil {
		return false
	}
	game := a.gamePath()
	if game == "" {
		return false
	}
	xign := filepath.Join(game, "XignCode")
	if _, err := os.Stat(xign); err != nil {
		return false
	}
	dst := filepath.Join(xign, "xcorona.xem")
	cur, curErr := os.ReadFile(dst)
	if curErr == nil && bytes.Equal(cur, src) {
		return true
	}
	if curErr == nil {
		bak := dst + ".orig"
		if _, err := os.Stat(bak); err != nil {
			if err := os.WriteFile(bak, cur, 0o644); err != nil {
				a.logf("could not back up the original shield (continuing with the stub): %v", err)
			} else {
				a.logf("original shield backed up as XignCode\\xcorona.xem.orig")
			}
		}
	}
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		a.logf("could not install the XignCode stub (falling back to the memory patch): %v", err)
		return false
	}
	a.logf("XignCode stub installed (the shield stays offline, the game boots normally).")
	return true
}

func (a *app) restoreShieldStub() bool {
	game := a.gamePath()
	if game == "" {
		return false
	}
	dst := filepath.Join(game, "XignCode", "xcorona.xem")
	bak := dst + ".orig"
	orig, err := os.ReadFile(bak)
	if err != nil {
		return false
	}
	if err := os.WriteFile(dst, orig, 0o644); err != nil {
		return false
	}
	_ = os.Remove(bak)
	return true
}

func (a *app) gameArgs() []string {
	pid := os.Getpid()
	return []string{
		"-QUICKMATCH/SERVER=127.0.0.1:28000",
		"-QUICKMATCH/PROTOCOL=tcp",
		"-QUICKMATCH/NOTISERVER=127.0.0.1:28001",
		fmt.Sprintf("-FIFA/LAUNCHER/PID=%d", pid),
		"-PUBLISHER/GARENA/TOKEN=testtoken",
		"-LOCALE/PUBLISHING_REGION=ID",
		"-DUMPTRUCK/SERVER=127.0.0.1",
		"region", "id", "token", "testtoken",
	}
}

func (a *app) startGame() {
	game := a.gamePath()
	if game == "" {
		a.logf("no game folder set yet.")
		return
	}
	exe := filepath.Join(game, "fifazf.exe")
	if _, err := os.Stat(exe); err != nil {
		a.logf("client not found: %s", exe)
		return
	}
	a.mu.Lock()
	if a.gameRun {
		a.mu.Unlock()
		return
	}
	a.gameRun = true
	a.gameMsg = "starting..."
	statsBefore := a.serverLogSize()
	a.mu.Unlock()
	a.notify()

	killImage("fifazf.exe")
	killImage("xcoronahost.xem")
	killImage("xm.exe")
	time.Sleep(250 * time.Millisecond)

	stub := a.ensureShieldStub()
	a.mu.Lock()
	a.stubMode = stub
	a.mu.Unlock()

	cmd := exec.Command(exe, a.gameArgs()...)
	cmd.Dir = game
	cmd.Env = append(os.Environ(), "__COMPAT_LAYER=RunAsInvoker")

	a.logf("starting client: %s", exe)
	if err := cmd.Start(); err != nil {
		a.logf("failed to start the client: %v", err)
		a.mu.Lock()
		a.gameRun = false
		a.gameMsg = "start failed"
		a.mu.Unlock()
		a.notify()
		return
	}
	pid := cmd.Process.Pid
	a.logf("client started (pid %d), waiting for it to unpack (~0.5s)...", pid)

	go func() {
		_ = cmd.Wait()
		a.mu.Lock()
		a.gameRun = false
		a.gameMsg = "exited"
		a.mu.Unlock()
		a.logf("client exited.")
		a.notify()
	}()

	go a.patchClient(pid)
	go a.watchServerContact(statsBefore)
}

func (a *app) patchClient(pid int) {
	var h uintptr
	var err error
	for i := 0; i < 50; i++ {
		if h, err = openProcess(pid); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		a.logf("cannot open the client process: %v", err)
		a.mu.Lock()
		a.gameMsg = "patch failed"
		a.mu.Unlock()
		a.notify()
		return
	}
	defer func() { procCloseHandle.Call(h) }()

	a.mu.Lock()
	stub := a.stubMode
	a.mu.Unlock()

	shieldOK := false
	if stub {
		a.logf("shield runs from the stub, skipping the memory patch.")
	} else {
		var err error
		shieldOK, err = patchShield(h)
		switch {
		case err != nil:
			a.logf("skipping XignCode failed: %v", err)
			a.logf("(if the game closes immediately, this client is not build 654420)")
		case shieldOK:
			a.logf("XignCode check skipped in memory.")
		}
	}

	if !needsHostPatch() {
		a.mu.Lock()
		a.hostMode = "hosts already points to localhost"
		a.mu.Unlock()
		a.logf("%s already resolves to localhost (existing hosts entry), skipping the memory redirect.", apiHost)
	} else {
		patched := 0
		for i := 0; i < 60 && patched == 0; i++ {
			p, found := patchHostInImage(h)
			if found > 0 {
				patched = p
				a.logf("pointed %s in the client to 127.0.0.1 (%d site(s)); hosts and DNS untouched.", apiHost, p)
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
		if patched == 0 {
			a.logf("warning: %s not found in memory; the client may not reach the local server.", apiHost)
		} else {
			a.mu.Lock()
			a.hostMode = "memory redirect (no DNS change)"
			a.mu.Unlock()
		}
	}
	a.notify()

	if shieldOK {
		va, want := shieldVerifyBytes()
		for i := 0; i < 3; i++ {
			time.Sleep(2 * time.Second)
			if cur, err := readMemory(h, va, len(want)); err == nil && !bytes.Equal(cur, want) {
				if writeMemory(h, va, want) == nil {
					a.logf("the packer reverted the patch; written again.")
				}
			}
		}
	}
}

func (a *app) watchServerContact(before int64) {
	path := a.serverLogPath()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		st, err := os.Stat(path)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if st.Size() > before {
			if b, err := readFrom(path, before); err == nil && strings.Contains(b, "/info/server/locale") {
				a.logf("client reached the local server (locale request received).")
				a.mu.Lock()
				a.gameMsg = "running - connected to server"
				a.mu.Unlock()
				a.notify()
				return
			}
			before = st.Size()
		}
		time.Sleep(500 * time.Millisecond)
	}
	a.mu.Lock()
	running := a.gameRun
	a.mu.Unlock()
	if !running {
		return
	}
	a.logf("warning: no client contact within 60s.")
	a.logf("         if the game is stuck on an error dialog, try running the launcher as administrator.")
}

func (a *app) serverLogPath() string {
	return filepath.Join(a.serverDir, "captures", "http_config.log")
}

func (a *app) serverLogSize() int64 {
	if st, err := os.Stat(a.serverLogPath()); err == nil {
		return st.Size()
	}
	return 0
}

func readFrom(path string, off int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Seek(off, 0); err != nil {
		return "", err
	}
	buf := make([]byte, 1<<20)
	n, _ := f.Read(buf)
	return string(buf[:n]), nil
}

func (a *app) killGame() {
	a.logf("closing the game...")
	killImage("fifazf.exe")
	a.mu.Lock()
	a.gameRun = false
	a.gameMsg = "not running"
	a.mu.Unlock()
	a.notify()
}

func killImage(name string) {
	cmd := exec.Command("taskkill", "/F", "/IM", name)
	hideWindow(cmd)
	_ = cmd.Run()
}
