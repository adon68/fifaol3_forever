package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

var (
	colBg      = ui.Hex("#0f1620")
	colPanel   = ui.Hex("#18232f")
	colPanel2  = ui.Hex("#1d2a37")
	colLine    = ui.Hex("#2b3a49")
	colText    = ui.Hex("#e8eef4")
	colMuted   = ui.Hex("#8fa3b5")
	colGold    = ui.Hex("#f2c200")
	colGoldInk = ui.Hex("#1a1400")
	colOk      = ui.Hex("#57b14a")
	colWarn    = ui.Hex("#e0a030")
	colBad     = ui.Hex("#d4483e")
)

func applyTheme(c *ui.Context) {
	t := *ui.DarkTheme()
	t.Background = colBg
	t.Surface = colPanel
	t.SurfaceHover = colPanel2
	t.SurfacePressed = ui.Hex("#26333f")
	t.Border = colLine
	t.Text = colText
	t.TextMuted = colMuted
	t.Accent = colGold
	t.AccentHover = ui.Hex("#ffd633")
	t.AccentPressed = ui.Hex("#d8ac00")
	t.AccentText = colGoldInk
	t.Danger = colBad
	t.Success = colOk
	t.Warning = colWarn
	t.Selection = colGold.Alpha(0.35)
	t.Focus = colGold
	t.Scrollbar = ui.Hex("#35495c")
	t.Radius = 6
	t.FontSize = 14
	c.SetTheme(&t)
}

type snap struct {
	gameDir   string
	gameRun   bool
	gameMsg   string
	status    svcStatus
	statusMsg string
	adminURL  string
	prepared  bool
	preparing bool
	prepErr   string
}

func (a *app) snapshot() snap {
	a.mu.Lock()
	defer a.mu.Unlock()
	return snap{
		gameDir:   a.gameDir,
		gameRun:   a.gameRun,
		gameMsg:   a.gameMsg,
		status:    a.status,
		statusMsg: a.statusMsg,
		adminURL:  fmt.Sprintf("http://127.0.0.1:%d/", adminPort),
		prepared:  a.prepared,
		preparing: a.preparing,
		prepErr:   a.prepErr,
	}
}

func (a *app) view(c *ui.Context) {
	applyTheme(c)
	st := a.snapshot()

	ui.Column(c).Fill().Children(func() {
		a.header(c, st)
		ui.Column(c).Fill().Padding(18, 20).Gap(12).Children(func() {
			a.dirCard(c, st)
			a.serviceCard(c, st)
			a.gameCard(c, st)
		})
		a.footer(c)
	})
}

func (a *app) header(c *ui.Context, st snap) {
	ui.Row(c).FillWidth().Padding(20, 20).Gap(16).AlignItems(ui.Center).Background(ui.Hex("#121c27")).Children(func() {
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, "FIFA ONLINE 3").FontSize(24).Bold().TextColor(colGold).LetterSpacing(0.5)
			ui.Text(c, a.tr("app.subtitle")).FontSize(12).TextColor(colMuted)
		})
		ui.Spacer(c)
		a.langSelect(c)
		a.statusPill(c, st)
	})
}

func (a *app) langSelect(c *ui.Context) {
	a.mu.Lock()
	if a.langPick != "en" && a.langPick != "cn" {
		if a.uiLang == "en" || a.uiLang == "cn" {
			a.langPick = a.uiLang
		} else {
			a.langPick = "cn"
		}
	}
	a.mu.Unlock()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.tr("lang")).FontSize(12).TextColor(colMuted)
		sel := ui.Select(c, &a.langPick, []string{"en", "cn"}).Width(88)
		if sel.Changed() {
			a.setUILang(a.langPick)
		}
	})
}

func (a *app) statusPill(c *ui.Context, st snap) {
	col, text := colMuted, a.tr("pill.stopped")
	switch st.status {
	case svcStarting:
		col, text = colWarn, a.tr("pill.starting")
	case svcRunning:
		col, text = colOk, a.tr("pill.running")
	case svcStopping:
		col, text = colWarn, a.tr("pill.stopping")
	case svcFailed:
		col, text = colBad, a.tr("pill.failed")
	}
	ui.Row(c).Padding(5, 11).Radius(999).Background(col.Alpha(0.16)).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "●").FontSize(10).TextColor(col)
		ui.Text(c, text).FontSize(12).Bold().TextColor(col)
	})
}

func (a *app) dirCard(c *ui.Context, st snap) {
	ui.Column(c).FillWidth().Padding(14).Gap(10).Radius(8).Background(colPanel).Border(1, colLine).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("dir.title")).FontSize(14).Bold()
			if st.gameDir != "" {
				ui.Text(c, a.tr("dir.ready")).FontSize(12).TextColor(colOk)
			} else {
				ui.Text(c, a.tr("dir.dropHint")).FontSize(12).TextColor(colMuted)
			}
		})

		if st.gameDir == "" {
			zone := ui.Column(c).FillWidth().Height(104).Radius(8).Gap(6).
				Background(colPanel2.Alpha(0.7)).Border(1, colLine).
				AlignItems(ui.Center).Justify(ui.Center)
			if zone.FileDragOver() {
				zone.Border(2, colGold).Background(colGold.Alpha(0.12))
			}
			if files := zone.DroppedFiles(); len(files) > 0 {
				go a.setGameFromDropped(files[0])
			}
			zone.Children(func() {
				ui.Text(c, a.tr("dir.drop")).FontSize(14).TextColor(colText)
				ui.Text(c, a.tr("dir.dropSub")).FontSize(12).TextColor(colMuted)
			})
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, a.tr("btn.pick")).Clicked() {
					go a.pickGameDir()
				}
				if ui.Button(c, a.tr("btn.find")).Clicked() {
					go a.autoFind()
				}
			})
			return
		}

		ui.Text(c, st.gameDir).FontSize(13).Selectable().SingleLine().TextColor(colText)
		ui.Text(c, a.describeClient(st.gameDir)).FontSize(12).TextColor(colMuted)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, a.tr("btn.change")).Clicked() {
				go a.pickGameDir()
			}
			if ui.Button(c, a.tr("btn.openGame")).Clicked() {
				a.openPath(st.gameDir)
			}
			if ui.Button(c, a.tr("btn.rescan")).Clicked() {
				go a.autoFind()
			}
		})
	})
}

func (a *app) serviceCard(c *ui.Context, st snap) {
	ui.Column(c).FillWidth().Padding(14).Gap(10).Radius(8).Background(colPanel).Border(1, colLine).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("svc.title")).FontSize(14).Bold()
			ui.Text(c, a.serviceLine(st)).FontSize(12).TextColor(colMuted).SingleLine()
		})

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			running := st.status == svcRunning
			busy := st.status == svcStarting || st.status == svcStopping

			if !running {
				if ui.PrimaryButton(c, a.tr("btn.startSvc")).Clicked() && !busy {
					go a.startService()
				}
			} else {
				if ui.PrimaryButton(c, a.tr("btn.stopSvc")).Clicked() && !busy {
					go a.stopService()
				}
			}
			if ui.Button(c, a.tr("btn.admin")).Clicked() {
				a.openAdmin()
			}
			if ui.Button(c, a.tr("btn.restart")).Clicked() && !busy {
				go a.restartService()
			}
			ui.Spacer(c)
			if st.preparing {
				ui.Text(c, a.tr("prep.working")).FontSize(12).TextColor(colWarn).SingleLine()
			}
		})

		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 10).Radius(6).Background(colPanel2).Children(func() {
			ui.Text(c, a.tr("svc.adminLabel")).FontSize(12).TextColor(colMuted)
			ui.Text(c, st.adminURL).FontSize(13).Selectable().TextColor(colGold)
			ui.Spacer(c)
			ui.Text(c, a.tr("svc.adminHint")).FontSize(11).TextColor(colMuted)
		})

		if st.prepErr != "" {
			ui.Text(c, st.prepErr).FontSize(12).TextColor(colBad).Wrap()
		}
	})
}

func (a *app) serviceLine(st snap) string {
	switch st.status {
	case svcStarting:
		return a.tr("svc.starting")
	case svcRunning:
		return fmt.Sprintf(a.tr("svc.running"), adminPort)
	case svcStopping:
		return a.tr("svc.stopping")
	case svcFailed:
		switch st.statusMsg {
		case "":
			return a.tr("svc.failed")
		case statusExit:
			return a.tr("svc.exited")
		default:
			return fmt.Sprintf(a.tr("svc.failedDetail"), st.statusMsg)
		}
	default:
		return a.tr("svc.stopped")
	}
}

func (a *app) gameLine(msg string) string {
	switch msg {
	case "", "未启动":
		return a.tr("game.idle")
	case "正在启动…":
		return a.tr("game.starting")
	case "启动失败", "补丁失败":
		return a.tr("game.fail")
	case "已退出":
		return a.tr("game.exited")
	case "运行中 · 已连上服务端":
		return a.tr("game.connected")
	default:
		return msg
	}
}

func (a *app) gameCard(c *ui.Context, st snap) {
	ui.Column(c).FillWidth().Padding(14).Gap(10).Radius(8).Background(colPanel).Border(1, colLine).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("game.title")).FontSize(14).Bold()
			ui.Text(c, a.gameLine(st.gameMsg)).FontSize(12).TextColor(colMuted).SingleLine()
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			canPlay := st.gameDir != "" && st.status == svcRunning && !st.gameRun
			if ui.PrimaryButton(c, a.tr("btn.play")).Clicked() && canPlay {
				go a.startGame()
			}
			if st.gameRun && ui.Button(c, a.tr("btn.stopGame")).Clicked() {
				go a.killGame()
			}
		})
	})
}

func (a *app) footer(c *ui.Context) {
	ui.Row(c).FillWidth().Padding(8, 20).Gap(12).AlignItems(ui.Center).Background(ui.Hex("#121c27")).Children(func() {
		ui.Text(c, "v"+version).FontSize(11).TextColor(colMuted)
		ui.Spacer(c)
		if ui.Button(c, a.tr("btn.openLog")).Clicked() {
			a.openLogFile()
		}
		if ui.Button(c, a.tr("btn.openData")).Clicked() {
			a.openPath(a.dataDir)
		}
	})
}
