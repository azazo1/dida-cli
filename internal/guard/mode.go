// Package guard 实现本地的两道安全闸门.
//
// 闸门的意义在于: agent 拿着一个能改真实数据的 CLI, 需要有办法把它的手绑住.
// 两道闸门都只允许"收紧", 命令行无法把配置里打开的闸门关掉, 否则这道闸
// 对 agent 就形同虚设.
package guard

import (
	"os"
	"strings"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/config"
	"github.com/azazo1/dida-cli/internal/official"
)

// Gate 描述单个闸门的开关状态与来源.
type Gate struct {
	// Enabled 表示闸门是否生效.
	Enabled bool `json:"enabled"`
	// Source 记录是谁打开了闸门, 便于 user 判断该去哪里关掉它.
	Source string `json:"source,omitempty"`
}

// Mode 是当前生效的闸门组合.
type Mode struct {
	ReadOnly       Gate   `json:"read_only"`
	NonDestructive Gate   `json:"non_destructive"`
	ConfigPath     string `json:"config_path,omitempty"`
}

// Operation 描述一次操作的性质.
type Operation struct {
	// ReadOnly 为真表示该操作不修改账户数据.
	ReadOnly bool
	// Destructive 为真表示该操作会删除或不可逆地改变数据.
	Destructive bool
}

// Resolve 按"配置或环境变量或命令行参数, 任一为真即生效"的规则算出当前闸门.
func Resolve(cfg *config.Config, flagReadOnly, flagNonDestructive bool) Mode {
	return Mode{
		ReadOnly:       resolveGate(cfg.ReadOnly, config.EnvReadOnly, flagReadOnly),
		NonDestructive: resolveGate(cfg.NonDestructive, config.EnvNonDestructive, flagNonDestructive),
		ConfigPath:     config.ConfigPath(),
	}
}

func resolveGate(configValue bool, envName string, flagValue bool) Gate {
	sources := make([]string, 0, 3)
	if configValue {
		sources = append(sources, "config")
	}
	if envTruthy(envName) {
		sources = append(sources, "env:"+envName)
	}
	if flagValue {
		sources = append(sources, "flag")
	}
	return Gate{Enabled: len(sources) > 0, Source: strings.Join(sources, "+")}
}

// envTruthy 判断环境变量是否显式打开.
func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Check 依据操作性质判断是否放行.
//
// action 是给人和 agent 看的动作描述, 例如 "dida task create".
func (m Mode) Check(op Operation, action string) error {
	if m.ReadOnly.Enabled && !op.ReadOnly {
		return apperr.ReadOnly(action+" 会对账户产生写入, 而只读模式已开启").
			WithHintf("只读模式来自 %s, 如需放行请修改 %s 中的 read_only, 或去掉 --read-only 参数", m.ReadOnly.Source, m.ConfigPath)
	}
	if m.NonDestructive.Enabled && op.Destructive {
		return apperr.NonDestructive(action+" 属于破坏性操作, 而非破坏性模式已开启").
			WithHintf("非破坏性模式来自 %s, 如需放行请修改 %s 中的 non_destructive, 或去掉 --non-destructive 参数", m.NonDestructive.Source, m.ConfigPath)
	}
	return nil
}

// CheckTool 依据官方工具注解判断是否放行.
//
// 注解缺失时按最保守的方式处理: 既算写操作, 也算破坏性操作.
func (m Mode) CheckTool(tool official.Tool, action string) error {
	readOnly, declared := tool.ReadOnly()
	if !declared {
		readOnly = false
	}
	return m.Check(Operation{ReadOnly: readOnly, Destructive: tool.Destructive()}, action)
}

// RequireConfirm 检查破坏性操作是否带了显式确认.
//
// 闸门与确认是两件事: 闸门是 user 给自己设的硬约束, 确认是单次操作的
// 谨慎要求. 因此破坏性操作即使闸门没开, 也默认需要 --yes.
func RequireConfirm(op Operation, action string, confirmed bool) error {
	if !op.Destructive || confirmed {
		return nil
	}
	return apperr.New(apperr.KindConfirm, action+" 属于破坏性操作, 需要显式确认").
		WithHint("先用 --dry-run 核对请求体, 确认目标无误后加上 --yes 重新执行")
}

// Describe 返回闸门状态的人类可读描述, 供 doctor 与 skill 使用.
func (m Mode) Describe() map[string]any {
	return map[string]any{
		"read_only":            m.ReadOnly.Enabled,
		"read_only_from":       m.ReadOnly.Source,
		"non_destructive":      m.NonDestructive.Enabled,
		"non_destructive_from": m.NonDestructive.Source,
	}
}

// OperationOf 把契约里的操作类型转成闸门判定用的操作性质.
func OperationOf(kind string) Operation {
	switch kind {
	case "read":
		return Operation{ReadOnly: true}
	case "destructive":
		return Operation{ReadOnly: false, Destructive: true}
	default:
		return Operation{ReadOnly: false, Destructive: false}
	}
}
