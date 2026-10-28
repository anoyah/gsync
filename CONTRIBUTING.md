# Contributing to gsync

感谢参与。gsync 的重点是可靠的 Git 自动拉取，TUI 是可选界面。

## 本地开发

需要 Go 1.22+、Git 2.27+。集成测试使用临时本地 bare 仓库，不需要账号或公网。

测试环境的 Git 还需支持 `git init --initial-branch`；运行时与测试工具的最低要求应分别验证。

```sh
go mod download
make check
make test-race
make build
```

没有 GNU Make 时可运行 `go test ./...`、`go vet ./...`、`go build .`；修改后运行 `gofmt`。使用 `make tui REPO=/path/to/test-repository` 检查界面。不要用重要仓库试验尚未验证的 Git 写操作。

## 提交修改

1. Bug 请提供系统、Go/Git 版本、实际命令、预期与实际行为，以及脱敏日志。
2. 较大的功能先提交 Issue 讨论范围；小修复可以直接提 PR。
3. 保持改动聚焦，更新相关文档。行为变更应添加能防止回归的测试，优先使用真实临时 Git 仓库。
4. PR 说明问题、最终行为和验证结果；界面变更可附截图，注明终端尺寸。

核心约束：公开 CLI 只允许 Fetch 和 Fast-forward，不自动 add、commit、stash、Rebase 或 Push，不丢弃本地修改。优化不得绕过写入前的安全检查与仓库锁。内部引擎仍有早期双向同步实现及测试，不代表当前产品支持这些行为；不要无意间将其暴露给 CLI。

不要提交凭据、真实私有仓库日志、本地二进制、覆盖率文件或发布包。不要在未经明确执行发布命令的开发流程中生成发布包。测试不得安装真实 LaunchAgent、修改用户 Git 配置或依赖个人账号。

提交 PR 即表示你有权贡献这些内容，并同意贡献按仓库的 MIT 许可证分发。交流请尊重他人，反馈围绕事实和代码，避免人身攻击与泄露私人信息。
