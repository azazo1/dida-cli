package cli

import (
	"sort"
	"strings"
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/azazo1/dida-cli/internal/timeparse"
	"github.com/spf13/cobra"
)

// 官方时间查询工具支持的取值.
const (
	queryToday      = "today"
	queryTomorrow   = "tomorrow"
	queryLast24Hour = "last24hour"
	queryLast7Day   = "last7day"
	queryNext24Hour = "next24hour"
	queryNext7Day   = "next7day"
)

// attachView 挂载 view 命令组.
func (rt *Runtime) attachView(root *cobra.Command) {
	group := rt.group(root, "view", "常用聚合视图")

	// 直接用官方时间查询的视图.
	directViews := []struct {
		name    string
		command string
		summary string
	}{
		{"today", queryToday, "今天到期与今天开始的任务"},
		{"tomorrow", queryTomorrow, "明天到期与明天开始的任务"},
		{"next", queryNext24Hour, "未来 24 小时内到期的任务"},
		{"upcoming", queryNext7Day, "未来 7 天内到期的任务"},
		{"last7day", queryLast7Day, "最近 7 天内的未完成任务"},
	}
	for _, view := range directViews {
		view := view
		rt.attach(group, commandSpec{
			path:      "view " + view.name,
			summary:   view.summary,
			operation: schema.OpRead,
			tools:     []string{"list_undone_tasks_by_time_query"},
			notes:     "该视图由官方服务端按账号时区解析, 与 dida task list 的本地时间范围语义不同.",
			run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
				return rt.callAndEmit("view "+view.name, time.Now(), "list_undone_tasks_by_time_query",
					map[string]any{"query_command": view.command}, output.Meta{})
			},
		})
	}

	rt.attach(group, commandSpec{
		path:      "view overdue",
		summary:   "已过期但尚未完成的任务",
		operation: schema.OpRead,
		tools:     []string{"filter_tasks"},
		notes: "官方没有提供过期视图, 这里取回全部未完成任务后在本地按截止时间筛选, " +
			"因此结果准确性取决于服务端返回的未完成任务是否完整.",
		params: []schema.Param{
			{Name: "limit", Type: "int", Default: "50", Target: "", Description: "返回条数上限"},
		},
		examples: []string{"dida view overdue --compact"},
		run: func(rt *Runtime, cmd *cobra.Command, _ map[string]any) error {
			started := time.Now()
			data, err := rt.Call(cmdContext(rt), "filter_tasks", map[string]any{
				"filter": map[string]any{"status": []any{0}},
			})
			if err != nil {
				return err
			}
			now := rt.Resolver.Now()
			overdue := make([]any, 0, 16)
			for _, task := range dataMaps(data) {
				due, ok := parseDue(task)
				if !ok || !due.Before(now) {
					continue
				}
				overdue = append(overdue, task)
			}
			sort.SliceStable(overdue, func(i, j int) bool {
				left, _ := parseDue(overdue[i].(map[string]any))
				right, _ := parseDue(overdue[j].(map[string]any))
				return left.Before(right)
			})
			overdue = applyLimit(overdue, rt.intFlag(cmd, "limit", 50))
			return rt.EmitData("view overdue", started, overdue, output.Meta{
				Notes: "按截止时间升序排列",
			})
		},
	})

	rt.attach(group, commandSpec{
		path:      "view inbox",
		summary:   "收集箱里未完成的任务",
		operation: schema.OpRead,
		tools:     []string{"filter_tasks"},
		notes:     "inbox 是官方返回的虚拟清单 id, 不需要先查询真实 id.",
		examples:  []string{"dida view inbox --compact"},
		run: func(rt *Runtime, _ *cobra.Command, _ map[string]any) error {
			return rt.callAndEmit("view inbox", time.Now(), "filter_tasks", map[string]any{
				"filter": map[string]any{
					"projectIds": []any{"inbox"},
					"status":     []any{0},
				},
			}, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "view completed",
		summary:   "按日期查看已完成任务",
		operation: schema.OpRead,
		tools:     []string{"list_completed_tasks_by_date"},
		notes:     "缺省查看今天. 官方要求给出时间范围, 这里用 --date 或 --start 与 --end 生成.",
		params: []schema.Param{
			{Name: "date", Type: "string", Target: "", Description: "单日, 写法同 --due, 缺省为今天"},
			{Name: "start", Type: "string", Target: "", Description: "范围开始, 与 --end 搭配使用"},
			{Name: "end", Type: "string", Target: "", Description: "范围结束"},
			{Name: "project", Type: "string-list", Target: "", Description: "清单 id, 可重复传入"},
		},
		examples: []string{
			"dida view completed --compact",
			"dida view completed --start -7d --end today --compact",
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			started := time.Now()
			start, end, err := rt.completedRange(args)
			if err != nil {
				return err
			}
			search := map[string]any{
				"startDate": start,
				"endDate":   end,
			}
			if projects := stringListArg(args, "project"); len(projects) > 0 {
				search["projectIds"] = toAnySlice(projects)
			}
			return rt.callAndEmit("view completed", started, "list_completed_tasks_by_date",
				map[string]any{"search": search}, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "view query",
		summary:   "按官方时间查询取值直接查询未完成任务",
		operation: schema.OpRead,
		tools:     []string{"list_undone_tasks_by_time_query"},
		notes:     "取值只有 " + strings.Join([]string{queryToday, queryTomorrow, queryLast24Hour, queryNext24Hour, queryLast7Day, queryNext7Day}, ", ") + " 这六种, 服务端不接受其它写法.",
		params: []schema.Param{
			{Name: "command", Type: "string", Required: true, Target: "query_command", Description: "官方时间查询取值"},
		},
		examples: []string{"dida view query --command next7day --compact"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			command := stringArg(args, "query_command")
			if !isOfficialQuery(command) {
				return apperr.Usagef("官方时间查询不支持 %q", command).
					WithHint("可用取值: today, tomorrow, last24hour, next24hour, last7day, next7day")
			}
			return rt.callAndEmit("view query", time.Now(), "list_undone_tasks_by_time_query", args, output.Meta{})
		},
	})
}

func isOfficialQuery(value string) bool {
	switch value {
	case queryToday, queryTomorrow, queryLast24Hour, queryNext24Hour, queryLast7Day, queryNext7Day:
		return true
	default:
		return false
	}
}

// completedRange 解析 view completed 的时间范围.
func (rt *Runtime) completedRange(args map[string]any) (string, string, error) {
	if date := stringArg(args, "date"); date != "" {
		parsed, err := rt.Resolver.Parse(date)
		if err != nil {
			return "", "", err
		}
		start, end := rt.Resolver.DayRange(parsed)
		return timeparse.Wire(start), timeparse.Wire(end), nil
	}
	startRaw := stringArg(args, "start")
	endRaw := stringArg(args, "end")
	if startRaw == "" && endRaw == "" {
		start, end := rt.Resolver.DayRange(rt.Resolver.Now())
		return timeparse.Wire(start), timeparse.Wire(end), nil
	}
	if startRaw == "" || endRaw == "" {
		return "", "", apperr.Usage("--start 与 --end 需要成对给出").
			WithHint("例如: dida view completed --start -7d --end today")
	}
	start, err := rt.Resolver.ParseWire(startRaw)
	if err != nil {
		return "", "", err
	}
	end, err := rt.Resolver.ParseWire(endRaw)
	if err != nil {
		return "", "", err
	}
	return start, end, nil
}

// parseDue 从任务对象里解析截止时间.
func parseDue(task map[string]any) (time.Time, bool) {
	raw, ok := task["dueDate"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		// 服务端也可能返回 2026-09-25T09:00:00.000+0800 这类不带冒号的偏移.
		parsed, err = time.Parse("2006-01-02T15:04:05.000-0700", raw)
		if err != nil {
			return time.Time{}, false
		}
	}
	return parsed, true
}

func applyLimit(items []any, limit int) []any {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func toAnySlice(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}
