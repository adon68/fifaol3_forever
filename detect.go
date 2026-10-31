package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

const clientExe = "fifazf.exe"

var skipDirs = map[string]bool{
	"_cache": true, "data": true, "screenshot": true, "instance0": true,
	"xigncode": true, "$recycle.bin": true, "system volume information": true,
	"windows": true, "winsxs": true, "node_modules": true, "appdata": true,
	"programdata": true, "recovery": true, "msocache": true,
}

func looksLikeClient(dir string) bool {
	if dir == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(dir, clientExe))
	return err == nil && !fi.IsDir()
}

func findClientDir(dir string) (string, bool) {
	if looksLikeClient(dir) {
		return dir, true
	}
	var found []string
	var walk func(p string, depth int)
	walk = func(p string, depth int) {
		if depth > 3 {
			return
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || skipDirs[strings.ToLower(e.Name())] {
				continue
			}
			sub := filepath.Join(p, e.Name())
			if looksLikeClient(sub) {
				found = append(found, sub)
				continue
			}
			walk(sub, depth+1)
		}
	}
	walk(dir, 0)
	if len(found) == 0 {
		return "", false
	}

	sort.Strings(found)
	return found[len(found)-1], true
}

func autoDetectClient() string {
	if env := os.Getenv("FO3_GAME_DIR"); env != "" {
		if d, ok := findClientDir(env); ok {
			return d
		}
	}
	drives := fixedDrives()
	names := []string{"FO3ID", "FO3TH", "FIFA Online 3", "FIFAOnline3", "Games", "Game", "游戏"}
	for _, drive := range drives {
		for _, n := range names {
			base := filepath.Join(drive+`\`, n)
			if _, err := os.Stat(base); err != nil {
				continue
			}
			if d, ok := findClientDir(base); ok {
				return d
			}
		}
	}

	for _, drive := range drives {
		entries, err := os.ReadDir(drive + `\`)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || skipDirs[strings.ToLower(e.Name())] {
				continue
			}
			if d, ok := findClientDir(filepath.Join(drive+`\`, e.Name())); ok {
				return d
			}
		}
	}
	return ""
}

func fixedDrives() []string {
	var out []string
	for c := 'C'; c <= 'Z'; c++ {
		d := string(c) + ":"
		if _, err := os.Stat(d + `\`); err == nil {
			out = append(out, d)
		}
	}
	return out
}

func (a *app) describeClient(dir string) string {
	fi, err := os.Stat(filepath.Join(dir, "data_0.big"))
	size := ""
	if err == nil {
		size = fmt.Sprintf(a.tr("dir.data"), float64(fi.Size())/(1<<20))
	}
	players := a.tr("dir.dbLater")
	if st, err := os.Stat(a.fifadbPath()); err == nil {
		players = fmt.Sprintf(a.tr("dir.dbReady"), float64(st.Size())/(1<<20))
	}
	return a.tr("dir.found") + size + players
}

func (a *app) detectInitial() {
	a.mu.Lock()
	saved := a.cfg.GameDir
	a.mu.Unlock()
	if saved != "" {
		if d, ok := findClientDir(saved); ok {
			a.applyGameDir(d, false)
			return
		}
	}
	go a.autoFind()
}

func (a *app) autoFind() {
	a.mu.Lock()
	prepared := a.prepared
	a.mu.Unlock()
	if !prepared {

	}
	a.logf("%s", a.tr("log.searching"))
	start := time.Now()
	dir := autoDetectClient()
	if dir == "" {
		a.logf("%s", a.tr("log.notFound"))
		return
	}
	a.logf(a.tr("log.found"), dir, time.Since(start).Seconds())
	a.applyGameDir(dir, false)
}

func (a *app) pickGameDir() {
	dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:                      a.tr("pick.title"),
		ButtonLabel:                a.tr("pick.button"),
		Directory:                  true,
		CreateDirectories:          false,
		TreatPackagesAsDirectories: true,
	})
	if err != nil {
		a.logf(a.tr("log.pickFail"), err)
		return
	}
	if len(dirs) == 0 {
		return
	}
	a.setGameFromDropped(dirs[0])
}

func (a *app) setGameFromDropped(path string) {
	path = strings.TrimSpace(strings.Trim(path, `"`))
	if path == "" {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		a.logf(a.tr("log.badDrop"), path)
		return
	}
	base := path
	if !fi.IsDir() {
		base = filepath.Dir(path)
	}
	dir, ok := findClientDir(base)
	if !ok {
		a.logf(a.tr("log.noExe"), clientExe, base)
		a.logf("%s", a.tr("log.noExeHint"))
		return
	}
	a.applyGameDir(dir, true)
}

func (a *app) applyGameDir(dir string, announce bool) {
	info := a.describeClient(dir)
	a.mu.Lock()
	a.gameDir = dir
	a.cfg.GameDir = dir
	a.gameInfo = info
	a.mu.Unlock()
	a.saveConfig()
	if announce || true {
		a.logf(a.tr("log.gameDir"), dir)
	}
}

func (a *app) gamePath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gameDir
}

func (a *app) cacheDir() string {
	return filepath.Join(a.gamePath(), "_cache", "games", "fo3id")
}

func (a *app) fifadbPath() string {
	return filepath.Join(a.serverDir, "gamedata", "fifadb.sqlite")
}
