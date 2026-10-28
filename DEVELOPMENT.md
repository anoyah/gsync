# Development guide

[使用文档](README.md) · [贡献指南](CONTRIBUTING.md) · [历史方案](docs/history/initial-development-plan.md)

## 当前产品边界

gsync 是只拉取的 Git watcher，不是双向同步、自动提交工具或 Git 的替代品。不带参数只输出命令说明后退出；持续监听需显式使用 watch 或参数式监听；TUI 只在用户按键后启动任务。

公开 CLI 固定禁止 Push 和 Rebase。工作区不干净、分叉、detached HEAD 或存在未完成的 Git 操作时，停止本轮整合并提示人工处理。仅允许 Fast-forward，不自动 stash、提交、丢弃文件或修改用户 Git 配置。

历史文档中的双向同步与默认 Rebase 是被替代的设计，不能作为当前实现要求。内部引擎中的旧分支和对应测试尚保留，以当前 CLI、README 和测试保护的边界为准。

## 结构

| 路径 | 职责 |
| --- | --- |
| main.go / menu.go | 参数、子命令、退出码和无参说明 |
| internal/git | 直接调用系统 Git，超时、脱敏和状态解析 |
| internal/syncer | 顺序检查、Fetch、整合和退避重试 |
| internal/lock | 操作系统级跨进程锁；linked worktree 共用身份 |
| internal/service | macOS 用户 LaunchAgent 的安装、状态、日志和回滚 |
| internal/tui | 输入处理、异步任务、布局缓存与增量渲染 |
| cmd/release | 显式生成跨平台压缩包和校验清单 |

## 日常流程

```sh
make                 # 仅输出帮助
make check           # 格式检查、go vet、全部测试
make test-race
make build
make tui REPO=/path/to/disposable/repository
```

仅在主动执行 make fmt 时格式化源码。手工修改保持清晰的小步差异。测试中的仓库、身份、hooks 和远程必须使用临时目录，不连接个人 GitHub 账号。LaunchAgent 测试不能修改真实登录项；真实后台运行与认证需要使用维护者指定的测试仓库验证。

## 安全和性能不变量

- 使用 os/exec 的独立参数调用 Git，不拼接 shell。
- 写操作前后保留工作区与分支安全检查。空闲周期可减少只读重复扫描，不得借优化绕过写入保护。
- 仓库锁覆盖整个监听生命周期，不删除锁文件来强行解锁；服务管理使用独立操作锁。
- 网络错误和命令超时退避至 5 分钟或更长的配置间隔；本地修改按正常间隔检查，成功后重置等待。
- TUI 的观察刷新不 Fetch，不占用 watcher 的仓库锁。快照任务不重叠，后台动作不阻塞键盘和退出。
- 只有受信任的渲染代码可输出 ANSI 控制序列，Git 输出必须过滤；日志缓冲有界，内容或宽度改变时才重新换行。
- 日志不能保存 HTTP(S) userinfo 凭据。脱敏不是任意内容的秘密扫描器。
- 硬件密钥、真实 launchd 会话和不同平台行为仍需实际环境验证，不能用交叉编译成功替代运行验收。

## GitHub 与发布

公开仓库为 https://github.com/anoyah/gsync，module 和内部导入使用 github.com/anoyah/gsync，不包含 .git 后缀。上传代码后，再验证 go install github.com/anoyah/gsync@latest；本地检查不代替远程安装验证。不要提交本地 .git/config 或打包 .git 目录。

CI 仅代表配置了检查，结果以实际 GitHub Actions 运行记录为准。不会自动创建 GitHub Release。开发阶段不生成发布包；发布时显式指定版本，包内包含 MIT 许可证与第三方声明。版本变更记录维护于 CHANGELOG.md；没有公开发布的内容记为 Unreleased。

当前限制包括：后台服务仅支持 macOS、无日志轮转、gsync 锁不能约束其他 Git 客户端，旧版内部双向引擎尚未移除。新的功能或限制请同步更新 README 和 CHANGELOG。
