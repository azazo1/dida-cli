// Package config 负责配置目录定位, 配置读写与配置版本迁移.
//
// 配置目录解析优先级: DIDA_CONFIG_DIR 环境变量 > 平台默认位置.
// 平台默认位置在类 Unix 系统上遵循 XDG 约定, 即 ~/.config/dida-cli.
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	// appDirName 是配置与缓存目录名.
	appDirName = "dida-cli"

	// EnvConfigDir 用于覆盖配置目录, 测试与多账号隔离都靠它.
	EnvConfigDir = "DIDA_CONFIG_DIR"
	// EnvCacheDir 用于覆盖缓存目录.
	EnvCacheDir = "DIDA_CACHE_DIR"
	// EnvToken 用于直接提供口令, 优先级高于口令文件.
	EnvToken = "DIDA_TOKEN"
	// EnvReadOnly 用于强制开启只读模式.
	EnvReadOnly = "DIDA_READ_ONLY"
	// EnvNonDestructive 用于强制开启非破坏性模式.
	EnvNonDestructive = "DIDA_NON_DESTRUCTIVE"
	// EnvEndpoint 用于覆盖 MCP 端点, 供测试指向 mock 服务.
	EnvEndpoint = "DIDA_ENDPOINT"
)

// Dir 返回配置目录的绝对路径.
func Dir() string {
	if custom := os.Getenv(EnvConfigDir); custom != "" {
		return expand(custom)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(expand(xdg), appDirName)
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(expand(appData), appDirName)
		}
	}
	return filepath.Join(homeDir(), ".config", appDirName)
}

// CacheDir 返回缓存目录的绝对路径.
func CacheDir() string {
	if custom := os.Getenv(EnvCacheDir); custom != "" {
		return expand(custom)
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(expand(xdg), appDirName)
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(expand(local), appDirName)
		}
	}
	return filepath.Join(homeDir(), ".cache", appDirName)
}

// ConfigPath 返回配置文件的绝对路径.
func ConfigPath() string { return filepath.Join(Dir(), "config.toml") }

// TokenPath 返回口令文件的绝对路径.
func TokenPath() string { return filepath.Join(Dir(), "token") }

// ToolCachePath 返回工具目录缓存的绝对路径.
func ToolCachePath() string { return filepath.Join(CacheDir(), "tools.json") }

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

// expand 展开路径开头的波浪号.
func expand(path string) string {
	if path == "~" {
		return homeDir()
	}
	if len(path) > 2 && path[0] == '~' && (path[1] == '/' || path[1] == '\\') {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}
