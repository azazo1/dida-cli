// Command dida 是面向 agent 的滴答清单命令行工具.
package main

import (
	"os"

	"github.com/azazo1/dida-cli/internal/cli"
)

// 以下变量由构建时通过 -ldflags 注入.
//
// 日常开发构建不注入任何信息, 版本号显示 dev-build; 发布构建由
// scripts/build-version.sh 生成带 commit 短 hash 的版本号.
var (
	version = "dev-build"
)

func main() {
	os.Exit(cli.Execute(cli.Options{
		Args:    os.Args[1:],
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: version,
	}))
}
