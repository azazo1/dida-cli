package config

import (
	"strings"

	"github.com/azazo1/dida-cli/internal/apperr"
)

// migration 描述一次配置版本跃迁.
//
// 迁移是显式的版本推进, 不是幂等的字段修补: 每个版本的语义变化都应当
// 在这里留下一段可追溯的记录.
type migration struct {
	// To 是本次迁移完成后配置文件的版本号.
	To int
	// Describe 是一句话说明, 会写进迁移日志.
	Describe string
	// Apply 执行迁移.
	Apply func(*Config) error
}

// migrations 按目标版本号升序排列.
var migrations = []migration{
	{
		To:       1,
		Describe: "初始版本, 补齐端点与超时等基础字段",
		Apply:    migrateToV1,
	},
}

// migrate 把配置提升到 CurrentVersion, 返回是否发生了实际变更.
func migrate(cfg *Config) (bool, error) {
	if cfg.Version > CurrentVersion {
		return false, apperr.Newf(apperr.KindGeneral,
			"config file version %d is newer than this binary supports (%d)", cfg.Version, CurrentVersion).
			WithHint("upgrade dida, or remove the config file to start from defaults")
	}
	changed := false
	for _, item := range migrations {
		if cfg.Version < item.To {
			if err := item.Apply(cfg); err != nil {
				return false, err
			}
			cfg.Version = item.To
			changed = true
		}
	}
	return changed, nil
}

// migrateToV1 为缺少基础字段的配置补齐默认值.
func migrateToV1(cfg *Config) error {
	defaults := Default()
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = defaults.Endpoint
	}
	if cfg.RequestTimeoutSeconds <= 0 {
		cfg.RequestTimeoutSeconds = defaults.RequestTimeoutSeconds
	}
	if cfg.CacheTTLSeconds < 0 {
		cfg.CacheTTLSeconds = defaults.CacheTTLSeconds
	}
	if strings.TrimSpace(cfg.LogLevel) == "" {
		cfg.LogLevel = defaults.LogLevel
	}
	return nil
}

// MigrationLog 返回迁移表的人类可读描述, 供 doctor 展示.
func MigrationLog() []string {
	lines := make([]string, 0, len(migrations))
	for _, item := range migrations {
		lines = append(lines, "v"+itoa(item.To)+": "+item.Describe)
	}
	return lines
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
