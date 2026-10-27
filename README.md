# gsync

在当前 Git 工作树（或子目录）运行的安全自动同步 CLI。使用 Go 标准库与系统 Git，无云服务或运行时依赖包。

## 安装与开发

需要 Git 2.27 或更新版本；源码构建需要 Go 1.22 或更新版本。

```sh
go build -o gsync .
go install .
go test ./...
go vet ./...
```

`go install .` 安装到 `GOBIN` 或 `$(go env GOPATH)/bin`，请将对应目录加入 PATH。卸载时删除安装的 `gsync` 二进制即可。当前 module 为本地名称 `gsync`；发布仓库确定后再设置真实 module path，支持远程 `go install <module>@latest`。可用 `go build -ldflags '-X main.version=0.1.0' -o gsync .` 设置版本。

## 使用

```sh
cd /path/to/repository
gsync
gsync --dry-run
gsync --no-push
gsync --no-rebase
gsync --verbose --timeout 30s
gsync --version
gsync --help
```

默认每条 Git 命令超时 2 分钟，支持 Ctrl+C / SIGTERM 取消。`-v` 等价于 `--verbose`，`-h` 等价于 `--help`。

| 状态 | 行为 |
| --- | --- |
| 完全一致 | 无需修改 |
| 本地领先 | 普通 Push 到当前分支的上游目标 |
| 远程领先 | Fast-forward |
| 双方分叉 | Rebase 后普通 Push |

**默认 Rebase 会重写本地未推送提交的 SHA，可能改变提交拓扑。** 共享提交或需要保留拓扑时使用 `--no-rebase`，分叉后会停止并提示人工处理。`--no-push` 仍会整合远程更新。

`--dry-run` 会 Fetch，因此会更新远程跟踪引用及 Git 获取信息；不执行 Merge、Rebase 或 Push。Fetch 默认不 prune。远程分支已删除时停止，不自动重建。

## 安全与恢复

未提交修改（包括未跟踪文件和子模块修改）、detached HEAD、正在进行的 Git 操作和常见锁均会阻止同步。不自动 stash、add 或 commit，不强推，不执行 `reset --hard` 或 `clean`，不修改用户 Git 配置。Push 明确指定上游分支，不受 `push.default` 影响，也不附带推送标签。

- 工作区不干净：运行 `git status`，提交修改或自行 `git stash` 后重试。
- 缺少上游：检查 `git remote -v`，用 `git branch --set-upstream-to=<remote>/<branch>` 设置。
- Rebase 冲突：尝试自动 `git rebase --abort`；失败会保留原始诊断并提示手动运行 `git status` 和 `git rebase --abort`。取消时恢复同样使用独立超时，请务必检查仓库状态。
- Push 被拒绝：检查权限并重新运行以获取远程变化，绝不自动强推。
- Fetch 失败：检查网络与现有 Git 认证；凭据管理交由系统 Git。

## 退出码

| 码 | 含义 |
| --- | --- |
| 0 | 成功或计划生成成功 |
| 1 | 未分类错误或 Git 不可用 |
| 2 | 参数错误 |
| 3 | 不属于可用 Git 工作树 |
| 4 | 不安全状态或策略禁止整合 |
| 5 | 上游配置或比较异常 |
| 6 | Fetch 失败 |
| 7 | Rebase / Fast-forward 失败 |
| 8 | Push 失败 |
| 9 | 超时或取消（即使恢复也失败） |

## 平台与限制

代码面向 macOS、Linux、Windows；当前开发环境为 macOS，其他平台尚未实机验证。集成测试使用本地 bare remote，不访问公网。可在本地生成六个平台的发布包，尚未公开发布或提供后台定时同步。

没有仓库级互斥锁：请勿对同一仓库同时执行 gsync 或其他会修改历史的 Git 操作。检查和写入之间仍存在竞争窗口。Git hooks、签名及 credential helper 由 Git 运行，需自行确保可信。支持继承标准输入，标准输出和错误被捕获；优先使用现成 credential helper / SSH agent。硬件密钥、交互式密码和平台终端行为尚未全面验证。Push 不递归推送子模块，子模块应单独管理。

错误日志会隐藏 HTTP(S) URL 的 userinfo 凭据；请勿在 URL 路径、查询参数或自定义 Git 输出中放置令牌。提交 Bug 时请提供系统、Go/Git 版本、脱敏后的 `gsync --verbose` 输出，以及可用本地测试仓库复现的步骤，勿附凭据。

## 构建发布包

在项目根目录执行：

```sh
go run ./cmd/release --version v0.1.0
```

生成 `dist/v0.1.0/`，包含 macOS / Linux / Windows 的 amd64、arm64 共六个压缩包，以及 `SHA256SUMS`。macOS 和 Linux 使用 tar.gz，Windows 使用 zip；每个包包含可执行文件与 README。构建关闭 CGO、注入版本并移除本地源码路径，不依赖额外打包工具。已有同版本目录时拒绝覆盖，失败时清理中间产物。可用 `--out <目录>` 指定输出位置。

校验包文件：macOS 使用 `shasum -a 256 -c SHA256SUMS`，Linux 使用 `sha256sum -c SHA256SUMS`，Windows 可用 PowerShell `Get-FileHash <压缩包> -Algorithm SHA256` 与清单比对。解压后将可执行文件放入 PATH 目录，运行 `gsync --version` 检查。

GitHub Actions 的 `Build release archives` 工作流可手动输入版本并下载构建产物；它只生成 artifact，不自动创建 Release 或上传发布资产。跨平台编译通过不代表在所有系统上完成运行验证，运行验证以各平台 CI 结果为准。
