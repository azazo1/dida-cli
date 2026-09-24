package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/pelletier/go-toml/v2"
)

// CurrentVersion 是当前配置文件版本号.
//
// 任何字段增删都必须同时提升此版本号并在 migrate.go 中补一条迁移,
// 不允许依赖"字段缺失就取默认值"这种隐式兼容.
const CurrentVersion = 1

// DefaultEndpoint 是官方 MCP 端点.
const DefaultEndpoint = "https://mcp.dida365.com"

// Config 是配置文件的内存表示.
type Config struct {
	// Version 是配置文件版本号, 必须是文件的首个字段.
	Version int `toml:"version"`

	// ReadOnly 开启后禁止一切对账户的写操作, 命令行无法覆盖.
	ReadOnly bool `toml:"read_only"`

	// NonDestructive 开启后禁止一切破坏性操作, 但允许创建与更新这类可逆写入.
	NonDestructive bool `toml:"non_destructive"`

	// Endpoint 是 MCP 端点地址.
	Endpoint string `toml:"endpoint"`

	// DefaultProject 是未显式指定项目时使用的项目 id.
	DefaultProject string `toml:"default_project"`

	// Timezone 是日期解析与展示使用的时区名.
	Timezone string `toml:"timezone"`

	// RequestTimeoutSeconds 是单次上游请求的超时秒数.
	RequestTimeoutSeconds int `toml:"request_timeout_seconds"`

	// CacheEnabled 控制是否缓存工具目录这类低频变化数据.
	CacheEnabled bool `toml:"cache_enabled"`

	// CacheTTLSeconds 是缓存的存活秒数.
	CacheTTLSeconds int `toml:"cache_ttl_seconds"`

	// LogLevel 是默认日志级别.
	LogLevel string `toml:"log_level"`
}

// Default 返回默认配置.
func Default() *Config {
	return &Config{
		Version:               CurrentVersion,
		ReadOnly:              false,
		NonDestructive:        false,
		Endpoint:              DefaultEndpoint,
		DefaultProject:        "",
		Timezone:              "",
		RequestTimeoutSeconds: 60,
		CacheEnabled:          true,
		CacheTTLSeconds:       300,
		LogLevel:              "info",
	}
}

// Load 读取配置文件.
//
// 文件不存在时返回默认配置且不落盘, 避免读操作产生副作用.
// 文件版本低于当前版本时执行迁移并回写.
func Load() (*Config, error) {
	path := ConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, apperr.Wrap(apperr.KindGeneral, err, "read config file failed").
			WithHintf("check permissions on %s", path)
	}
	cfg := Default()
	if err := toml.Unmarshal(raw, cfg); err != nil {
		return nil, apperr.Wrap(apperr.KindGeneral, err, "parse config file failed").
			WithHintf("fix the toml syntax in %s, or delete it to fall back to defaults", path)
	}
	migrated, err := migrate(cfg)
	if err != nil {
		return nil, err
	}
	if migrated {
		if err := Save(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// Save 把配置写回磁盘, 权限收紧到 0600.
func Save(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return apperr.Wrap(apperr.KindGeneral, err, "create config directory failed")
	}
	cfg.Version = CurrentVersion
	encoded, err := toml.Marshal(cfg)
	if err != nil {
		return apperr.Wrap(apperr.KindGeneral, err, "encode config failed")
	}
	path := ConfigPath()
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return apperr.Wrap(apperr.KindGeneral, err, "write config file failed")
	}
	// WriteFile 不会修改已存在文件的权限, 这里显式收紧一次.
	if err := os.Chmod(path, 0o600); err != nil {
		return apperr.Wrap(apperr.KindGeneral, err, "tighten config file permission failed")
	}
	return nil
}

// Field 描述一个可读写的配置项, 供 config get / set / list 复用.
type Field struct {
	Name        string
	Type        string
	Description string
	Get         func(*Config) string
	Set         func(*Config, string) error
}

// Fields 返回全部可读写配置项.
func Fields() []Field {
	return []Field{
		{
			Name: "read_only", Type: "bool",
			Description: "只读模式, 开启后拒绝一切对账户的写操作",
			Get:         func(c *Config) string { return strconv.FormatBool(c.ReadOnly) },
			Set:         func(c *Config, v string) error { parsed, err := parseBool(v); c.ReadOnly = parsed; return err },
		},
		{
			Name: "non_destructive", Type: "bool",
			Description: "非破坏性模式, 开启后拒绝删除类操作, 但允许创建与更新",
			Get:         func(c *Config) string { return strconv.FormatBool(c.NonDestructive) },
			Set:         func(c *Config, v string) error { parsed, err := parseBool(v); c.NonDestructive = parsed; return err },
		},
		{
			Name: "endpoint", Type: "string",
			Description: "官方 MCP 端点地址",
			Get:         func(c *Config) string { return c.Endpoint },
			Set:         func(c *Config, v string) error { c.Endpoint = strings.TrimSpace(v); return nil },
		},
		{
			Name: "default_project", Type: "string",
			Description: "未显式指定项目时使用的项目 id",
			Get:         func(c *Config) string { return c.DefaultProject },
			Set:         func(c *Config, v string) error { c.DefaultProject = strings.TrimSpace(v); return nil },
		},
		{
			Name: "timezone", Type: "string",
			Description: "日期解析与展示使用的时区名, 留空表示跟随系统",
			Get:         func(c *Config) string { return c.Timezone },
			Set:         func(c *Config, v string) error { c.Timezone = strings.TrimSpace(v); return nil },
		},
		{
			Name: "request_timeout_seconds", Type: "int",
			Description: "单次上游请求超时秒数",
			Get:         func(c *Config) string { return strconv.Itoa(c.RequestTimeoutSeconds) },
			Set: func(c *Config, v string) error {
				parsed, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil || parsed <= 0 {
					return apperr.Usagef("request_timeout_seconds must be a positive integer, got %q", v)
				}
				c.RequestTimeoutSeconds = parsed
				return nil
			},
		},
		{
			Name: "cache_enabled", Type: "bool",
			Description: "是否缓存工具目录等低频数据",
			Get:         func(c *Config) string { return strconv.FormatBool(c.CacheEnabled) },
			Set:         func(c *Config, v string) error { parsed, err := parseBool(v); c.CacheEnabled = parsed; return err },
		},
		{
			Name: "cache_ttl_seconds", Type: "int",
			Description: "缓存存活秒数",
			Get:         func(c *Config) string { return strconv.Itoa(c.CacheTTLSeconds) },
			Set: func(c *Config, v string) error {
				parsed, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil || parsed < 0 {
					return apperr.Usagef("cache_ttl_seconds must be a non-negative integer, got %q", v)
				}
				c.CacheTTLSeconds = parsed
				return nil
			},
		},
		{
			Name: "log_level", Type: "string",
			Description: "日志级别, 取值为 debug / info / warn / error",
			Get:         func(c *Config) string { return c.LogLevel },
			Set: func(c *Config, v string) error {
				switch strings.ToLower(strings.TrimSpace(v)) {
				case "debug", "info", "warn", "warning", "error":
					c.LogLevel = strings.ToLower(strings.TrimSpace(v))
					return nil
				default:
					return apperr.Usagef("log_level must be one of debug, info, warn, error, got %q", v)
				}
			},
		},
	}
}

// FindField 按名字查找配置项.
func FindField(name string) (*Field, error) {
	normalized := strings.TrimSpace(name)
	for _, field := range Fields() {
		if field.Name == normalized {
			return &field, nil
		}
	}
	return nil, apperr.Usagef("unknown config key %q", name).
		WithHint("run: dida config list")
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, apperr.Usagef("expected a boolean value, got %q", value)
	}
}

// EnsureDir 创建配置目录并收紧权限, 供需要落盘的命令调用.
func EnsureDir() error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return apperr.Wrap(apperr.KindGeneral, err, "create config directory failed")
	}
	return os.Chmod(dir, 0o700)
}

// Describe 返回配置目录与关键文件路径, 供 config path 与 doctor 使用.
func Describe() map[string]string {
	return map[string]string{
		"config_dir":  Dir(),
		"config_file": ConfigPath(),
		"token_file":  TokenPath(),
		"cache_dir":   CacheDir(),
		"tool_cache":  ToolCachePath(),
	}
}
