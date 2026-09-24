package cli

import (
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/spf13/cobra"
)

// taskWriteParams 返回任务创建与修改共用的字段表.
func taskWriteParams(required bool) []schema.Param {
	return []schema.Param{
		{Name: "title", Type: "string", Required: required, Target: "task.title", Description: "任务标题"},
		{Name: "project", Type: "string", Target: "task.projectId", Description: "所属清单 id, 缺省为收集箱 inbox"},
		{Name: "content", Type: "string", Target: "task.content", Description: "任务正文"},
		{Name: "desc", Type: "string", Target: "task.desc", Description: "清单型任务的描述"},
		{Name: "due", Type: "string", Target: "task.dueDate", Description: "截止时间, 支持 2026-09-25 / 2026-09-25 09:00 / today / tomorrow / +3d"},
		{Name: "start", Type: "string", Target: "task.startDate", Description: "开始时间, 写法同 --due"},
		{Name: "priority", Type: "int", Target: "task.priority", Description: "优先级, 取值为 0 无 / 1 低 / 3 中 / 5 高"},
		{Name: "tags", Type: "string-list", Target: "task.tags", Description: "标签, 可重复传入"},
		{Name: "kind", Type: "string", Target: "task.kind", Description: "任务类型, 取值为 TEXT / NOTE / CHECKLIST"},
		{Name: "all-day", Type: "bool", Target: "task.isAllDay", Description: "是否为全天任务"},
		{Name: "parent", Type: "string", Target: "task.parentId", Description: "父任务 id, 用于创建子任务"},
		{Name: "column", Type: "string", Target: "task.columnId", Description: "看板列 id"},
		{Name: "reminders", Type: "string-list", Target: "task.reminders", Description: "提醒规则, 例如 TRIGGER:-PT60M"},
		{Name: "repeat", Type: "string", Target: "task.repeatFlag", Description: "重复规则, 例如 RRULE:FREQ=DAILY"},
		{Name: "repeat-from", Type: "string", Target: "task.repeatFrom", Description: "重复基准, 取值为 0 按截止时间 / 1 按完成时间 / 2 默认"},
	}
}

// taskCreateTransform 把任务字段搬进嵌套对象, 并补上默认清单.
func taskCreateTransform(rt *Runtime) func(*Runtime, *cobra.Command, map[string]any) error {
	return func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
		task := nestArgs(args, "task.")
		if err := rt.normalizeMapTimes(task, "dueDate", "startDate"); err != nil {
			return err
		}
		rt.applyDefaultProject(task, "projectId")
		args["task"] = task
		return nil
	}
}

// taskUpdateTransform 只搬运字段, 不擅自补清单.
//
// 修改场景下补 projectId 会把任务挪走, 属于典型的越权修改.
func taskUpdateTransform(rt *Runtime) func(*Runtime, *cobra.Command, map[string]any) error {
	return func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
		task := nestArgs(args, "task.")
		if err := rt.normalizeMapTimes(task, "dueDate", "startDate"); err != nil {
			return err
		}
		if len(task) == 0 {
			return apperr.Usage("task update 没有任何要修改的字段").
				WithHint("至少给出一个字段, 例如 --title 新标题")
		}
		if id := stringArg(args, "task_id"); id != "" {
			task["id"] = id
		}
		args["task"] = task
		return nil
	}
}

// attachTask 挂载 task 命令组.
func (rt *Runtime) attachTask(root *cobra.Command) {
	group := rt.group(root, "task", "任务操作")

	rt.attach(group, commandSpec{
		path:      "task list",
		summary:   "按条件筛选任务",
		operation: schema.OpRead,
		tools:     []string{"filter_tasks"},
		notes: "筛选字段在官方契约里都是数组, 因此这里的取值也可以重复传入. " +
			"时间范围请用 --start 与 --end 给出, 服务端对范围长度有限制, 过宽的范围会被拒绝.",
		params: []schema.Param{
			{Name: "start", Type: "string", Target: "filter.startDate", Description: "范围开始时间"},
			{Name: "end", Type: "string", Target: "filter.endDate", Description: "范围结束时间"},
			{Name: "project", Type: "string-list", Target: "filter.projectIds", Description: "清单 id, 可重复传入"},
			{Name: "priority", Type: "int-list", Target: "filter.priority", Description: "优先级筛选, 取值为 0 / 1 / 3 / 5, 可重复传入"},
			{Name: "tag", Type: "string-list", Target: "filter.tag", Description: "标签筛选, 可重复传入"},
			{Name: "kind", Type: "string-list", Target: "filter.kind", Description: "任务类型筛选, 取值为 TASK / NOTE / CHECKLIST"},
			{Name: "status", Type: "int-list", Target: "filter.status", Description: "状态筛选, 0 未完成 / 2 已完成 / -1 已放弃"},
		},
		examples: []string{
			"dida task list --project inbox --compact",
			"dida task list --start today --end +7d --compact",
		},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			filter := nestArgs(args, "filter.")
			if err := rt.normalizeMapTimes(filter, "startDate", "endDate"); err != nil {
				return err
			}
			if len(filter) == 0 {
				return apperr.Usage("task list 至少需要一个筛选条件").
					WithHint("例如: dida task list --project inbox")
			}
			args["filter"] = filter
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task list", time.Now(), "filter_tasks", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "task get",
		summary:    "按 id 获取任务详情",
		operation:  schema.OpRead,
		tools:      []string{"get_task_by_id", "get_task_in_project"},
		notes:      "给了 --project 走 get_task_in_project, 否则走 get_task_by_id 自动定位.",
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id"},
		},
		examples: []string{"dida task get 6ab51c89e4b06220cd6f0525"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			tool := "get_task_by_id"
			if stringArg(args, "project_id") != "" {
				tool = "get_task_in_project"
			}
			return rt.callAndEmit("task get", time.Now(), tool, args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "task create",
		summary:       "新建任务",
		operation:     schema.OpWrite,
		tools:         []string{"create_task"},
		allowBodyJSON: true,
		notes:         "未指定 --project 时任务落在收集箱 inbox.",
		params:        taskWriteParams(true),
		examples: []string{
			"dida task create --title 买牛奶 --due tomorrow",
			"dida task create --title 写周报 --project inbox --priority 5 --dry-run",
		},
		transform: taskCreateTransform(rt),
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task create", time.Now(), "create_task", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "task update",
		summary:       "修改任务字段",
		operation:     schema.OpWrite,
		tools:         []string{"update_task"},
		allowBodyJSON: true,
		notes: "清空截止时间请传 1970-01-01T00:00:00.000+0000. " +
			"解除子任务关系请把 --parent 传空字符串.",
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params:     taskWriteParams(false),
		examples: []string{
			"dida task update 6ab51c89e4b06220cd6f0525 --title 新标题 --dry-run",
		},
		transform: taskUpdateTransform(rt),
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task update", time.Now(), "update_task", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "task complete",
		summary:    "把任务标记为完成",
		operation:  schema.OpWrite,
		tools:      []string{"complete_task"},
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id, 缺省时自动反查"},
		},
		examples: []string{"dida task complete 6ab51c89e4b06220cd6f0525"},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.resolveTaskProject(args, "project_id", "task_id")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task complete", time.Now(), "complete_task", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "task delete",
		summary:    "删除任务",
		operation:  schema.OpDestructive,
		tools:      []string{"delete_task"},
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id, 缺省时自动反查"},
		},
		notes:    "删除不可撤销, 需要 --yes. 建议先 --dry-run 确认目标.",
		examples: []string{"dida task delete 6ab51c89e4b06220cd6f0525 --yes"},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.resolveTaskProject(args, "project_id", "task_id")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task delete", time.Now(), "delete_task", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "task move",
		summary:    "把任务移动到另一个清单",
		operation:  schema.OpWrite,
		tools:      []string{"move_task"},
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "to-project", Type: "string", Required: true, Target: "to_project", Description: "目标清单 id"},
			{Name: "from-project", Type: "string", Target: "from_project", Description: "源清单 id, 缺省时自动反查"},
			{Name: "sort-order", Type: "int", Target: "sort_order", Description: "在目标清单中的排序值"},
		},
		examples: []string{"dida task move 6ab51c89e4b06220cd6f0525 --to-project inbox --dry-run"},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			// 官方要求同时给出源清单, 缺省时按任务 id 反查.
			if err := rt.resolveTaskProject(args, "from_project", "task_id"); err != nil {
				return err
			}
			move := map[string]any{
				"taskId":        stringArg(args, "task_id"),
				"fromProjectId": stringArg(args, "from_project"),
				"toProjectId":   stringArg(args, "to_project"),
			}
			if order, ok := intArg(args, "sort_order"); ok {
				move["sortOrder"] = order
			}
			for _, key := range []string{"task_id", "from_project", "to_project", "sort_order"} {
				delete(args, key)
			}
			args["moves"] = []any{move}
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task move", time.Now(), "move_task", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "task search",
		summary:   "按关键词搜索任务",
		operation: schema.OpRead,
		tools:     []string{"search_task"},
		params: []schema.Param{
			{Name: "query", Type: "string", Required: true, Target: "query", Description: "搜索关键词"},
		},
		examples: []string{"dida task search --query 周报 --compact"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task search", time.Now(), "search_task", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "task checklist-done",
		summary:    "勾选任务里的一个子项",
		operation:  schema.OpWrite,
		tools:      []string{"complete_checklist_item"},
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "item-id", Type: "string", Required: true, Target: "checklist_item_id", Description: "子项 id"},
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id, 缺省时自动反查"},
		},
		notes:    "只对 kind 为 CHECKLIST 的任务有效, 子项 id 从 dida task get 的结果里取.",
		examples: []string{"dida task checklist-done 6ab51c89e4b06220cd6f0525 --item-id 6ab51c8a --dry-run"},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.resolveTaskProject(args, "project_id", "task_id")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task checklist-done", time.Now(), "complete_checklist_item", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "task assign",
		summary:    "把任务分派给共享清单成员",
		operation:  schema.OpWrite,
		tools:      []string{"assign_task"},
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "username", Type: "string", Required: true, Target: "assignee_username", Description: "成员用户名, 从 dida project members 获取"},
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id, 缺省时自动反查"},
		},
		examples: []string{"dida task assign 6ab51c89e4b06220cd6f0525 --username someone --dry-run"},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.resolveTaskProject(args, "project_id", "task_id")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task assign", time.Now(), "assign_task", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "task unassign",
		summary:    "取消任务的分派",
		operation:  schema.OpWrite,
		tools:      []string{"unassign_task"},
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id, 缺省时自动反查"},
		},
		examples: []string{"dida task unassign 6ab51c89e4b06220cd6f0525 --dry-run"},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.resolveTaskProject(args, "project_id", "task_id")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task unassign", time.Now(), "unassign_task", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "task batch-add",
		summary:       "批量新建任务",
		operation:     schema.OpWrite,
		tools:         []string{"batch_add_tasks"},
		allowBodyJSON: true,
		notes: "任务数组里每个元素的结构与 task create 的 --body-json 一致, " +
			"至少要有 title 与 projectId. 复杂结构建议用 --tasks-json @file 从文件读.",
		params: []schema.Param{
			{Name: "tasks-json", Type: "json", Required: true, Target: "tasks", Description: "任务数组 JSON"},
		},
		examples: []string{
			`dida task batch-add --tasks-json '[{"title":"a","projectId":"inbox"},{"title":"b","projectId":"inbox"}]' --dry-run`,
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task batch-add", time.Now(), "batch_add_tasks", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "task batch-update",
		summary:       "批量修改任务",
		operation:     schema.OpWrite,
		tools:         []string{"batch_update_tasks"},
		allowBodyJSON: true,
		notes:         "任务数组里每个元素必须带 id 与 projectId.",
		params: []schema.Param{
			{Name: "tasks-json", Type: "json", Required: true, Target: "tasks", Description: "任务数组 JSON"},
		},
		examples: []string{
			`dida task batch-update --tasks-json '[{"id":"x","projectId":"inbox","priority":5}]' --dry-run`,
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task batch-update", time.Now(), "batch_update_tasks", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "task complete-project",
		summary:   "批量完成某个清单里的多个任务",
		operation: schema.OpWrite,
		tools:     []string{"complete_tasks_in_project"},
		notes:     "单次最多 20 个任务.",
		params: []schema.Param{
			{Name: "project", Type: "string", Required: true, Target: "project_id", Description: "清单 id"},
			{Name: "task-ids", Type: "string-list", Required: true, Target: "task_ids", Description: "任务 id, 可重复传入"},
		},
		examples: []string{"dida task complete-project --project inbox --task-ids a --task-ids b --dry-run"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("task complete-project", time.Now(), "complete_tasks_in_project", args, output.Meta{})
		},
	})
}
