[private]
default:
    @just --list

# 构建当前平台的二进制, 版本号显示为 dev-build.
build:
    go build -o bin/dida ./cmd/dida

# 生成当前平台的发布产物, 版本号自动带上 commit 短 hash.
[macos]
dist:
    PROJECT_BUILD_VERSION="v$(bash scripts/build-version.sh)" bash scripts/dist.sh

# 生成当前平台的发布产物, 版本号自动带上 commit 短 hash.
[linux]
dist:
    PROJECT_BUILD_VERSION="v$(bash scripts/build-version.sh)" bash scripts/dist.sh

# 生成当前平台的发布产物, 版本号自动带上 commit 短 hash.
[windows]
[script('powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File')]
dist:
    $ErrorActionPreference = 'Stop'
    $env:PROJECT_BUILD_VERSION = "v$(& 'scripts/build-version.ps1' | Out-String).Trim()"
    & 'scripts/dist.ps1'
    if ($LASTEXITCODE) { exit $LASTEXITCODE }

# 当前构建应当显示的版本号.
version:
    @bash scripts/build-version.sh

# 运行全部测试.
test:
    go test ./... -count=1

# 静态检查.
vet:
    go vet ./...

# 检查格式是否符合 gofmt, 只报告不修改.
fmt-check:
    @unformatted=$(shell gofmt -l .); \
    if [ -n "$$unformatted" ]; then \
        echo "以下文件未格式化:"; \
        echo "$$unformatted"; \
        exit 1; \
    fi

# 整理依赖.
tidy:
    go mod tidy

# 直接运行 CLI.
# just run view today --compact
run *args:
    go run ./cmd/dida {{args}}

# 输出 agent 使用指南.
guide:
    @go run ./cmd/dida skill
