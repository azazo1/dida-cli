$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $false

# 生成当前平台的发布产物, 规则与 scripts/dist.sh 一致.

$root = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $root
try {
    $platform = (go env GOOS)
    $arch = (go env GOARCH)

    switch ($platform) {
        "darwin" { $platform = "macos" }
        "windows" { $platform = "windows" }
        "linux" { $platform = "linux" }
    }
    switch ($arch) {
        "amd64" { $arch = "x86_64" }
        "arm64" { $arch = "aarch64" }
    }

    $version = $env:PROJECT_BUILD_VERSION
    if (-not $version) {
        $version = "v$(& 'scripts/build-version.ps1' | Out-String).Trim()"
    }

    Write-Host "构建 dida $version ($platform-$arch)"

    $binary = "dida.exe"
    New-Item -ItemType Directory -Force -Path "dist/build" | Out-Null
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "dist/build/$binary" ./cmd/dida
    if ($LASTEXITCODE -ne 0) { throw "构建失败" }

    $reported = (& "dist/build/$binary" version | Out-String)
    if ($reported -notmatch [regex]::Escape("`"$version`"")) {
        throw "版本号校验失败: 期望 $version, 实际输出 $reported"
    }
    Write-Host "版本号校验通过: $version"

    New-Item -ItemType Directory -Force -Path "dist" | Out-Null
    $archive = "dist/dida-$version-$platform-$arch.zip"
    if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive -Force }
    Compress-Archive -LiteralPath "dist/build/$binary" -DestinationPath $archive
    Write-Host "已生成 $archive"
} finally {
    Pop-Location
}
