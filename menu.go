package main

const menuText = `
  GSYNC · Git 自动拉取
  只拉取，不提交、不推送、不 Rebase。

  gsync tui       打开交互面板
  gsync watch     前台持续监听
  gsync --once    立即拉取一次
  gsync start     启动后台服务（仅 macOS）
  gsync stop      停止后台服务（仅 macOS）
  gsync status    查看后台状态（仅 macOS）
  gsync logs      查看后台日志（仅 macOS）
  gsync --help    查看完整帮助

  示例：gsync watch --repo /path/to/repository --interval 5s
        gsync tui --repo /path/to/repository

`
