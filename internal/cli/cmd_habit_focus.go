package cli

import (
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/spf13/cobra"
)

// habitWriteParams 返回习惯创建与修改共用的字段表.
func habitWriteParams(required bool) []schema.Param {
	return []schema.Param{
		{Name: "name", Type: "string", Required: required, Target: "habit.name", Description: "习惯名"},
		{Name: "goal", Type: "number", Target: "habit.goal", Description: "目标值, 例如每天 8 杯水就填 8"},
		{Name: "step", Type: "number", Target: "habit.step", Description: "每次打卡的步长"},
		{Name: "unit", Type: "string", Target: "habit.unit", Description: "计量单位, 例如 杯 / 次 / 分钟"},
		{Name: "type", Type: "string", Target: "habit.type", Description: "习惯类型"},
		{Name: "color", Type: "string", Target: "habit.color", Description: "颜色"},
		{Name: "icon", Type: "string", Target: "habit.iconRes", Description: "图标资源名, 自定义图标需带 txt_ 前缀"},
		{Name: "encouragement", Type: "string", Target: "habit.encouragement", Description: "鼓励语"},
		{Name: "repeat-rule", Type: "string", Target: "habit.repeatRule", Description: "重复规则"},
		{Name: "section", Type: "string", Target: "habit.sectionId", Description: "所属习惯分组 id"},
		{Name: "record-enable", Type: "bool", Target: "habit.recordEnable", Description: "是否开启数值记录"},
		{Name: "target-days", Type: "int", Target: "habit.targetDays", Description: "目标天数"},
		{Name: "style", Type: "int", Target: "habit.style", Description: "样式编号"},
		{Name: "sort-order", Type: "int", Target: "habit.sortOrder", Description: "排序值"},
	}
}

// attachHabit 挂载 habit 命令组.
func (rt *Runtime) attachHabit(root *cobra.Command) {
	group := rt.group(root, "habit", "习惯操作")

	rt.attach(group, commandSpec{
		path:      "habit list",
		summary:   "列出全部习惯",
		operation: schema.OpRead,
		tools:     []string{"list_habits"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("habit list", time.Now(), "list_habits", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "habit sections",
		summary:   "列出习惯分组",
		operation: schema.OpRead,
		tools:     []string{"list_habit_sections"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("habit sections", time.Now(), "list_habit_sections", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "habit get",
		summary:    "按 id 获取习惯详情",
		operation:  schema.OpRead,
		tools:      []string{"get_habit"},
		positional: []schema.Param{{Name: "habit-id", Required: true, Target: "habit_id", Description: "习惯 id"}},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("habit get", time.Now(), "get_habit", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "habit create",
		summary:       "新建习惯",
		operation:     schema.OpWrite,
		tools:         []string{"create_habit"},
		allowBodyJSON: true,
		params:        habitWriteParams(true),
		notes: "官方通道没有提供删除习惯的工具, 因此本命令建出来的习惯无法通过本 CLI 撤销, " +
			"只能去网页端处理. 正式创建前请先用 --dry-run 核对.",
		examples: []string{"dida habit create --name 喝水 --goal 8 --unit 杯 --dry-run"},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			args["habit"] = nestArgs(args, "habit.")
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("habit create", time.Now(), "create_habit", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "habit update",
		summary:       "修改习惯",
		operation:     schema.OpWrite,
		tools:         []string{"update_habit"},
		allowBodyJSON: true,
		positional:    []schema.Param{{Name: "habit-id", Required: true, Target: "habit_id", Description: "习惯 id"}},
		params:        habitWriteParams(false),
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			habit := nestArgs(args, "habit.")
			if len(habit) == 0 {
				return apperr.Usage("habit update 没有任何要修改的字段").
					WithHint("至少给出一个字段, 例如 --name 新名字")
			}
			if id := stringArg(args, "habit_id"); id != "" {
				habit["id"] = id
			}
			args["habit"] = habit
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("habit update", time.Now(), "update_habit", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "habit checkin",
		summary:    "给习惯打卡或补打卡",
		operation:  schema.OpWrite,
		tools:      []string{"upsert_habit_checkins"},
		positional: []schema.Param{{Name: "habit-id", Required: true, Target: "habit_id", Description: "习惯 id"}},
		notes: "stamp 是 YYYYMMDD 形式的整数日期, 例如 2026 年 9 月 24 日写 20260924. " +
			"status 取值为 0 未标记 / 1 未完成 / 2 已完成.",
		params: []schema.Param{
			{Name: "stamp", Type: "int", Required: true, Target: "checkin.stamp", Description: "打卡日期, 形式为 YYYYMMDD"},
			{Name: "value", Type: "number", Target: "checkin.value", Description: "打卡数值"},
			{Name: "goal", Type: "number", Target: "checkin.goal", Description: "当日目标值"},
			{Name: "status", Type: "int", Target: "checkin.status", Description: "打卡状态, 0 未标记 / 1 未完成 / 2 已完成"},
			{Name: "time", Type: "string", Target: "checkin.time", Description: "打卡时刻"},
		},
		examples: []string{"dida habit checkin 6ab51c89e4b06220cd6f0525 --stamp 20260924 --value 1 --status 2 --dry-run"},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			args["checkin_data"] = nestArgs(args, "checkin.")
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("habit checkin", time.Now(), "upsert_habit_checkins", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "habit checkins",
		summary:    "查询习惯在日期范围内的打卡记录",
		operation:  schema.OpRead,
		tools:      []string{"get_habit_checkins"},
		positional: []schema.Param{{Name: "habit-id", Required: true, Target: "habit_id", Description: "习惯 id"}},
		params: []schema.Param{
			{Name: "from-stamp", Type: "int", Required: true, Target: "from_stamp", Description: "起始日期, 形式为 YYYYMMDD"},
			{Name: "to-stamp", Type: "int", Required: true, Target: "to_stamp", Description: "结束日期, 形式为 YYYYMMDD"},
		},
		examples: []string{"dida habit checkins 6ab51c89e4b06220cd6f0525 --from-stamp 20260901 --to-stamp 20260930"},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			habitID := stringArg(args, "habit_id")
			delete(args, "habit_id")
			args["habit_ids"] = []any{habitID}
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("habit checkins", time.Now(), "get_habit_checkins", args, output.Meta{})
		},
	})
}

// attachFocus 挂载 focus 命令组.
func (rt *Runtime) attachFocus(root *cobra.Command) {
	group := rt.group(root, "focus", "专注记录操作")

	rt.attach(group, commandSpec{
		path:      "focus list",
		summary:   "按时间范围查询专注记录",
		operation: schema.OpRead,
		tools:     []string{"get_focuses_by_time"},
		notes:     "官方限制时间范围不得超过一个月.",
		params: []schema.Param{
			{Name: "from", Type: "string", Required: true, Target: "from_time", Description: "范围开始, 写法同 --due"},
			{Name: "to", Type: "string", Required: true, Target: "to_time", Description: "范围结束, 写法同 --due"},
			{Name: "type", Type: "int", Required: true, Target: "type", Description: "记录类型, 0 番茄钟 / 1 正计时"},
		},
		examples: []string{"dida focus list --from -7d --to today --type 0 --compact"},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.normalizeTimeFields(args, "from_time", "to_time")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("focus list", time.Now(), "get_focuses_by_time", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "focus get",
		summary:    "按 id 获取单条专注记录",
		operation:  schema.OpRead,
		tools:      []string{"get_focus"},
		positional: []schema.Param{{Name: "focus-id", Required: true, Target: "focus_id", Description: "专注记录 id"}},
		params: []schema.Param{
			{Name: "type", Type: "int", Required: true, Target: "type", Description: "记录类型, 0 番茄钟 / 1 正计时"},
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("focus get", time.Now(), "get_focus", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "focus create",
		summary:       "补录一条专注记录",
		operation:     schema.OpWrite,
		tools:         []string{"create_focus"},
		allowBodyJSON: true,
		notes:         "官方限制: 番茄钟最长 3 小时, 正计时最长 12 小时.",
		params: []schema.Param{
			{Name: "start", Type: "string", Required: true, Target: "start_time", Description: "开始时间"},
			{Name: "end", Type: "string", Required: true, Target: "end_time", Description: "结束时间"},
			{Name: "type", Type: "int", Required: true, Target: "type", Description: "记录类型, 0 番茄钟 / 1 正计时"},
			{Name: "task", Type: "string", Target: "task_id", Description: "关联任务 id"},
			{Name: "habit", Type: "string", Target: "habit_id", Description: "关联习惯 id"},
			{Name: "note", Type: "string", Target: "note", Description: "备注"},
		},
		examples: []string{"dida focus create --start 'today 09:00' --end 'today 09:25' --type 0 --dry-run"},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.normalizeTimeFields(args, "start_time", "end_time")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("focus create", time.Now(), "create_focus", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "focus update",
		summary:       "修改专注记录",
		operation:     schema.OpWrite,
		tools:         []string{"update_focus"},
		allowBodyJSON: true,
		positional:    []schema.Param{{Name: "focus-id", Required: true, Target: "focus_id", Description: "专注记录 id"}},
		params: []schema.Param{
			{Name: "type", Type: "int", Required: true, Target: "focus.type", Description: "记录类型, 0 番茄钟 / 1 正计时"},
			{Name: "note", Type: "string", Target: "focus.note", Description: "备注"},
			{Name: "start", Type: "string", Target: "focus.startTime", Description: "开始时间"},
			{Name: "end", Type: "string", Target: "focus.endTime", Description: "结束时间"},
		},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			focus := nestArgs(args, "focus.")
			if err := rt.normalizeMapTimes(focus, "startTime", "endTime"); err != nil {
				return err
			}
			args["focus"] = focus
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("focus update", time.Now(), "update_focus", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "focus delete",
		summary:    "删除专注记录",
		operation:  schema.OpDestructive,
		tools:      []string{"delete_focus"},
		positional: []schema.Param{{Name: "focus-id", Required: true, Target: "focus_id", Description: "专注记录 id"}},
		params: []schema.Param{
			{Name: "type", Type: "int", Required: true, Target: "type", Description: "记录类型, 0 番茄钟 / 1 正计时"},
		},
		notes:    "删除不可撤销, 需要 --yes.",
		examples: []string{"dida focus delete 6ab51c89 --type 0 --yes"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("focus delete", time.Now(), "delete_focus", args, output.Meta{})
		},
	})
}
