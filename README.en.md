# gsync

[中文文档](README.md) · [GitHub](https://github.com/anoyah/gsync) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [MIT License](LICENSE)

A small, pull-only Git watcher. It fetches your current branch's configured upstream and applies remote changes only when they can be fast-forwarded. It never automatically commits, stashes, rebases or pushes through the public CLI.

Early-stage software: macOS has local runtime coverage; Linux and Windows have CI workflows whose results should be checked before relying on those platforms. Background service management currently supports macOS only.

## Build and use

Requires Go 1.22+ to build and Git 2.27+ at runtime. Clone the repository, then:

```sh
go build -o gsync .
go install .
gsync                                      # print available operations and exit
gsync watch --repo /path/to/repo --interval 30s
gsync --once --repo /path/to/repo
gsync --dry-run --repo /path/to/repo
gsync tui --repo /path/to/repo
```

The module path is `github.com/anoyah/gsync` (without `.git`). After this code is uploaded to the public repository, install remotely with:

```sh
go install github.com/anoyah/gsync@latest
```

Remote availability depends on the code actually published on GitHub; local changes are not uploaded automatically.

The optional TUI opens without starting a watcher: use `w` to toggle foreground watching, `s` / `x` to start / stop a macOS background service, and `q` to exit. Exiting the TUI stops its foreground watcher but leaves an independent background service running.

```sh
gsync start --repo /path/to/repo
gsync status --repo /path/to/repo
gsync logs --repo /path/to/repo
gsync stop --repo /path/to/repo
```

macOS `start` installs a user LaunchAgent that starts at login, not before login. `stop` removes its login-start configuration. Prepare noninteractive Git authentication first.

## Safety and limitations

- Dirty worktrees, detached HEAD, unfinished operations and divergent histories prevent automatic integration.
- A shared repository lock excludes other gsync instances, including linked worktrees. Other Git clients are outside this lock.
- Dry-run still fetches and updates remote-tracking information.
- Git hooks and credential helpers run as the current user; use trusted repositories and tools.
- Network failures back off up to five minutes, or the configured interval if longer. Successful checks reset the delay.
- No log rotation yet. Launchd and hardware-key authentication need validation in your own environment.

See the Chinese README for exit codes, terminal controls, authentication and service storage details.

## Development

```sh
make check
make test-race
make build
```

No public accounts are needed for integration tests. `make` alone prints help; ordinary development never creates release archives. Packaging is explicit: `make release VERSION=v0.2.1`. This creates local artifacts only and does not publish anything.

Licensed under MIT. Dependencies retain their respective licenses. Please use private reporting for security-sensitive issues.
