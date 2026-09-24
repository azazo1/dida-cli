package cli

import (
	"io"
	"strings"
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/config"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/spf13/cobra"
)

// attachAuth 挂载 auth 命令组.
func (rt *Runtime) attachAuth(root *cobra.Command) {
	group := rt.group(root, "auth", "管理官方 API 口令")

	rt.attach(group, commandSpec{
		path:      "auth login",
		summary:   "从 stdin 读取官方 API 口令并保存到本地",
		operation: schema.OpRead,
		notes: "该命令只写本地凭证文件, 不访问账户, 因此只读模式不会拦截它. " +
			"口令必须通过管道传入, 不接受命令行参数, 避免留在 shell 历史里.",
		examples: []string{
			"printf 'dp_xxxxxxxx' | dida auth login",
		},
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			started := time.Now()
			if isTerminalReader(rt.Stdin) {
				return apperr.Usage("检测到 stdin 是终端, 拒绝从终端直接输入口令").
					WithHint("改用管道: printf 'dp_xxxxxxxx' | dida auth login")
			}
			content, err := io.ReadAll(rt.Stdin)
			if err != nil {
				return apperr.Wrap(apperr.KindGeneral, err, "读取 stdin 失败")
			}
			token := firstTokenLine(string(content))
			if err := config.SaveToken(token); err != nil {
				return err
			}
			_, status, err := config.ReadTokenFile()
			if err != nil {
				return err
			}
			rt.Logger.Info("口令已保存", "path", status.Path)
			return rt.Emit("auth login", started, map[string]any{
				"saved":   true,
				"path":    status.Path,
				"preview": status.Preview,
				"mode":    status.Mode,
			}, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "auth logout",
		summary:   "删除本地保存的口令",
		operation: schema.OpRead,
		notes:     "该命令只删本地凭证文件, 不影响账号本身.",
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			started := time.Now()
			path := config.TokenPath()
			if err := config.ClearToken(); err != nil {
				return err
			}
			rt.Logger.Info("本地口令已清除", "path", path)
			return rt.Emit("auth logout", started, map[string]any{
				"removed": true,
				"path":    path,
			}, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "auth status",
		summary:   "查看当前口令来源与状态",
		operation: schema.OpRead,
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			started := time.Now()
			status, err := rt.tokenStatus()
			if err != nil {
				return err
			}
			return rt.Emit("auth status", started, status, output.Meta{})
		},
	})
}

// tokenStatus 汇总口令状态, 不返回错误, 缺失也视为一种可报告的状态.
func (rt *Runtime) tokenStatus() (map[string]any, error) {
	_, status, err := config.ReadTokenFile()
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"available": status.Available,
		"source":    status.Source,
		"path":      status.Path,
	}
	if status.Preview != "" {
		payload["preview"] = status.Preview
	}
	if status.SavedAt != "" {
		payload["saved_at"] = status.SavedAt
	}
	if status.Mode != "" {
		payload["mode"] = status.Mode
	}
	if status.Permissive {
		payload["permissive"] = true
		payload["warning"] = "口令文件权限过宽, 建议 chmod 600"
	}
	if env := strings.TrimSpace(envToken()); env != "" {
		payload["available"] = true
		payload["source"] = string(config.TokenSourceEnv)
		payload["preview"] = config.RedactToken(env)
		payload["env_override"] = true
	}
	return payload, nil
}

// attachConfig 挂载 config 命令组.
func (rt *Runtime) attachConfig(root *cobra.Command) {
	group := rt.group(root, "config", "查看与修改本地配置")

	rt.attach(group, commandSpec{
		path:      "config path",
		summary:   "输出配置目录与关键文件路径",
		operation: schema.OpRead,
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			return rt.Emit("config path", time.Now(), config.Describe(), output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "config list",
		summary:   "列出全部可读写配置项及其当前取值",
		operation: schema.OpRead,
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			items := make([]map[string]any, 0, len(config.Fields()))
			for _, field := range config.Fields() {
				items = append(items, map[string]any{
					"key":         field.Name,
					"value":       field.Get(rt.Config),
					"type":        field.Type,
					"description": field.Description,
				})
			}
			return rt.EmitData("config list", time.Now(), items, output.Meta{
				Notes: "配置文件: " + config.ConfigPath(),
			})
		},
	})

	rt.attach(group, commandSpec{
		path:       "config get",
		summary:    "读取单个配置项",
		operation:  schema.OpRead,
		positional: []schema.Param{{Name: "key", Required: true, Description: "配置项名"}},
		examples:   []string{"dida config get read_only"},
		run: func(rt *Runtime, cmd *cobra.Command, _ map[string]any) error {
			key := cmdPositional(cmd, 0)
			field, err := config.FindField(key)
			if err != nil {
				return err
			}
			return rt.Emit("config get", time.Now(), map[string]any{
				"key":   field.Name,
				"value": field.Get(rt.Config),
				"type":  field.Type,
			}, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "config set",
		summary:   "修改单个配置项并写回配置文件",
		operation: schema.OpRead,
		notes:     "该命令只写本地配置文件, 不访问账户. 写回后新配置对后续命令立即生效.",
		positional: []schema.Param{
			{Name: "key", Required: true, Description: "配置项名"},
			{Name: "value", Required: true, Description: "配置项取值"},
		},
		examples: []string{
			"dida config set read_only true",
			"dida config set non_destructive true",
			"dida config set timezone Asia/Shanghai",
		},
		run: func(rt *Runtime, cmd *cobra.Command, _ map[string]any) error {
			started := time.Now()
			key := cmdPositional(cmd, 0)
			value := cmdPositional(cmd, 1)
			field, err := config.FindField(key)
			if err != nil {
				return err
			}
			if err := field.Set(rt.Config, value); err != nil {
				return err
			}
			if err := config.Save(rt.Config); err != nil {
				return err
			}
			rt.Logger.Info("配置已更新", "key", field.Name, "path", config.ConfigPath())
			return rt.Emit("config set", started, map[string]any{
				"key":   field.Name,
				"value": field.Get(rt.Config),
				"path":  config.ConfigPath(),
			}, output.Meta{})
		},
	})
}

// attachDoctor 挂载 doctor 命令.
func (rt *Runtime) attachDoctor(root *cobra.Command) {
	rt.attach(root, commandSpec{
		path:      "doctor",
		summary:   "检查配置, 口令与上游连通性",
		operation: schema.OpRead,
		tools:     []string{"initialize", "tools/list"},
		notes: "退出码反映健康状态: 0 表示全部检查通过, 非 0 表示第一项失败检查对应的退出码. " +
			"命令本身的执行结果始终是成功信封, 具体结论看 data.checks.",
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			started := time.Now()
			report := rt.runDoctor()
			rt.exitCode = report.exitCode
			return rt.Emit("doctor", started, report.payload, output.Meta{
				Notes: report.notes,
			})
		},
	})
}

type doctorReport struct {
	payload  map[string]any
	notes    string
	exitCode int
}

func (rt *Runtime) runDoctor() doctorReport {
	checks := make([]map[string]any, 0, 6)
	exitCode := 0
	addCheck := func(name string, ok bool, detail string, code int) {
		checks = append(checks, map[string]any{"name": name, "ok": ok, "detail": detail})
		if !ok && exitCode == 0 {
			exitCode = code
		}
	}

	_, tokenStatus, tokenErr := config.ReadTokenFile()
	tokenOK := tokenStatus.Available
	detail := "未找到口令"
	if tokenOK {
		detail = "来源 " + string(tokenStatus.Source) + ", 预览 " + tokenStatus.Preview
	}
	if tokenStatus.Permissive {
		detail += ", 权限过宽 " + tokenStatus.Mode
	}
	if tokenErr != nil {
		detail = tokenErr.Error()
	}
	addCheck("token", tokenOK, detail, 3)

	toolCount := 0
	readCount := 0
	writeCount := 0
	destructiveCount := 0
	upstreamDetail := "跳过, 没有可用口令"
	upstreamOK := false
	if tokenOK {
		client, err := rt.Client(cmdContext(rt))
		if err != nil {
			upstreamDetail = err.Error()
		} else if tools, err := client.Tools(cmdContext(rt)); err != nil {
			upstreamDetail = err.Error()
		} else {
			upstreamOK = true
			toolCount = len(tools)
			for _, tool := range tools {
				readOnly, declared := tool.ReadOnly()
				if !declared || !readOnly {
					writeCount++
				} else {
					readCount++
				}
				if tool.Destructive() && (!declared || !readOnly) {
					destructiveCount++
				}
			}
			upstreamDetail = "端点 " + client.Endpoint() + " 可用"
		}
	}
	addCheck("upstream", upstreamOK, upstreamDetail, 5)

	payload := map[string]any{
		"healthy": exitCode == 0,
		"checks":  checks,
		"version": rt.Version,
		"config": map[string]any{
			"dir":             config.Dir(),
			"file":            config.ConfigPath(),
			"version":         rt.Config.Version,
			"endpoint":        rt.Config.Endpoint,
			"timezone":        rt.Config.Timezone,
			"timeout_seconds": rt.Config.RequestTimeoutSeconds,
			"cache_enabled":   rt.Config.CacheEnabled,
			"migrations":      config.MigrationLog(),
		},
		"guard": rt.Mode.Describe(),
		"tools": map[string]any{
			"total":       toolCount,
			"read":        readCount,
			"write":       writeCount,
			"destructive": destructiveCount,
		},
	}
	return doctorReport{
		payload:  payload,
		notes:    "只读模式与非破坏性模式来自配置或环境变量或命令行参数, 任一为真即生效",
		exitCode: exitCode,
	}
}

// attachVersion 挂载 version 命令.
func (rt *Runtime) attachVersion(root *cobra.Command) {
	rt.attach(root, commandSpec{
		path:      "version",
		summary:   "输出版本信息",
		operation: schema.OpRead,
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			return rt.Emit("version", time.Now(), map[string]any{
				"version": rt.Version,
				"go":      goVersion(),
			}, output.Meta{})
		},
	})
}

func firstTokenLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}
