.DEFAULT_GOAL := help

GO ?= go
VERSION ?= dev
BINARY ?= gsync$(shell $(GO) env GOEXE)
DIST ?= dist
ARGS ?=
REPO ?= .
INTERVAL ?= 30s

.PHONY: help build run tui watch test test-race coverage vet fmt fmt-check check install clean release

help:
	@printf '%s\n' \
	  'gsync 开发命令' \
	  '  make build          编译当前平台程序（VERSION=dev）' \
	  '  make run            显示程序操作说明；ARGS 可传入参数' \
	  '  make tui            打开 TUI（REPO=.）' \
	  '  make watch          前台监听（REPO=. INTERVAL=30s）' \
	  '  make test           运行全部测试' \
	  '  make test-race      运行全部测试并检查数据竞争' \
	  '  make coverage       生成 coverage.out 并显示覆盖率' \
	  '  make vet            运行 go vet' \
	  '  make fmt            格式化 Go 源码' \
	  '  make fmt-check      检查格式，不修改源码' \
	  '  make check          检查格式、静态分析和测试' \
	  '  make install        安装到 GOBIN 或 GOPATH/bin' \
	  '  make clean          删除本地程序和 coverage.out，保留发布包' \
	  '  make release VERSION=v0.2.1  显式构建六个平台发布包' \
	  '' \
	  '示例：make tui REPO="/path with spaces/repository"' \
	  '      make watch REPO=/path/to/repository INTERVAL=5s' \
	  '      make run ARGS="--version"' \
	  '默认仅显示帮助，不编译、不运行、不生成发布包。'

build:
	$(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o "$(BINARY)" .

run: build
	"./$(BINARY)" $(ARGS)

tui: build
	"./$(BINARY)" tui --repo "$(REPO)" --interval "$(INTERVAL)" $(ARGS)

watch: build
	"./$(BINARY)" watch --repo "$(REPO)" --interval "$(INTERVAL)" $(ARGS)

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

fmt-check:
	@unformatted="$$(gofmt -l .)" || exit $$?; \
	if [ -n "$$unformatted" ]; then \
	  printf '%s\n' '以下文件需要格式化，请运行 make fmt：' "$$unformatted"; \
	  exit 1; \
	fi

check: fmt-check vet test

install:
	$(GO) install -trimpath -ldflags "-X main.version=$(VERSION)" .

clean:
	rm -f -- gsync gsync.exe coverage.out

# Packaging is opt-in and never part of build, check, or install.
release:
	@if [ "$(VERSION)" = dev ] || [ -z "$(VERSION)" ]; then \
	  printf '%s\n' '请显式指定发布版本，例如 make release VERSION=v0.2.1' >&2; \
	  exit 2; \
	fi
	$(GO) run ./cmd/release --version "$(VERSION)" --out "$(DIST)"
