package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed all:assets
var assets embed.FS

const (
	serverDst  = "server"
	runtimeDst = "runtime"
	stampName  = ".stamp"
)

func (a *app) setPrep(preparing bool, msg string) {
	a.mu.Lock()
	a.preparing = preparing
	a.prepMsg = msg
	a.mu.Unlock()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) prepare() {
	a.setPrep(true, a.tr("prep.working"))
	defer a.setPrep(false, "")

	stampBytes, _ := assets.ReadFile("assets/stamp.txt")
	stamp := strings.TrimSpace(string(stampBytes))

	a.mu.Lock()
	a.serverDir = filepath.Join(a.dataDir, serverDst)
	a.runtimeExe = filepath.Join(a.dataDir, runtimeDst, "python.exe")
	a.stamp = stamp
	needExtract := readStamp(a.dataDir) != stamp
	a.mu.Unlock()

	if needExtract {
		a.logf("%s", a.tr("log.unpack"))
		if err := a.extractAll(); err != nil {
			a.setPrepErr(fmt.Sprintf(a.tr("err.unpack"), err.Error()))
			a.logf(a.tr("log.unpackFail"), err)
			return
		}
		writeStamp(a.dataDir, stamp)
		a.logf("%s", a.tr("log.unpacked"))
	}

	if _, err := os.Stat(a.runtimeExe); err != nil {
		a.setPrepErr(fmt.Sprintf(a.tr("err.noPyUI"), a.runtimeExe))
		a.logf(a.tr("log.noPy"), err)
		return
	}
	a.ensureLangPack()
	a.ensureTrust()

	a.mu.Lock()
	a.prepared = true
	a.mu.Unlock()
	a.logf(a.tr("log.pyReady"), a.runtimeExe)
	a.mu.Lock()
	dir := a.gameDir
	a.mu.Unlock()
	if dir != "" {
		a.logf(a.tr("log.configured"), adminPort)
	}
}

func (a *app) setPrepErr(msg string) {
	a.mu.Lock()
	a.prepErr = msg
	a.mu.Unlock()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func readStamp(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, stampName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeStamp(dir, stamp string) {
	_ = os.WriteFile(filepath.Join(dir, stampName), []byte(stamp), 0o644)
}

func (a *app) extractAll() error {
	serverDir := filepath.Join(a.dataDir, serverDst)
	runtimeDir := filepath.Join(a.dataDir, runtimeDst)

	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		return err
	}
	err := fs.WalkDir(assets, "assets/server", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "assets/server")
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(serverDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyEmbedded(p, target)
	})
	if err != nil {
		return err
	}

	if err := fs.WalkDir(assets, "assets/stub", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, "assets/stub"), "/")
		target := filepath.Join(a.dataDir, "stub", filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyEmbedded(p, target)
	}); err != nil {
		return err
	}

	if err := seedIfMissing(filepath.Join(serverDir, "gamedata", "state.json"), `{"coaches":[],"defaultcoachid":"","teams":{}}`); err != nil {
		return err
	}

	if _, err := os.Stat(runtimeDir); err != nil {
		if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
			return err
		}
		a.logf("%s", a.tr("log.unpackPy"))
		if err := unzipEmbedded("assets/runtime.zip", runtimeDir); err != nil {
			return err
		}
	}
	return nil
}

func copyEmbedded(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b, err := assets.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, b, 0o644)
}

func seedIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func unzipEmbedded(name, dst string) error {
	b, err := assets.ReadFile(name)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return err
	}
	for _, zf := range zr.File {
		target := filepath.Join(dst, filepath.FromSlash(zf.Name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dst)) {
			return os.ErrInvalid
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *app) ensureLangPack() {
	game := a.gamePath()
	if game == "" {
		return
	}
	dst := filepath.Join(game, "_cache", "games", "fo3id", "externalAssets", "language_chi.big")
	if _, err := os.Stat(dst); err == nil {
		return
	}
	src, err := assets.ReadFile("assets/langpack/language_chi.big")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		a.logf(a.tr("log.langFail"), err)
		return
	}

	alt := filepath.Join(game, "_cache", "cdn", "externalAssets", "language_chi.big")
	if dir := filepath.Dir(alt); dir != "" {
		if st, err := os.Stat(filepath.Dir(dir)); err == nil && st.IsDir() {
			_ = os.MkdirAll(dir, 0o755)
			_ = os.WriteFile(alt, src, 0o644)
		}
	}
	a.logf(a.tr("log.langOk"), dst)
}

func (a *app) ensureTrust() {
	dir := filepath.Join(a.serverDir, "certs")
	cer := filepath.Join(dir, "ca.cer")
	if _, err := os.Stat(cer); err != nil {
		return
	}
	flag := filepath.Join(dir, ".trusted")
	if _, err := os.Stat(flag); err == nil {
		return
	}

	cmd := exec.Command("certutil", "-user", "-addstore", "Root", cer)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		a.logf(a.tr("log.certFail"), err)
		return
	}
	if strings.Contains(string(out), "0x8007") {
		a.logf(a.tr("log.certOut"), strings.TrimSpace(string(out)))
		return
	}
	_ = os.WriteFile(flag, []byte("ok"), 0o644)
	a.logf("%s", a.tr("log.certOk"))
}
