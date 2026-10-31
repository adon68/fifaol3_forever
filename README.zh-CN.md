# FO3 启动器

一个 Windows 程序：在同一个窗口里启动本地服务，再启动 FIFA Online 3。把你自己的客户端文件夹交给它即可，不需要单独安装 Python。

窗口语言可以是英文或中文。在标题栏的下拉框里选择 **en** 或 **cn**，选择会保存下来。

本程序是自由软件，遵循 **GNU General Public License v3.0**。许可文本见 [LICENSE](LICENSE)。你可以自由使用、分享和修改。如果再分发，请保留该许可证并提供源代码。

English: [README.md](README.md)

与 EA、Spearhead、Garena 无关。这里不包含游戏客户端，请使用你已经拥有的副本。

## 致谢

启动器里带的本地服务来自：

<https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server>

感谢该项目，以及它所基于的上游提交，其中包括：

| 提交 | 内容 |
| --- | --- |
| [`a9cb5b7`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/a9cb5b7) | 最初的本地服务（soloplayer24，与 Claude 合作） |
| [`fc2db53`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/fc2db53) | 加上 GPL-3.0 许可证 |
| [`c443884`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/c443884) | 阵型、经理与球员等级、球探交换 |
| [`c59a417`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/c59a417) | 一键启动脚本，以及 `start_all` 的修复 |
| [`708b25f`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/708b25f) | 全赛季卡包、交易池、小头像同步 |
| [`f45b760`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/f45b760) | 按评分和等级给卡片定价 |
| [`54d1e71`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/54d1e71) | 球队颜色加成 |
| [`28fd936`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/28fd936)、[`c6ef9f4`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/c6ef9f4) | 扩大赛季卡包范围 |
| [`1f336c5`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/1f336c5) | 强化成功率与强化页说明对齐 |
| [`e75eebc`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/e75eebc) | 更新贡献者名称 |
| [`afaa316`](https://github.com/soloplayer24/FIFA-ONLINE-3-ID-Private-Server/commit/afaa316) | 更新说明文档，写入近期功能 |

## 你需要准备

- Windows 10 或更高版本
- 你自己的 FIFA Online 3 客户端文件夹，里面要有 `fifazf.exe`

## 如何使用

1. 运行 `FO3Launcher.exe`。
2. 把客户端文件夹拖进窗口，或点 **选择文件夹**，或点 **自动查找**。
3. 点 **启动服务**。第一次会从你的客户端生成球员数据库，大约需要一两分钟。
4. 需要管理页面时点 **打开后台管理**：`http://127.0.0.1:8088/`
5. 服务运行后再点 **启动游戏**。
6. 标题栏的 **en** / **cn** 用来切换窗口语言。
7. **打开日志文件** 会打开 `launcher.log`。**打开数据目录** 会打开保存设置和服务文件的文件夹。

关掉窗口会同时停止服务。如果游戏还开着，连接会断开。托盘菜单可以显示窗口、打开后台，或退出。

设置和服务文件在 `%AppData%\FO3启动器\`：

- `config.json` — 游戏目录和窗口语言
- `launcher.log` — 日志
- `server\` — 本地服务及其数据
- `runtime\` — 第一次解包后的 Python 运行时

## 下载

Windows 程序是 [Windows 发布页](https://github.com/adon68/fifaol3_forever/releases/tag/windows) 上的 `FO3Launcher.exe`。每次推送到 `main` 都会重新编译并替换这个文件。

## 编译

在本目录执行：

```powershell
go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=1.0.0" -o dist/FO3Launcher.exe .
```

`-H=windowsgui` 会生成不带控制台窗口的 Windows 程序。`-X main.version=1.0.0` 是窗口里显示的版本号。

`assets/runtime.zip` 已经包含在仓库里，编译出的 exe 会自带便携 Python。

需要 Go 1.27.3 或更高版本，以 `go.mod` 为准。
