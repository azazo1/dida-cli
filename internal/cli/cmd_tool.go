package cli

import (
	"sort"
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/guard"
	"github.com/azazo1/dida-cli/internal/official"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/spf13/cobra"
)

// attachTool 挂载 tool 命令组, 它是官方 MCP 原生工具的直通层.
func (rt *Runtime) attachTool(root *cobra.Command) {
	group := rt.group(root, "tool", "直通官方 MCP 原生工具, 不做包装")

	rt.attach(group, commandSpec{
		path:      "tool list",
		summary:   "列出全部官方工具及其读写属性",
		operation: schema.OpRead,
		tools:     []string{"tools/list"},
		notes:     "read_only 与 destructive 直接来自官方工具注解, 只读模式的判定依据就是它们.",
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			started := time.Now()
			client, err := rt.Client(cmdContext(rt))
			if err != nil {
				return err
			}
			tools, err := client.Tools(cmdContext(rt))
			if err != nil {
				return err
			}
			items := make([]map[string]any, 0, len(tools))
			for _, tool := range tools {
				readOnly, declared := tool.ReadOnly()
				items = append(items, map[string]any{
					"name":        tool.Name,
					"read_only":   readOnly && declared,
					"destructive": tool.Destructive(),
					"required":    tool.RequiredArgs(),
					"summary":     firstLine(tool.Description),
				})
			}
			sort.Slice(items, func(i, j int) bool {
				return items[i]["name"].(string) < items[j]["name"].(string)
			})
			return rt.EmitData("tool list", started, items, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "tool show",
		summary:    "查看某个官方工具的完整参数契约",
		operation:  schema.OpRead,
		tools:      []string{"tools/list"},
		positional: []schema.Param{{Name: "tool-name", Required: true, Description: "官方工具名"}},
		examples:   []string{"dida tool show create_task"},
		run: func(rt *Runtime, cmd *cobra.Command, _ map[string]any) error {
			started := time.Now()
			name := cmdPositional(cmd, 0)
			client, err := rt.Client(cmdContext(rt))
			if err != nil {
				return err
			}
			tool, err := client.ToolByName(cmdContext(rt), name)
			if err != nil {
				return err
			}
			readOnly, declared := tool.ReadOnly()
			return rt.Emit("tool show", started, map[string]any{
				"name":           tool.Name,
				"description":    tool.Description,
				"read_only":      readOnly && declared,
				"destructive":    tool.Destructive(),
				"required_args":  tool.RequiredArgs(),
				"argument_names": sortedKeys(tool.ArgNames()),
				"input_schema":   tool.InputSchema,
			}, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "tool call",
		summary:       "调用任意官方工具",
		operation:     schema.OpDynamic,
		allowBodyJSON: true,
		positional:    []schema.Param{{Name: "tool-name", Required: true, Description: "官方工具名"}},
		params: []schema.Param{
			{Name: "args-json", Type: "json", Target: "arguments", Description: "工具参数 JSON 对象, 写法同 --body-json"},
		},
		notes: "这个命令的操作性质由被调用的工具决定, 因此闸门在运行时依据官方注解判定. " +
			"参数名以 dida tool show 的输出为准, 传错不会静默成功: 官方返回的错误会被提升为退出码 5. " +
			"建议先用 --dry-run 看清楚将要发送的请求体.",
		examples: []string{
			`dida tool call list_projects --args-json '{}'`,
			`dida tool call delete_task --args-json '{"project_id":"inbox","task_id":"xxx"}' --yes`,
		},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			raw, exists := args["arguments"]
			if !exists {
				delete(args, "arguments")
				return nil
			}
			body, ok := raw.(map[string]any)
			if !ok {
				return apperr.Usage("--args-json 需要一个 JSON 对象").
					WithHint(`例如: --args-json '{"project_id":"inbox"}'`)
			}
			for key := range args {
				delete(args, key)
			}
			for key, value := range body {
				args[key] = value
			}
			return nil
		},
		preview: func(rt *Runtime, cmd *cobra.Command, args map[string]any) map[string]any {
			return map[string]any{
				"operation": "由官方工具注解决定",
				"tool":      cmdPositional(cmd, 0),
				"arguments": args,
			}
		},
		run: func(rt *Runtime, cmd *cobra.Command, args map[string]any) error {
			started := time.Now()
			name := cmdPositional(cmd, 0)
			client, err := rt.Client(cmdContext(rt))
			if err != nil {
				return err
			}
			tool, err := client.ToolByName(cmdContext(rt), name)
			if err != nil {
				return err
			}
			action := "dida tool call " + name
			if err := rt.Mode.CheckTool(*tool, action); err != nil {
				return err
			}
			readOnly, declared := tool.ReadOnly()
			operation := guard.Operation{ReadOnly: readOnly && declared, Destructive: tool.Destructive()}
			if err := guard.RequireConfirm(operation, action, rt.boolFlag(cmd, "yes")); err != nil {
				return err
			}
			if err := validateRequiredArgs(tool, args); err != nil {
				return err
			}
			if !operation.ReadOnly {
				rt.Logger.Info("执行写入工具",
					"command", "tool call",
					"tool", name,
					"destructive", operation.Destructive,
				)
			}
			result, err := rt.CallResult(cmdContext(rt), name, args)
			if err != nil {
				return err
			}
			return rt.EmitData("tool call", started, result.Data, output.Meta{
				Unparsed: result.Unparsed,
				Notes:    unparsedNote(result),
			})
		},
	})
}

// validateRequiredArgs 在发请求前先检查官方声明的必填参数.
//
// 服务端对缺参数的回复是一段校验文本, 对 agent 不友好; 在本地拦下可以给出
// 明确到具体参数名的错误.
func validateRequiredArgs(tool *official.Tool, args map[string]any) error {
	missing := make([]string, 0, 4)
	for _, name := range tool.RequiredArgs() {
		value, exists := args[name]
		if !exists || value == nil {
			missing = append(missing, name)
			continue
		}
		if text, ok := value.(string); ok && text == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return apperr.Usagef("%s 缺少必填参数: %s", tool.Name, joinComma(missing)).
		WithHintf("查看参数说明: dida tool show %s", tool.Name)
}

func unparsedNote(result *official.Result) string {
	if !result.Unparsed {
		return ""
	}
	return "官方返回的是纯文本而不是结构化 JSON, 完整内容在 data 字段里"
}

func firstLine(text string) string {
	for index, char := range text {
		if char == '\n' {
			return text[:index]
		}
	}
	return text
}

func sortedKeys(values []string) []string {
	keys := make([]string, len(values))
	copy(keys, values)
	sort.Strings(keys)
	return keys
}

func joinComma(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += ", "
		}
		result += value
	}
	return result
}
