# GoSSH

一个仿 FinalShell 布局的 SSH 客户端，纯 Go 编写，界面用 [MyGo](https://github.com/egoist/mygo) 的原生 UI，终端是 MyGo 的 terminal 插件（底层 libghostty-vt，运行时加载，不需要 cgo）。

和 FinalShell 的区别：会话标签是竖向的，放在左下角，可以滚动、拖动排序。

## 运行

需要 Go 1.27.3+。

```sh
go tool mygo dev     # 开发模式，改代码自动重启
go tool mygo build   # 打包到 build/<平台>/
go test ./...
```

第一次 `go run` / `go test` 会把 libghostty-vt 下载到用户缓存目录；`mygo build` 打包的应用自带这个库。

用真实服务器跑集成测试：

```sh
GOSSH_TEST_SSH=user:password@127.0.0.1:22 go test -run SSH -v
```

## 目前有的功能

- 连接管理器：新建、编辑、删除主机，密码或私钥登录。主机列表存在 `<用户配置目录>/gossh/hosts.json`，密码不落盘，只存在内存里。
- 第一次连接时弹窗显示主机指纹，信任后记入 `gossh/known_hosts`；指纹变化时拒绝连接。
- 左上：当前会话服务器的 IP、运行时间、负载、CPU/内存/交换、内存占用前 5 的进程、网速和延迟曲线、磁盘。每 2 秒通过 SSH 读取一次 `/proc`，只支持 Linux 服务器。
- 左下：竖向会话列表，状态点（绿：已连接，黄：连接中，红：失败），右键菜单（重新连接、复制会话、断开、关闭），拖动排序。
- 右上：终端。断开后保留最后的画面，顶部出现“重新连接”。
- 右下：SFTP 文件浏览，左边目录树，右边文件表格，路径栏可以直接输入路径。

快捷键：Ctrl+Shift+T 连接管理器，Ctrl+Tab / Ctrl+Shift+Tab 切换会话，Ctrl+Shift+W 关闭会话。终端内复制粘贴是 Ctrl+Shift+C / Ctrl+Shift+V（macOS 是 Cmd+C / Cmd+V）。

## 代码结构

| 文件 | 内容 |
|---|---|
| `main.go` | 窗口 |
| `app.go` | 应用状态、整体布局、快捷键、会话的打开和关闭 |
| `sidebar.go` | 左侧：监控面板和竖向会话列表 |
| `mainview.go` | 右侧：终端区域和文件面板 |
| `dialogs.go` | 连接管理器、密码输入、主机指纹确认 |
| `session.go` | 一个会话：连接、终端、监控、SFTP |
| `sshconn.go` | SSH 拨号、认证、known_hosts、给终端用的 Conn |
| `monitor.go` | 服务器状态采集和解析 |
| `files.go` | SFTP 目录读取 |

图标来自 [Lucide](https://lucide.dev)（ISC 协议，见 `assets/icons/LICENSE`）。
