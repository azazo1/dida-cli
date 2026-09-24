package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/azazo1/dida-cli/internal/skill"
	"github.com/spf13/cobra"
)

// attachSkill 挂载 skill 命令.
func (rt *Runtime) attachSkill(root *cobra.Command) {
	rt.attach(root, commandSpec{
		path:      "skill",
		summary:   "输出面向 agent 的使用指南, 相当于这个工具的 man",
		operation: schema.OpRead,
		notes: "这是全项目唯一不套 JSON 信封的命令. 指南是给人和大模型读的文档, " +
			"套进 JSON 只会增加阅读成本, 所以这里直接把 Markdown 写到 stdout. " +
			"需要结构化信息请改用 dida schema list.",
		examples: []string{
			"dida skill",
			"dida skill --compact",
		},
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			content := skill.Guide
			if rt.Compact {
				content = compactIndex(rt)
			}
			_, err := fmt.Fprint(rt.Stdout, content)
			if err != nil {
				return err
			}
			rt.Logger.Info("已输出使用指南", "compact", rt.Compact, "bytes", len(content))
			return nil
		},
	})
}

// compactIndex 从契约表生成精简命令索引.
//
// 索引由契约表实时生成而不是手写, 因此永远与真实命令一致.
func compactIndex(rt *Runtime) string {
	var builder strings.Builder
	builder.WriteString("# dida 命令索引\n\n")
	builder.WriteString("参数详情: dida schema show <command>. 完整指南: dida skill.\n\n")
	operationLabels := map[schema.Operation]string{
		schema.OpRead:        "读",
		schema.OpWrite:       "写",
		schema.OpDestructive: "删",
		schema.OpDynamic:     "动态",
	}
	for _, entry := range rt.Registry.All() {
		label := operationLabels[entry.Operation]
		if label == "" {
			label = string(entry.Operation)
		}
		fmt.Fprintf(&builder, "%-4s %-24s %s\n", label, entry.Command, entry.Summary)
	}
	builder.WriteString("\n退出码: 0 成功, 1 通用错误, 2 用法错误, 3 认证错误, 4 不存在, 5 上游错误, 6 缺少确认, 7 被闸门拦下.\n")
	return builder.String()
}

// attachSchema 挂载 schema 命令组.
func (rt *Runtime) attachSchema(root *cobra.Command) {
	group := rt.group(root, "schema", "命令契约自描述")

	rt.attach(group, commandSpec{
		path:      "schema list",
		summary:   "列出全部命令契约",
		operation: schema.OpRead,
		notes:     "输出是 agent 判断一条命令是否安全可用的权威依据: operation 表明读写属性, dry_run 表明是否可以先预览.",
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			started := time.Now()
			entries := rt.Registry.All()
			items := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				items = append(items, map[string]any{
					"command":               entry.Command,
					"summary":               entry.Summary,
					"operation":             entry.Operation,
					"tools":                 entry.Tools,
					"dry_run":               entry.SupportsDryRun,
					"confirmation_required": entry.NeedsConfirm,
				})
			}
			return rt.EmitData("schema list", started, items, output.Meta{
				Notes: "参数详情用 dida schema show <command> 查询",
			})
		},
	})

	rt.attach(group, commandSpec{
		path:      "schema show",
		summary:   "查看单条命令的完整契约",
		operation: schema.OpRead,
		positional: []schema.Param{
			{Name: "command", Required: true, Description: "命令路径的第一段"},
			{Name: "subcommand", Description: "命令路径的第二段"},
			{Name: "action", Description: "命令路径的第三段"},
		},
		examples: []string{
			"dida schema show task create",
			"dida schema show view",
		},
		run: func(rt *Runtime, cmd *cobra.Command, _ map[string]any) error {
			started := time.Now()
			name := strings.Join(cmd.Flags().Args(), " ")
			entry, err := rt.Registry.Get(name)
			if err != nil {
				return err
			}
			return rt.Emit("schema show", started, entry, output.Meta{})
		},
	})
}
