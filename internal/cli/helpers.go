package cli

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/config"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/spf13/cobra"
)

// stringArg 从请求体里取字符串参数.
func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

// intArg 从请求体里取整数参数, 兼容 JSON 解码产生的浮点表示.
func intArg(args map[string]any, key string) (int, bool) {
	switch value := args[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

// boolArg 从请求体里取布尔参数.
func boolArg(args map[string]any, key string) (bool, bool) {
	value, ok := args[key].(bool)
	return value, ok
}

// stringListArg 从请求体里取字符串列表参数.
func stringListArg(args map[string]any, key string) []string {
	switch value := args[key].(type) {
	case []string:
		return value
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

// mapArg 从请求体里取嵌套对象参数.
func mapArg(args map[string]any, key string) map[string]any {
	value, _ := args[key].(map[string]any)
	return value
}

// envToken 返回环境变量里的口令.
func envToken() string { return os.Getenv(config.EnvToken) }

// goVersion 返回构建本程序的 Go 版本.
func goVersion() string { return runtime.Version() }

// cmdPositional 返回命令的第 index 个位置参数.
//
// 有些位置参数是命令自身的定位信息 (例如 dida tool show 的工具名),
// 它们不应该进入请求体, 因此不声明 Target, 由实现直接取用.
func cmdPositional(cmd *cobra.Command, index int) string {
	values := cmd.Flags().Args()
	if index < 0 || index >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[index])
}

// cmdContext 返回命令的基础上下文.
func cmdContext(rt *Runtime) context.Context {
	if rt.ctx != nil {
		return rt.ctx
	}
	return context.Background()
}

// callAndEmit 调用单个官方工具并把结果渲染成信封.
func (rt *Runtime) callAndEmit(command string, started time.Time, tool string, args map[string]any, meta output.Meta) error {
	data, err := rt.Call(cmdContext(rt), tool, args)
	if err != nil {
		return err
	}
	return rt.EmitData(command, started, data, meta)
}

// nestArgs 把带前缀的参数搬进嵌套对象.
//
// 官方很多工具把业务字段包在一层对象里, 例如 create_task 需要 {"task": {...}}.
// 契约里用 "task.title" 这样的目标名声明字段, 这里统一完成搬运,
// 避免每个命令各写一遍组装逻辑.
func nestArgs(args map[string]any, prefix string) map[string]any {
	nested := map[string]any{}
	for key, value := range args {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		nested[strings.TrimPrefix(key, prefix)] = value
		delete(args, key)
	}
	return nested
}

// applyDefaultProject 在请求体里补上默认清单.
func (rt *Runtime) applyDefaultProject(args map[string]any, key string) {
	if _, exists := args[key]; exists {
		return
	}
	if project := strings.TrimSpace(rt.Config.DefaultProject); project != "" {
		args[key] = project
		return
	}
	// 与滴答自身行为一致: 未指定清单的新任务落在收集箱.
	args[key] = "inbox"
}

// normalizeTimeFields 把请求体里的时间字段统一转成服务端线格式.
func (rt *Runtime) normalizeTimeFields(args map[string]any, fields ...string) error {
	return rt.normalizeMapTimes(args, fields...)
}

// normalizeMapTimes 归一化一个对象里的时间字段.
func (rt *Runtime) normalizeMapTimes(target map[string]any, fields ...string) error {
	for _, field := range fields {
		raw, ok := target[field].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		normalized, err := rt.Resolver.ParseWire(raw)
		if err != nil {
			return err
		}
		target[field] = normalized
	}
	return nil
}

// resolveTaskProject 解析任务所属清单.
//
// 官方多数任务工具要求同时提供 project_id 与 task_id. 为了让 agent 不必
// 先查清单, 这里在缺少 project_id 时用 task_id 反查一次.
func (rt *Runtime) resolveTaskProject(args map[string]any, projectKey, taskKey string) error {
	if project := stringArg(args, projectKey); project != "" {
		return nil
	}
	taskID := stringArg(args, taskKey)
	if taskID == "" {
		return apperr.Usagef("缺少 %s, 且无法通过 %s 反查", projectKey, taskKey).
			WithHint("显式提供 --project")
	}
	data, err := rt.Call(cmdContext(rt), "get_task_by_id", map[string]any{"task_id": taskID})
	if err != nil {
		return err
	}
	records := dataMaps(data)
	if len(records) == 0 {
		return apperr.NotFound("没有找到任务 " + taskID)
	}
	project, _ := records[0]["projectId"].(string)
	if project == "" {
		return apperr.NotFound("任务 " + taskID + " 没有返回所属清单").
			WithHint("显式提供 --project")
	}
	args[projectKey] = project
	rt.Logger.Debug("已通过任务反查清单", "task_id", taskID, "project_id", project)
	return nil
}

// dataList 把官方返回的数据统一成列表形态.
//
// 官方工具有时返回裸数组, 有时返回 {"result": [...]} 这类外壳, 这里统一拍平,
// 让上层命令不必各自处理.
func dataList(data any) []any {
	switch value := data.(type) {
	case []any:
		return value
	case map[string]any:
		for _, key := range []string{"result", "results", "tasks", "projects", "habits", "items"} {
			if items, ok := value[key].([]any); ok {
				return items
			}
		}
		return []any{value}
	case nil:
		return nil
	default:
		return []any{value}
	}
}

// dataMaps 把官方返回的数据统一成对象列表.
func dataMaps(data any) []map[string]any {
	items := dataList(data)
	records := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			records = append(records, record)
		}
	}
	return records
}
