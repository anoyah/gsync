# gsync

持续监听当前 Git 仓库的上游分支，自动从服务器拉取更新。默认在前台运行，每轮完成后等待 30 秒再次检查，Ctrl+C 停止。使用 Go 标准库和系统 Git，采用跨平台轮询。

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
gsync                         # 持续监听，自动拉取
gsync --interval 5s            # 每轮完成后等待 5 秒
gsync --once                  # 只检查并拉取一轮
gsync --dry-run               # 只 Fetch 并预览一轮，不整合提交
gsync --verbose --timeout 30s  # 详细日志；每条 Git 命令超时 30 秒
gsync --version
gsync --help
```

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

没有跨进程仓库互斥锁，请勿对同一仓库同时运行多个 gsync。与其他 Git 程序并发修改时仍有竞争窗口。Git hooks、签名与 credential helper 由 Git 执行；标准输入继承，标准输出和错误被捕获。硬件密钥及交互式认证尚未全面验证，建议使用已有 SSH agent 或 credential helper。程序在前台持续运行，不自动安装系统后台服务。

HTTP(S) URL 的 userinfo 凭据会脱敏，请勿在 URL 路径、查询参数或自定义输出中放置令牌。报告问题时附系统、Git/Go 版本、脱敏后的 `gsync --verbose` 日志和复现步骤。

## 退出码与开发

`--once` / `--dry-run` 的结果码：0 成功，1 未分类错误或 Git 不可用，2 参数错误，3 非 Git 仓库，4 不安全状态或分叉，5 上游配置异常，6 Fetch 失败，7 整合失败，9 超时或取消。内部引擎保留旧版 Push 失败码 8，当前 CLI 不执行 Push。

监听模式遇到可恢复问题会继续等待，不立即返回错误码；Ctrl+C 或 SIGTERM 退出返回 9。不可恢复错误返回对应退出码。

```sh
go test ./...
go vet ./...
```

测试使用临时本地 bare remote，无需公网，包含监听重试、修改和分叉保护、恢复后拉取、取消等待。代码面向 macOS、Linux、Windows，目前仅在 macOS 实际运行，其他平台运行验证待 CI 执行。

## 发布包

```sh
go run ./cmd/release --version v0.2.0
```

生成 `dist/v0.2.0/`：macOS / Linux / Windows 的 amd64、arm64 六个平台包和 `SHA256SUMS`。Unix 平台为 tar.gz，Windows 为 zip，包含二进制和 README。关闭 CGO，注入版本并移除本地源码路径；同版本输出存在时拒绝覆盖。`--out <目录>` 可指定输出父目录。

macOS 校验：`shasum -a 256 -c SHA256SUMS`；Linux：`sha256sum -c SHA256SUMS`；Windows：`Get-FileHash <压缩包> -Algorithm SHA256` 与清单比对。解压后将二进制放入 PATH，运行 `gsync --version` 检查。

GitHub Actions 的 `Build release archives` 可手动输入版本并下载 artifact，不自动公开发布。交叉编译通过不等同于各平台运行验证。
