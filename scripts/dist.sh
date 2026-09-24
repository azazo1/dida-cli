#!/usr/bin/env bash

# 生成当前平台的发布产物.
#
# 版本号来自 PROJECT_BUILD_VERSION 环境变量, 未设置时用 scripts/build-version.sh
# 的结果. 产物命名遵循 PROJECT-VERSION-PLATFORM-ARCH.EXT 约定.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

platform="$(go env GOOS)"
arch="$(go env GOARCH)"

# 归档名使用更通用的平台与架构写法.
case "$platform" in
  darwin) platform="macos" ;;
  windows) platform="windows" ;;
  linux) platform="linux" ;;
esac
case "$arch" in
  amd64) arch="x86_64" ;;
  arm64) arch="aarch64" ;;
esac

version="${PROJECT_BUILD_VERSION:-}"
if [[ -z "$version" ]]; then
  version="v$(bash scripts/build-version.sh)"
fi

echo "构建 dida $version ($platform-$arch)"

binary="dida"
if [[ "$(go env GOOS)" == "windows" ]]; then
  binary="dida.exe"
fi

mkdir -p "dist/build"
go build -trimpath -ldflags "-s -w -X main.version=$version" -o "dist/build/$binary" ./cmd/dida

# 冒烟检查: 二进制必须能启动并报出注入的版本号.
reported="$("dist/build/$binary" version | tr -d '\r')"
if [[ "$reported" != *"\"$version\""* ]]; then
  echo "版本号校验失败: 期望 $version, 实际输出 $reported" >&2
  exit 1
fi
echo "版本号校验通过: $version"

mkdir -p dist
archive="dist/dida-${version}-${platform}-${arch}.tar.gz"
tar -czf "$archive" -C dist/build "$binary"
echo "已生成 $archive"
