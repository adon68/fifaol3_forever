//go:build windows

package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) probeHost() int {
	a.prepare()
	if !a.prepared {
		fmt.Println("server not prepared:", a.prepErr)
		return 1
	}
	game := a.gamePath()
	if game == "" {
		fmt.Println("no game folder")
		return 1
	}
	exe := filepath.Join(game, "fifazf.exe")
	killImage("fifazf.exe")
	time.Sleep(300 * time.Millisecond)

	cmd := exec.Command(exe, a.gameArgs()...)
	cmd.Dir = game
	cmd.Env = append(os.Environ(), "__COMPAT_LAYER=RunAsInvoker")
	if err := cmd.Start(); err != nil {
		fmt.Println("start failed:", err)
		return 1
	}
	pid := cmd.Process.Pid

	var h uintptr
	var err error
	for i := 0; i < 50; i++ {
		if h, err = openProcess(pid); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		fmt.Println("OpenProcess failed:", err)
		return 1
	}

	var out strings.Builder
	out.WriteString("== APSEODESK172 in client memory ==\n")
	if ok, err := patchShield(h); true {
		out.WriteString(fmt.Sprintf("shield patch: ok=%v err=%v\n", ok, err))
	}
	base, size := imageRange(h)
	out.WriteString(fmt.Sprintf("image base=0x%X size=0x%X\n", base, size))

	needles := map[string][]byte{
		"ascii":       []byte(apiHost),
		"utf16le":     utf16le(apiHost),
		"utf16be":     utf16be(apiHost),
		"ascii-lower": []byte("apseodesk"),
	}
	for name, n := range needles {
		hits := findInRegion(h, base, size, n)
		out.WriteString(fmt.Sprintf("\n-- %s: %d hit(s)\n", name, len(hits)))
		for i, addr := range hits {
			if i >= 4 {
				out.WriteString("   ...\n")
				break
			}
			start := addr - 48
			dump, err := readMemory(h, start, 160)
			if err != nil {
				out.WriteString(fmt.Sprintf("   0x%X unreadable: %v\n", addr, err))
				continue
			}
			out.WriteString(fmt.Sprintf("   0x%X\n", addr))
			out.WriteString("   hex: " + hex.EncodeToString(dump) + "\n")
			out.WriteString("   txt: " + printable(dump) + "\n")
			w := decodeUTF16(dump)
			out.WriteString("   utf16: " + printable([]byte(w)) + "\n")
		}
	}

	for _, name := range []string{"ascii", "utf16le"} {
		regions := readableRegions(h, false)
		total := 0
		for _, rg := range regions {
			total += len(findInRegion(h, rg[0], rg[1], needles[name]))
		}
		out.WriteString(fmt.Sprintf("\nheap %s: %d hit(s)\n", name, total))
	}

	_ = os.WriteFile(filepath.Join(a.dataDir, "probe.txt"), []byte(out.String()), 0o644)
	fmt.Println("probe written to", filepath.Join(a.dataDir, "probe.txt"))
	killImage("fifazf.exe")
	return 0
}

func utf16be(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}

func decodeUTF16(b []byte) string {
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(b[i]) | rune(b[i+1])<<8
		if r < 32 || r > 0x7E {
			sb.WriteByte('.')
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func printable(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 32 && c < 127 {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}
