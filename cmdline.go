package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runHeadless() bool {
	parseFlags()
	mode := ""
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "--") {
			mode = a
		}
	}
	if mode == "" {
		return false
	}

	a := newApp()
	a.dataDir = a.appData()
	_ = os.MkdirAll(a.dataDir, 0o755)
	a.serverDir = filepath.Join(a.dataDir, serverDst)
	a.runtimeExe = filepath.Join(a.dataDir, runtimeDst, "python.exe")
	a.loadConfig()
	a.mu.Lock()
	a.gameDir = a.cfg.GameDir
	a.mu.Unlock()

	switch mode {
	case "--selftest":
		os.Exit(a.selfTest())
	case "--play":
		os.Exit(a.headlessPlay())
	case "--probe":
		os.Exit(a.probeHost())
	case "--service":
		a.prepare()
		if err := a.startServiceInner(); err != nil {
			fmt.Println("启动失败：", err)
			os.Exit(1)
		}
		fmt.Printf("服务已启动（HTTPS :%d，通知 :%d，后台 http://127.0.0.1:%d/）\n", httpPort, notiPort, adminPort)
		os.Exit(0)
	case "--stop":
		a.killStale()
		fmt.Println("已清理残留的服务进程。")
		os.Exit(0)
	}
	return false
}

func (a *app) headlessPlay() int {
	a.prepare()
	if !a.prepared {
		fmt.Println("服务端没准备好：", a.prepErr)
		return 1
	}
	if !portOpen(httpPort) {
		if err := a.startServiceInner(); err != nil {
			fmt.Println("启动服务失败：", err)
			return 1
		}
	}
	if a.gamePath() == "" {
		fmt.Println("配置里没有游戏目录（先用界面拖一次客户端文件夹）")
		return 1
	}

	if err := a.ensurePlayerDB(); err != nil {
		fmt.Println("球员库准备失败：", err)
	}
	a.startGame()
	for i := 0; i < 240; i++ {
		time.Sleep(500 * time.Millisecond)
		if i > 30 && !a.gameRunning() {
			break
		}
	}
	return 0
}

func (a *app) gameRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gameRun
}

func (a *app) selfTest() int {
	logPath := filepath.Join(a.dataDir, "selftest.log")
	var sb strings.Builder
	pass := true
	say := func(ok bool, format string, args ...any) {
		mark := "OK  "
		if !ok {
			mark = "FAIL"
			pass = false
		}
		line := fmt.Sprintf("[%s] ", mark) + fmt.Sprintf(format, args...)
		sb.WriteString(line + "\n")
		fmt.Println(line)
	}

	say(true, "数据目录 %s", a.dataDir)
	a.prepare()
	say(a.prepared, "服务端解包（内置 Python：%s）", a.runtimeExe)

	if !a.prepared {
		_ = os.WriteFile(logPath, []byte(sb.String()), 0o644)
		return 1
	}

	a.killStale()
	for _, p := range []int{httpPort, notiPort, adminPort} {
		if portOpen(p) {
			say(false, "端口 %d 被占用，无法自检", p)
			_ = os.WriteFile(logPath, []byte(sb.String()), 0o644)
			return 1
		}
	}

	start := time.Now()
	err := a.startServiceInner()
	say(err == nil, "启动服务（用时 %.1fs）%v", time.Since(start).Seconds(), errText(err))
	if err != nil {
		a.stopProcs()
		_ = os.WriteFile(logPath, []byte(sb.String()), 0o644)
		return 1
	}
	defer a.stopProcs()

	say(portOpen(httpPort), "HTTPS 端口 %d", httpPort)
	say(portOpen(notiPort), "通知端口 %d", notiPort)
	say(portOpen(adminPort), "后台端口 %d", adminPort)

	body, err := getInsecure(fmt.Sprintf("https://127.0.0.1:%d/info/server/locale", httpPort))
	say(err == nil && strings.Contains(body, `"language": "chi"`), "证书握手 + /info/server/locale：%s %v",
		truncate(body, 120), errText(err))

	admin, err := getInsecure(fmt.Sprintf("http://127.0.0.1:%d/", adminPort))
	say(err == nil && strings.Contains(admin, "</html>"), "后台页面可访问（%d 字节）", len(admin))
	say(strings.Contains(admin, "后台") || strings.Contains(admin, "管理"), "后台页面是中文")

	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", notiPort), 2*time.Second)
	say(err == nil, "messenger 可连接")
	if c != nil {
		c.Close()
	}

	a.stopProcs()
	time.Sleep(500 * time.Millisecond)
	say(!portOpen(httpPort), "停止服务后端口已释放")

	cmd := exec.Command("taskkill", "/F", "/IM", "fifazf.exe")
	hideWindow(cmd)
	_ = cmd.Run()

	sb.WriteString(fmt.Sprintf("结果：%v\n", map[bool]string{true: "PASS", false: "FAIL"}[pass]))
	_ = os.WriteFile(logPath, []byte(sb.String()), 0o644)
	fmt.Printf("日志：%s\n", logPath)
	if pass {
		return 0
	}
	return 1
}

func parseFlags() {
	for _, a := range os.Args[1:] {
		switch a {
		case "--branch":
			useBranchPatch = true
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return "（" + err.Error() + "）"
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func getInsecure(url string) (string, error) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}
