package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	httpPort  = 443
	notiPort  = 28002
	adminPort = 8088

	statusExit = "process-exit"
)

type svcStatus int

const (
	svcStopped svcStatus = iota
	svcStarting
	svcRunning
	svcStopping
	svcFailed
)

type config struct {
	GameDir  string `json:"gameDir"`
	Language string `json:"language"`
	UILang   string `json:"uiLang"`
}

type app struct {
	mu   sync.Mutex
	win  *mygo.Window
	tray *mygo.Tray
	cfg  config

	dataDir    string
	serverDir  string
	runtimeExe string
	stamp      string

	preparing bool
	prepared  bool
	prepMsg   string
	prepErr   string

	status    svcStatus
	statusMsg string
	pids      []int

	gameDir  string
	gameInfo string
	gameRun  bool
	gameMsg  string
	hostMode string
	stubMode bool
	uiLang   string
	langPick string

	quit bool

	logMu   sync.Mutex
	logFile *os.File
}

func newApp() *app {
	return &app{status: svcStopped, uiLang: "cn"}
}

func (a *app) ready() {
	a.dataDir = a.appData()
	_ = os.MkdirAll(a.dataDir, 0o755)
	a.loadConfig()

	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:   a.windowTitle(),
		Width:   760,
		Height:  560,
		Content: ui.View(a.view),
	})
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if a.isBusy() {

			if !a.confirmQuit() {
				e.PreventDefault()
				return
			}
		}
		go a.shutdown()
	})
	a.setUpTray()
	a.logf(a.tr("log.ready"), version, a.dataDir)

	a.detectInitial()
	go a.prepare()
}

func (a *app) focusWindow() {
	if a.win == nil {
		return
	}
	a.win.Show()
	a.win.Focus()
}

func (a *app) windowTitle() string { return a.tr("app.title") }

func (a *app) setUILang(lang string) {
	if lang != "en" && lang != "cn" {
		return
	}
	a.mu.Lock()
	same := a.uiLang == lang
	a.uiLang = lang
	a.cfg.UILang = lang
	a.mu.Unlock()
	if same {
		return
	}
	a.saveConfig()
	if a.win != nil {
		a.win.SetTitle(a.windowTitle())
	}
	a.setUpTray()
	a.notify()
}

func (a *app) setUpTray() {
	if a.tray == nil {
		icon := trayIcon()
		tray, err := mygo.NewTray(mygo.TrayOptions{Icon: icon, ToolTip: a.windowTitle()})
		if err != nil {
			a.logf(a.tr("log.tray"), err)
			return
		}
		a.tray = tray
	}
	a.tray.SetToolTip(a.windowTitle())
	a.tray.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: a.tr("tray.show"), Click: func(*mygo.MenuItem, *mygo.Window) { a.focusWindow() }},
		{Label: a.tr("tray.admin"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openAdmin() }},
		mygo.Separator(),
		{Label: a.tr("tray.quit"), Click: func(*mygo.MenuItem, *mygo.Window) {
			go func() {
				a.shutdown()
				mygo.App.Quit()
			}()
		}},
	}))
}

func (a *app) shutdown() {
	a.mu.Lock()
	a.quit = true
	first := a.status == svcRunning || a.status == svcStarting
	a.status = svcStopping
	a.mu.Unlock()
	if first {
		a.stopProcs()
	}
	if a.tray != nil {
		a.tray.Destroy()
	}
}

func (a *app) isBusy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status == svcRunning || a.status == svcStarting
}

func (a *app) appData() string {
	if p, err := mygo.App.Path(mygo.PathAppData); err == nil && p != "" {
		return filepath.Join(p, "FO3启动器")
	}
	if p, err := os.UserConfigDir(); err == nil && p != "" {
		return filepath.Join(p, "FO3启动器")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "FO3启动器")
}

func (a *app) configPath() string { return filepath.Join(a.dataDir, "config.json") }

func (a *app) loadConfig() {
	b, err := os.ReadFile(a.configPath())
	if err != nil {
		return
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	var c config
	if json.Unmarshal(b, &c) == nil {
		a.cfg = c
		if c.Language == "" {
			a.cfg.Language = "chi"
		}
		if c.UILang == "en" || c.UILang == "cn" {
			a.uiLang = c.UILang
		}
	}
}

func (a *app) saveConfig() {
	a.mu.Lock()
	c := a.cfg
	path := a.configPath()
	a.mu.Unlock()
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		a.logf(a.tr("log.save"), err)
	}
}

func (a *app) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	line = strings.TrimRight(line, "\r\n")
	ts := time.Now().Format("15:04:05")
	for _, part := range strings.Split(line, "\n") {
		part = strings.TrimRight(part, "\r")
		if strings.TrimSpace(part) == "" {
			continue
		}
		a.appendLog(ts + "  " + part)
	}
}

func (a *app) appendLog(line string) {
	a.writeLogFile(line)
	a.notify()
}

func (a *app) writeLogFile(line string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logFile == nil {
		if a.dataDir == "" {
			return
		}
		path := filepath.Join(a.dataDir, "launcher.log")
		if st, err := os.Stat(path); err == nil && st.Size() > 4<<20 {
			_ = os.Remove(path)
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		if st, err := f.Stat(); err == nil && st.Size() == 0 {
			_, _ = f.WriteString("\xEF\xBB\xBF")
		}
		a.logFile = f
	}
	_, _ = a.logFile.WriteString(line + "\n")
}

func (a *app) notify() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

func portOpen(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 350*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func (a *app) openAdmin() {
	url := fmt.Sprintf("http://127.0.0.1:%d/", adminPort)
	if !portOpen(adminPort) {
		a.logf(a.tr("log.adminDown"), url)
	}
	if err := mygo.Shell.OpenExternal(url); err != nil {
		a.logf(a.tr("log.browser"), err)
	}
}

func (a *app) logFilePath() string {
	return filepath.Join(a.dataDir, "launcher.log")
}

func (a *app) openLogFile() {
	path := a.logFilePath()
	if path == "" || a.dataDir == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		_ = os.WriteFile(path, []byte{}, 0o644)
	}
	a.openPath(path)
}

func (a *app) openPath(path string) {
	if path == "" {
		return
	}
	if err := mygo.Shell.OpenPath(path); err != nil {
		a.logf(a.tr("log.openFail"), path, err)
	}
}

func (a *app) confirmQuit() bool {
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Type:    mygo.MessageQuestion,
		Title:   a.tr("quit.title"),
		Message: a.tr("quit.message"),
		Detail:  a.tr("quit.detail"),
		Buttons: []string{a.tr("quit.ok"), a.tr("quit.cancel")},
	})
	if err != nil {
		return true
	}
	return res.Button == 0
}
