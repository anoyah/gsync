# gsync

持续监听当前 Git 仓库的上游分支，自动从服务器拉取更新。不带参数启动时仅输出可用命令和示例，然后退出。持续监听每轮完成等待 30 秒再次检查，Ctrl+C 停止。同步通过系统 Git 执行，采用跨平台轮询；TUI 使用轻量的 `golang.org/x/term` 处理终端状态。

**只拉取，不自动提交、不 Push、不 Rebase。** 远程更新仅通过 Fast-forward 整合；未提交文件、本地提交和分叉历史不会被自动覆盖。

## 安装

运行需要 Git 2.27+，源码构建需要 Go 1.22+：

```sh
go build -o gsync .
go install .
```

`go install .` 安装到 `GOBIN` 或 `$(go env GOPATH)/bin`，需将对应目录加入 PATH。卸载时删除安装的二进制。当前 module 为本地名称 `gsync`，远程安装地址尚未配置。

## 使用

```sh
cd /path/to/repository
gsync                         # 输出可用命令和示例后退出
gsync watch                   # 持续监听，自动拉取
gsync watch --interval 5s      # 每轮完成后等待 5 秒
gsync --once                  # 只检查并拉取一轮
gsync --dry-run               # 只 Fetch 并预览一轮，不整合提交
gsync --verbose --timeout 30s  # 详细日志；每条 Git 命令超时 30 秒
gsync --version
gsync --help
```

在其他目录管理仓库时使用 `--repo /absolute/path/to/repository`。路径含空格时加引号。

不带参数时，无需位于 Git 仓库内；终端和重定向输出均只展示可用命令并退出，不读取输入、不执行 Git 操作。需要操作时直接执行对应命令。原有仅带参数的监听方式（如 `gsync --repo /path --interval 5s`）继续兼容，推荐使用明确的 `watch` 子命令；后台服务也显式运行 `watch`。

## 终端交互界面（TUI）

```sh
gsync tui --repo /path/to/repository --interval 5s
```

在仓库内可直接运行 `gsync tui`。界面显示仓库路径、分支、上游、工作区状态、最近一次 Fetch 的提交差异、前台监听状态、后台服务状态和日志，每 2 秒刷新观察数据。打开界面只读取状态，不自动启动监听或安装后台服务。

界面采用深色配色；宽窗口显示左侧仓库概览和右侧日志，窄窗口使用紧凑布局。日志支持自动换行、行号、状态着色与滚动位置提示，底部固定快捷操作。界面颜色仅区分信息层级与运行状态，不影响同步策略。

| 按键 | 操作 |
| --- | --- |
| `w` | 启动 / 停止前台自动拉取 |
| `s` | 启动 macOS 后台服务，配置登录启动 |
| `x` | 停止后台服务，取消登录启动 |
| `r` | 刷新本地观察状态，不执行 Fetch |
| `Tab` | 切换前台 / 后台日志 |
| `k` / `j` 或上下箭头 | 滚动日志 |
| `G` | 回到最新日志 |
| `q` / Ctrl+C | 退出 TUI，安全停止界面内的前台监听 |

退出界面不停止已启动的后台服务。前台监听与后台服务共用仓库锁，切换为后台模式时先按 `w` 停止前台，再按 `s`。后台日志从原有日志文件读取，前台日志最多保留 500 行。启动、停止和刷新异步执行；窗口至少 58 列 × 18 行，缩小时显示提示，仍可按 `q` 退出。

TUI 需要真实交互式终端，不接受管道输入输出；退出时恢复终端输入模式、光标和原屏幕。TUI 内 Git 子进程不读取界面按键，禁用终端认证提示，需要预先配置非交互式认证。前台模式支持 macOS、Linux、Windows；后台操作仍仅限 macOS。TUI 不接受 `--once` / `--dry-run`，脚本或单次预览请使用原有 CLI 命令。

## macOS 后台服务

```sh
gsync start --repo /path/to/repository --interval 5s
gsync status --repo /path/to/repository
gsync logs --repo /path/to/repository --lines 100
gsync stop --repo /path/to/repository
```

在仓库目录中可省略 `--repo`。`start` 安装并加载当前用户的 LaunchAgent，关闭终端后继续运行，后续登录时自动启动；这不是登录前的系统守护进程，休眠和注销期间不会同步。后台模式仅支持 macOS，前台监听仍支持 macOS、Linux、Windows。

每个仓库对应一个服务，同一仓库的 linked worktree 共用服务身份。后台服务固定管理首次 `start` 指定的工作树；在其他 linked worktree 中执行 `status` 或 `stop` 会管理同一服务。已加载时重复 `start` 不覆盖配置。更改间隔、PATH、更新程序或切换管理的 worktree，请先 `stop` 再 `start`；移动或删除仓库前也应先停止。

服务会把当前程序复制到 `~/Library/Application Support/gsync/<仓库标识>/gsync`，避免 `go run` 临时文件消失或重建原二进制影响启动。配置位于 `~/Library/LaunchAgents/io.gsync.<仓库标识>.plist`，日志位于 `~/Library/Logs/gsync/<仓库标识>.log`。`status` 显示配置与日志路径、launchd 状态和可用的 PID / 最近退出码。异常退出由 launchd 重新启动，重启节流为 30 秒；“已加载”不保证拉取成功，请结合日志检查网络、认证和仓库状态。

`stop` 卸载服务并删除登录启动配置，保留程序副本与日志；如需完全清理，可按 `status` 展示的仓库标识手动删除对应副本目录和日志。`logs` 默认显示最后 100 行，最多读取末尾 64 KiB；日志暂未自动轮转。后台不会继承终端输入，HTTPS 交互提示被禁用，请先配置可非交互使用的 credential helper / SSH agent。服务保存启动时的 PATH，不保存令牌或 SSH agent socket。

`-v` 等价于 `--verbose`，`-h` 等价于 `--help`。`--interval` 必须大于 0，默认 `30s`；`--timeout` 默认 `2m`。启动后立即检查一次，每轮执行结束才开始等待间隔，不会重叠执行同步。检测延迟取决于间隔和 Git 执行耗时。相同状态和错误不会持续刷屏，`--verbose` 可显示每轮诊断。

默认监听当前分支的已配置上游。切换分支后，下一轮重新识别分支与上游。`--dry-run` 固定只执行一轮，会更新远程跟踪引用及 FETCH_HEAD，但不修改工作区、整合提交或推送。Fetch 不 prune、不自动获取标签；上游分支被删除时报告错误，等待修复配置。

| 仓库状态 | 监听行为 |
| --- | --- |
| 本地与远程一致 | 等待下一轮 |
| 仅远程领先 | Fast-forward 拉取 |
| 仅本地领先 | 保留本地提交，不推送 |
| 双方分叉 | 暂停本轮，人工处理后恢复 |
| 存在未提交或未跟踪文件 | 暂停本轮，保留修改，干净后自动恢复 |
| 存在 Git 操作、锁或 detached HEAD | 暂停本轮，处理后自动恢复 |
| 缺少上游、网络/认证失败或命令超时 | 提示并在下一轮重试 |
| 非 Git 仓库、Git 不可用或 Fast-forward 失败 | 退出，处理后重启 |

这版取代旧版的一次性双向同步 CLI。`--no-push` 和 `--no-rebase` 已移除，这两个安全限制现在始终启用。

## 安全与恢复

不自动 stash、add、commit 或修改 Git 配置，不强推，不执行 `reset --hard` 或 `clean`。工作区、暂存区、未跟踪文件及子模块修改均受检查。

- 工作区有修改：运行 `git status`，自行保存、提交或 stash；工作区干净后监听会恢复。
- 缺少上游：检查 `git remote -v`，用 `git branch --set-upstream-to=<remote>/<branch>` 设置。
- 双方分叉：自行决定如何整合历史；监听不会 Rebase 或 Merge 分叉，处理后下一轮继续。
- 网络或认证错误：修复现有 Git 凭据或网络，监听会重试；凭据交由系统 Git 管理。
- Ctrl+C / SIGTERM 会取消等待及当前命令。若在整合时中断，请运行 `git status` 检查状态。

gsync 使用 Git common directory 中的 `gsync.lock` 进行跨进程互斥，整个前台或后台监听期间持锁；`--once` / `--dry-run` 同样需要获取锁。同一仓库及 linked worktree 的第二个 gsync 会拒绝启动，退出码 4。进程退出或崩溃后系统释放锁；残留的锁文件是正常现象，**请勿删除锁文件来绕过正在运行的进程**。

锁只约束 gsync，与其他 Git 程序并发修改时仍有竞争窗口。Git hooks、签名与 credential helper 由 Git 执行；前台继承标准输入，标准输出和错误被捕获。硬件密钥及交互式认证尚未全面验证。后台管理操作还有独立互斥锁，避免同时 start / stop 写坏服务配置；服务安装本身仍可能遇到 launchd 权限或登录会话限制。

HTTP(S) URL 的 userinfo 凭据会脱敏，请勿在 URL 路径、查询参数或自定义输出中放置令牌。报告问题时附系统、Git/Go 版本、脱敏后的 `gsync --verbose` 日志和复现步骤。

## 退出码与开发

`--once` / `--dry-run` 的结果码：0 成功，1 未分类错误或 Git 不可用，2 参数错误，3 非 Git 仓库，4 不安全状态或分叉，5 上游配置异常，6 Fetch 失败，7 整合失败，9 超时或取消。内部引擎保留旧版 Push 失败码 8，当前 CLI 不执行 Push。

监听模式遇到可恢复问题会继续等待，不立即返回错误码；Ctrl+C 或 SIGTERM 退出返回 9。不可恢复错误返回对应退出码。

```sh
go test ./...
go vet ./...
```

测试使用临时本地 bare remote，无需公网，包含监听重试、修改和分叉保护、恢复后拉取、取消等待、跨进程锁与崩溃释放、linked worktree 互斥、服务安装回滚和日志尾部读取。TUI 测试覆盖按键、粘贴保护、终端输出过滤、前后台操作和退出取消。服务测试使用临时目录与受控的 launchctl 替身，macOS 上额外用 plutil 验证配置格式，不会安装真实登录服务。真实 launchd 加载及凭据访问需在目标仓库验证；Linux、Windows 的运行验证待 CI 执行。

## 发布包

日常开发只编译和测试，不自动打包；以下命令仅在需要发布时手动执行。

```sh
go run ./cmd/release --version v0.2.0
```

生成 `dist/v0.2.0/`：macOS / Linux / Windows 的 amd64、arm64 六个平台包和 `SHA256SUMS`。Unix 平台为 tar.gz，Windows 为 zip，包含二进制和 README。关闭 CGO，注入版本并移除本地源码路径；同版本输出存在时拒绝覆盖。`--out <目录>` 可指定输出父目录。

macOS 校验：`shasum -a 256 -c SHA256SUMS`；Linux：`sha256sum -c SHA256SUMS`；Windows：`Get-FileHash <压缩包> -Algorithm SHA256` 与清单比对。解压后将二进制放入 PATH，运行 `gsync --version` 检查。

GitHub Actions 的 `Build release archives` 可手动输入版本并下载 artifact，不自动公开发布。交叉编译通过不等同于各平台运行验证。
