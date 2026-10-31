# FO3 Launcher

A Windows program that starts the local FIFA Online 3 service and the game from one window. Point it at your own client folder. You do not install Python yourself.

The window language is English or Chinese. Choose **en** or **cn** from the dropdown in the title bar. The choice is saved.

This program is free software under the **GNU General Public License v3.0**. See [LICENSE](LICENSE). You may use, share, and modify it. If you distribute it, keep this license and make the source available.

中文说明：[README.zh-CN.md](README.zh-CN.md)

Not affiliated with EA, Spearhead, or Garena. The game client is not included. Use a copy you already have.

## Thanks

The local service inside this launcher comes from:

<https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server>

Thank you to that project and to the upstream commits it is built on, including:

| Commit | What it added |
| --- | --- |
| [`a9cb5b7`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/a9cb5b7) | The original local service (soloplayer24, with Claude) |
| [`fc2db53`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/fc2db53) | GPL-3.0 license |
| [`c443884`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/c443884) | Formations, manager and player levels, scout trades |
| [`c59a417`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/c59a417) | One-click start script and the `start_all` fix |
| [`708b25f`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/708b25f) | All-season packs, the trade pool, and mini face sync |
| [`f45b760`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/f45b760) | Card prices from rating and grade |
| [`54d1e71`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/54d1e71) | Team color bonus |
| [`28fd936`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/28fd936), [`c6ef9f4`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/c6ef9f4) | Larger season-pack pools |
| [`1f336c5`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/1f336c5) | Upgrade success bonus aligned with the Upgrade tab |
| [`e75eebc`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/e75eebc) | Contributor name update |
| [`afaa316`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/afaa316) | README update for the recent service features |

## What you need

- Windows 10 or later
- Your own FIFA Online 3 client folder, the one that contains `fifazf.exe`

## How to use

1. Run `FO3Launcher.exe`.
2. Drop the client folder onto the window, or click **Choose folder**, or click **Search**.
3. Click **Start service**. The first start builds a player database from your client. That can take one or two minutes.
4. Click **Open admin** when you want the management page: `http://127.0.0.1:8088/`
5. Click **Start game** after the service is running.
6. Use **en** / **cn** in the title bar to switch the window language.
7. **Open log file** opens `launcher.log`. **Open data folder** opens the folder where settings and the service files are kept.

Closing the window stops the service. If a game is running, it loses the connection. The tray menu can show the window, open the admin page, or quit.

Settings and service files are stored in `%AppData%\FO3启动器\`:

- `config.json` — game folder and window language
- `launcher.log` — the log
- `server\` — the local service and its data
- `runtime\` — the bundled Python runtime, after the first unpack

## Download

The Windows program is `FO3Launcher.exe` on the [Windows release](https://github.com/adon68/fifaol3_forever/releases/tag/windows). Each push to `main` builds it again and replaces that file.

## Build

From this directory:

```powershell
go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=1.0.0" -o dist/FO3Launcher.exe .
```

`-H=windowsgui` builds a Windows program with no console window. `-X main.version=1.0.0` is the version shown in the window.

`assets/runtime.zip` is included, so the exe carries a portable Python runtime.

Go 1.27.3 or newer, as declared in `go.mod`.
