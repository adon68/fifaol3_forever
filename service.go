package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type service struct {
	mu    sync.Mutex
	cmds  []*exec.Cmd
	names []string
}

var svc service

func (a *app) setStatus(s svcStatus, msg string) {
	a.mu.Lock()
	a.status = s
	a.statusMsg = msg
	a.mu.Unlock()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) startService() {
	a.mu.Lock()
	if a.status == svcRunning || a.status == svcStarting {
		a.mu.Unlock()
		return
	}
	prepared := a.prepared
	a.mu.Unlock()
	if !prepared {
		a.logf("%s", a.tr("log.waitPrep"))
		return
	}

	a.setStatus(svcStarting, "")
	if err := a.startServiceInner(); err != nil {
		a.logf(a.tr("log.startFail"), err)
		a.setStatus(svcFailed, err.Error())
		a.stopProcs()
		return
	}
	a.setStatus(svcRunning, "")
	a.logf(a.tr("log.started"), httpPort, notiPort, adminPort)
	a.mu.Lock()
	a.hostMode = ""
	a.mu.Unlock()
}

func (a *app) startServiceInner() error {

	a.killStale()

	for _, port := range []int{httpPort, notiPort, adminPort} {
		if portOpen(port) {
			return fmt.Errorf(a.tr("err.port"), port)
		}
	}

	if err := a.ensurePlayerDB(); err != nil {
		a.logf(a.tr("log.dbWarn"), err)
	}

	env := a.serviceEnv()

	if err := svc.start(a, "http_config.py", env, "443", "--tls"); err != nil {
		return err
	}
	if err := svc.start(a, "messenger.py", env); err != nil {
		return err
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if portOpen(httpPort) && portOpen(notiPort) {

			for i := 0; i < 20 && !portOpen(adminPort); i++ {
				time.Sleep(150 * time.Millisecond)
			}
			a.savePids()
			return nil
		}
		if svc.dead() {
			return errors.New(a.tr("err.exited"))
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New(a.tr("err.timeout"))
}

func (a *app) stopService() {
	a.setStatus(svcStopping, "")
	a.stopProcs()
	a.setStatus(svcStopped, "")
	a.logf("%s", a.tr("log.stopped"))
}

func (a *app) restartService() {
	a.logf("%s", a.tr("log.restart"))
	a.stopProcs()
	time.Sleep(600 * time.Millisecond)
	a.startService()
}

func (a *app) serviceEnv() []string {
	a.mu.Lock()
	game := a.gameDir
	a.mu.Unlock()

	env := os.Environ()
	set := func(k, v string) {
		env = append(env, k+"="+v)
	}

	set("FO3_LANGUAGE", "chi")
	set("FO3_ADMIN_PORT", fmt.Sprint(adminPort))
	set("PYTHONUTF8", "1")
	set("PYTHONIOENCODING", "utf-8")
	set("PYTHONUNBUFFERED", "1")
	if game != "" {

		set("FO3_ASSETS", filepath.Join(game, "_cache", "games", "fo3id"))
	}
	return env
}

func (s *service) start(a *app, script string, env []string, args ...string) error {
	python := a.runtimeExe
	if _, err := os.Stat(python); err != nil {
		return fmt.Errorf(a.tr("err.noPy"), python)
	}
	full := append([]string{"-X", "utf8", "-u", script}, args...)
	cmd := exec.Command(python, full...)
	cmd.Dir = a.serverDir
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	hideWindow(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf(a.tr("err.script"), script, err)
	}

	name := strings.TrimSuffix(script, ".py")
	s.mu.Lock()
	s.cmds = append(s.cmds, cmd)
	s.names = append(s.names, name)
	s.mu.Unlock()
	a.logf(a.tr("log.scriptUp"), script, cmd.Process.Pid)

	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			a.logf("[%s] %s", name, sc.Text())
		}
	}()
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		for i, c := range s.cmds {
			if c == cmd {
				s.cmds = append(s.cmds[:i], s.cmds[i+1:]...)
				s.names = append(s.names[:i], s.names[i+1:]...)
				break
			}
		}
		left := len(s.cmds)
		s.mu.Unlock()
		if err != nil && left > 0 {
			a.logf(a.tr("log.scriptExit"), script, err)
		}

		if left > 0 {
			go a.stopProcs()
			a.setStatus(svcFailed, statusExit)
		}
	}()
	return nil
}

func (s *service) dead() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cmds) == 0
}

func (a *app) stopProcs() {
	svc.mu.Lock()
	cmds := append([]*exec.Cmd(nil), svc.cmds...)
	svc.cmds = nil
	svc.names = nil
	svc.mu.Unlock()

	for _, c := range cmds {
		if c.Process != nil {
			_ = c.Process.Kill()
		}
	}
	for _, c := range cmds {
		done := make(chan struct{})
		go func(c *exec.Cmd) {
			_ = c.Wait()
			close(done)
		}(c)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}

	a.savePidsOf(nil)
	time.Sleep(300 * time.Millisecond)
}

func (a *app) pidFile() string { return filepath.Join(a.dataDir, "pids.json") }

func (a *app) savePids() {
	svc.mu.Lock()
	pids := make([]int, 0, len(svc.cmds))
	for _, c := range svc.cmds {
		if c.Process != nil {
			pids = append(pids, c.Process.Pid)
		}
	}
	svc.mu.Unlock()
	a.savePidsOf(pids)
}

func (a *app) savePidsOf(pids []int) {
	b, _ := json.Marshal(pids)
	_ = os.WriteFile(a.pidFile(), b, 0o644)
}

func (a *app) killStale() {
	b, err := os.ReadFile(a.pidFile())
	if err != nil {
		return
	}
	var pids []int
	if json.Unmarshal(b, &pids) != nil {
		return
	}
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		p, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := p.Kill(); err == nil {
			a.logf(a.tr("log.stale"), pid)
		}
	}
	a.savePidsOf(nil)
	time.Sleep(300 * time.Millisecond)
}

func (a *app) ensurePlayerDB() error {
	db := a.fifadbPath()
	if st, err := os.Stat(db); err == nil && st.Size() > 0 {
		return nil
	}
	game := a.gamePath()
	if game == "" {
		return errors.New("还没有指定游戏目录")
	}
	if _, err := os.Stat(filepath.Join(game, "data", "data_0.big")); err != nil {
		if _, err := os.Stat(filepath.Join(game, "data_0.big")); err != nil {
			return fmt.Errorf("客户端里没有 data_0.big（应在 %s）", filepath.Join(game, "data"))
		}
	}

	a.logf("%s", a.tr("log.extractDB"))
	_ = os.MkdirAll(filepath.Join(a.serverDir, "gamedata"), 0o755)

	cmd := exec.Command(a.runtimeExe, "-X", "utf8", "-u", "extract_fifadb.py", game, db)
	cmd.Dir = a.serverDir
	cmd.Env = a.serviceEnv()
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			a.logf("[extract] %s", line)
		}
	}
	if err != nil {
		return err
	}
	if st, err := os.Stat(db); err != nil || st.Size() == 0 {
		return errors.New("抽取完成但没有生成 fifadb.sqlite")
	}
	a.logf(a.tr("log.dbReady"), db)
	return nil
}
