package cli

import (
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/guard"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// commandSpec 同时描述一条命令的契约与实现.
//
// 契约与实现共用一份声明, 是为了从根上避免"文档写了一套, 代码做了一套".
type commandSpec struct {
	// path 是完整命令路径, 例如 "task create".
	path string
	// summary 是一句话说明.
	summary string
	// aliases 是命令别名.
	aliases []string
	// operation 是操作类型, 决定闸门判定与是否需要 --yes.
	operation schema.Operation
	// tools 是该命令调用的官方工具名.
	tools []string
	// params 是参数表, 同时驱动 flag 注册与请求体组装.
	params []schema.Param
	// positional 是位置参数, 按声明顺序接收.
	positional []schema.Param
	// notes 是给 agent 的补充说明.
	notes string
	// examples 是可直接复制的示例.
	examples []string
	// allowBodyJSON 表示该命令接受 --body-json 完整参数模式.
	allowBodyJSON bool
	// run 是命令实现, args 是组装好的请求体.
	run func(rt *Runtime, cmd *cobra.Command, args map[string]any) error
	// transform 对请求体做纯本地加工, 不允许发起网络请求.
	transform func(rt *Runtime, cmd *cobra.Command, args map[string]any) error
	// resolve 补齐需要联网查询才能得到的参数, 例如由任务 id 反查所属清单.
	//
	// 它排在闸门之后, 因此被闸门拦下的写入不会产生任何网络流量.
	resolve func(rt *Runtime, cmd *cobra.Command, args map[string]any) error
	// preview 覆盖默认的 dry-run 预览内容, 可为空.
	preview func(rt *Runtime, cmd *cobra.Command, args map[string]any) map[string]any
}

// allParams 返回 flag 参数与位置参数的合并视图, 供契约登记使用.
//
// 位置参数在这里统一打上标记, 避免各处声明时漏写导致契约里把位置参数
// 误报成 flag -- agent 照着调就会踩空.
func (s commandSpec) allParams() []schema.Param {
	params := make([]schema.Param, 0, len(s.params)+len(s.positional))
	params = append(params, s.params...)
	for _, param := range s.positional {
		param.Positional = true
		params = append(params, param)
	}
	return params
}

// attach 把一条命令挂到父命令下, 并登记契约.
func (rt *Runtime) attach(parent *cobra.Command, spec commandSpec) *cobra.Command {
	use := spec.path
	if index := strings.LastIndex(spec.path, " "); index >= 0 {
		use = spec.path[index+1:]
	}
	if usage := positionalUsage(spec.positional); usage != "" {
		use += " " + usage
	}
	cmd := &cobra.Command{
		Use:     use,
		Aliases: spec.aliases,
		Short:   spec.summary,
		Long:    longDescription(spec),
		Example: strings.Join(spec.examples, "\n"),
		Args:    argsValidator(spec.positional),
		RunE: func(cmd *cobra.Command, positional []string) error {
			return rt.execute(spec, cmd, positional)
		},
	}
	registerParams(cmd.Flags(), spec.params)
	if spec.operation != schema.OpRead {
		cmd.Flags().Bool("dry-run", false, "只输出将要发送的请求体, 不实际执行")
	}
	if spec.operation == schema.OpDestructive || spec.operation == schema.OpDynamic {
		cmd.Flags().Bool("yes", false, "确认执行破坏性操作")
	}
	if spec.operation != schema.OpRead && spec.allowBodyJSON {
		cmd.Flags().String("body-json", "", "完整参数模式: 直接提供请求体 JSON, 取值为 JSON 文本, - 表示从 stdin 读取, @文件 表示从文件读取")
	}
	parent.AddCommand(cmd)
	rt.Registry.Add(schema.Entry{
		Command:        spec.path,
		Summary:        spec.summary,
		Operation:      spec.operation,
		Tools:          spec.tools,
		Params:         spec.allParams(),
		SupportsDryRun: spec.operation != schema.OpRead,
		NeedsConfirm:   spec.operation == schema.OpDestructive,
		Notes:          spec.notes,
		Examples:       spec.examples,
	})
	return cmd
}

// execute 是命令的统一入口: 先过闸门, 再组装请求体, 最后交给实现.
// execute 是命令的统一入口.
//
// 顺序是有讲究的: 先组装请求体, 再处理 dry-run, 最后才过闸门. 这样做的
// 理由是 --dry-run 本身不产生任何写入, 因此不应被只读模式或确认要求挡住,
// 它恰恰是 user 在受限环境下核对请求体的手段.
func (rt *Runtime) execute(spec commandSpec, cmd *cobra.Command, positional []string) error {
	started := time.Now()
	action := "dida " + spec.path
	operation := guard.OperationOf(string(spec.operation))

	args, usedBody, err := rt.buildPayload(spec, cmd, positional)
	if err != nil {
		return rt.Fail(spec.path, started, err)
	}
	// 完整参数模式下 payload 由 user 整份提供, 任何本地加工都会篡改它的语义,
	// 因此跳过 transform 与 resolve.
	if !usedBody {
		if spec.transform != nil {
			if err := spec.transform(rt, cmd, args); err != nil {
				return rt.Fail(spec.path, started, err)
			}
		}
	}

	if rt.boolFlag(cmd, "dry-run") {
		// dry-run 也要先把需要联网补齐的参数补齐, 否则预览出来的请求体
		// 与实际会发出的并不一致, 预览就失去了意义.
		if spec.resolve != nil && !usedBody {
			if err := spec.resolve(rt, cmd, args); err != nil {
				return rt.Fail(spec.path, started, err)
			}
		}
		payload := dryRunPreview(spec, args)
		if spec.preview != nil {
			payload = spec.preview(rt, cmd, args)
		}
		payload["applied_gates"] = rt.Mode.Describe()
		return rt.Emit(spec.path, started, payload, output.Meta{DryRun: true})
	}

	// 闸门放在 dry-run 之后: 预览不产生任何写入, 因此在受限环境下也必须可用,
	// 它正是 user 在不放松闸门的前提下核对请求体的手段.
	if spec.operation != schema.OpDynamic {
		if err := rt.Mode.Check(operation, action); err != nil {
			return rt.Fail(spec.path, started, err)
		}
		if err := guard.RequireConfirm(operation, action, rt.boolFlag(cmd, "yes")); err != nil {
			return rt.Fail(spec.path, started, err)
		}
	}
	if spec.resolve != nil && !usedBody {
		if err := spec.resolve(rt, cmd, args); err != nil {
			return rt.Fail(spec.path, started, err)
		}
	}

	// 写操作是审计重点, 用 info; 只读与操作性质待定的命令只是诊断信息, 用 debug.
	if spec.operation == schema.OpRead || spec.operation == schema.OpDynamic {
		rt.Logger.Debug("执行命令", "command", spec.path, "operation", string(spec.operation), "tools", spec.tools)
	} else {
		rt.Logger.Info("执行写入命令",
			"command", spec.path,
			"operation", string(spec.operation),
			"tools", spec.tools,
		)
	}
	if err := spec.run(rt, cmd, args); err != nil {
		return rt.Fail(spec.path, started, err)
	}
	return nil
}

// buildPayload 组装发给官方工具的请求体.
//
// 第二个返回值表示本次是否走了完整参数模式, 调用方据此决定要不要再做本地加工.
func (rt *Runtime) buildPayload(spec commandSpec, cmd *cobra.Command, positional []string) (map[string]any, bool, error) {
	if cmd.Flags().Lookup("body-json") != nil {
		raw, err := cmd.Flags().GetString("body-json")
		if err != nil {
			return nil, false, apperr.Wrap(apperr.KindUsage, err, "读取 --body-json 失败")
		}
		if strings.TrimSpace(raw) != "" {
			body, err := rt.decodeBody(raw)
			if err != nil {
				return nil, false, err
			}
			return body, true, nil
		}
	}
	args := map[string]any{}
	for index, param := range spec.positional {
		if index >= len(positional) {
			break
		}
		value := strings.TrimSpace(positional[index])
		if value == "" || param.Target == "" {
			continue
		}
		args[param.Target] = value
	}
	for _, param := range spec.params {
		if param.Target == "" {
			continue
		}
		flag := cmd.Flags().Lookup(param.Name)
		if flag == nil {
			continue
		}
		value, err := flagValue(cmd.Flags(), param)
		if err != nil {
			return nil, false, err
		}
		if value == nil {
			continue
		}
		args[param.Target] = value
	}
	for _, param := range spec.allParams() {
		if !param.Required || param.Target == "" {
			continue
		}
		if _, ok := args[param.Target]; !ok {
			return nil, false, apperr.Usagef("%s 缺少必填参数 %s", spec.path, paramLabel(param)).
				WithHintf("查看参数说明: dida schema show %s", spec.path)
		}
	}
	return args, false, nil
}

// decodeBody 解析完整参数模式的输入.
func (rt *Runtime) decodeBody(raw string) (map[string]any, error) {
	text := strings.TrimSpace(raw)
	switch {
	case text == "-":
		content, err := io.ReadAll(rt.Stdin)
		if err != nil {
			return nil, apperr.Wrap(apperr.KindUsage, err, "从 stdin 读取请求体失败")
		}
		text = strings.TrimSpace(string(content))
	case strings.HasPrefix(text, "@"):
		content, err := os.ReadFile(strings.TrimPrefix(text, "@"))
		if err != nil {
			return nil, apperr.Wrap(apperr.KindUsage, err, "读取请求体文件失败")
		}
		text = strings.TrimSpace(string(content))
	}
	if text == "" {
		return nil, apperr.Usage("完整参数模式收到空请求体")
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(text), &args); err != nil {
		return nil, apperr.Wrap(apperr.KindUsage, err, "请求体不是合法 JSON 对象")
	}
	return args, nil
}

// flagValue 按参数类型取出取值, 未显式设置的参数返回 nil.
//
// 只有用户真的写了某个 flag 才把它放进请求体, 这样默认值不会悄悄改变
// 服务端的既有数据.
func flagValue(flags *pflag.FlagSet, param schema.Param) (any, error) {
	switch param.Type {
	case "string-list":
		items, err := flags.GetStringArray(param.Name)
		if err != nil {
			return nil, apperr.Wrap(apperr.KindUsage, err, "读取 --"+param.Name+" 失败")
		}
		if len(items) == 0 {
			return nil, nil
		}
		return items, nil
	case "int-list":
		raw, err := flags.GetStringArray(param.Name)
		if err != nil {
			return nil, apperr.Wrap(apperr.KindUsage, err, "读取 --"+param.Name+" 失败")
		}
		if len(raw) == 0 {
			return nil, nil
		}
		values := make([]any, 0, len(raw))
		for _, item := range raw {
			parsed, err := strconv.Atoi(strings.TrimSpace(item))
			if err != nil {
				return nil, apperr.Usagef("--%s 需要整数列表, 收到 %q", param.Name, item)
			}
			values = append(values, parsed)
		}
		return values, nil
	case "bool":
		if !flags.Changed(param.Name) {
			return nil, nil
		}
		value, err := flags.GetBool(param.Name)
		if err != nil {
			return nil, apperr.Wrap(apperr.KindUsage, err, "读取 --"+param.Name+" 失败")
		}
		return value, nil
	case "int":
		if !flags.Changed(param.Name) {
			return nil, nil
		}
		value, err := flags.GetInt(param.Name)
		if err != nil {
			return nil, apperr.Wrap(apperr.KindUsage, err, "读取 --"+param.Name+" 失败")
		}
		return value, nil
	}

	text, err := flags.GetString(param.Name)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindUsage, err, "读取 --"+param.Name+" 失败")
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	switch param.Type {
	case "number":
		parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return nil, apperr.Usagef("--%s 需要数字, 收到 %q", param.Name, text)
		}
		return parsed, nil
	case "json":
		var decoded any
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return nil, apperr.Wrap(apperr.KindUsage, err, "--"+param.Name+" 不是合法 JSON")
		}
		return decoded, nil
	default:
		return text, nil
	}
}

// registerParams 按契约注册 flag.
func registerParams(flags *pflag.FlagSet, params []schema.Param) {
	for _, param := range params {
		description := param.Description
		if param.Required {
			description += " (必填)"
		}
		switch param.Type {
		case "int":
			fallback := 0
			if param.Default != "" {
				if parsed, err := strconv.Atoi(param.Default); err == nil {
					fallback = parsed
				}
			}
			flags.Int(param.Name, fallback, description)
		case "bool":
			flags.Bool(param.Name, param.Default == "true", description)
		case "string-list", "int-list":
			flags.StringArray(param.Name, nil, description)
		default:
			flags.String(param.Name, param.Default, description)
		}
	}
}

// argsValidator 依据位置参数声明生成参数个数校验.
func argsValidator(positional []schema.Param) cobra.PositionalArgs {
	if len(positional) == 0 {
		return cobra.NoArgs
	}
	minimum := 0
	for _, param := range positional {
		if param.Required {
			minimum++
		}
	}
	return cobra.RangeArgs(minimum, len(positional))
}

// positionalUsage 生成位置参数在 Use 行里的展示形式.
func positionalUsage(positional []schema.Param) string {
	if len(positional) == 0 {
		return ""
	}
	parts := make([]string, 0, len(positional))
	for _, param := range positional {
		if param.Required {
			parts = append(parts, "<"+param.Name+">")
			continue
		}
		parts = append(parts, "["+param.Name+"]")
	}
	return strings.Join(parts, " ")
}

func paramLabel(param schema.Param) string {
	if param.Positional {
		return "<" + param.Name + ">"
	}
	return "--" + param.Name
}

// dryRunPreview 组装 dry-run 的输出内容.
func dryRunPreview(spec commandSpec, args map[string]any) map[string]any {
	preview := map[string]any{
		"operation": string(spec.operation),
		"arguments": args,
	}
	if len(spec.tools) == 1 {
		preview["tool"] = spec.tools[0]
	} else {
		preview["tools"] = spec.tools
	}
	return preview
}

func longDescription(spec commandSpec) string {
	var builder strings.Builder
	builder.WriteString(spec.summary)
	builder.WriteString("\n\n操作类型: ")
	builder.WriteString(string(spec.operation))
	if len(spec.tools) > 0 {
		builder.WriteString("\n官方工具: ")
		builder.WriteString(strings.Join(spec.tools, ", "))
	}
	if spec.notes != "" {
		builder.WriteString("\n\n")
		builder.WriteString(spec.notes)
	}
	return builder.String()
}

func (rt *Runtime) boolFlag(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	value, err := cmd.Flags().GetBool(name)
	if err != nil {
		return false
	}
	return value
}

// intFlag 读取整数 flag, 缺失或非法时返回兜底值.
func (rt *Runtime) intFlag(cmd *cobra.Command, name string, fallback int) int {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return fallback
	}
	value, err := cmd.Flags().GetInt(name)
	if err != nil || value == 0 {
		return fallback
	}
	return value
}

// endpointOverride 返回环境变量指定的端点覆盖值.
func endpointOverride() string {
	return strings.TrimSpace(os.Getenv("DIDA_ENDPOINT"))
}
