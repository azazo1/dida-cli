package cli

import (
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/spf13/cobra"
)

// attachTag 挂载 tag 命令组.
func (rt *Runtime) attachTag(root *cobra.Command) {
	group := rt.group(root, "tag", "标签操作")

	rt.attach(group, commandSpec{
		path:      "tag list",
		summary:   "列出全部标签",
		operation: schema.OpRead,
		tools:     []string{"list_tags"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("tag list", time.Now(), "list_tags", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "tag create",
		summary:       "新建标签",
		operation:     schema.OpWrite,
		tools:         []string{"create_tag"},
		allowBodyJSON: true,
		params: []schema.Param{
			{Name: "name", Type: "string", Required: true, Target: "tag.name", Description: "标签名"},
			{Name: "label", Type: "string", Target: "tag.label", Description: "显示名"},
			{Name: "color", Type: "string", Target: "tag.color", Description: "颜色"},
			{Name: "parent", Type: "string", Target: "tag.parent", Description: "父标签"},
			{Name: "type", Type: "string", Target: "tag.type", Description: "标签类型"},
			{Name: "sort-order", Type: "int", Target: "tag.sortOrder", Description: "排序值"},
		},
		examples: []string{"dida tag create --name 重要 --dry-run"},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			args["tag"] = nestArgs(args, "tag.")
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("tag create", time.Now(), "create_tag", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "tag update",
		summary:       "修改标签属性",
		operation:     schema.OpWrite,
		tools:         []string{"update_tag"},
		allowBodyJSON: true,
		notes:         "官方说明该工具不能改名字, 改名请用 dida tag rename.",
		params: []schema.Param{
			{Name: "name", Type: "string", Required: true, Target: "tag.name", Description: "要修改的标签名"},
			{Name: "label", Type: "string", Target: "tag.label", Description: "显示名"},
			{Name: "color", Type: "string", Target: "tag.color", Description: "颜色"},
			{Name: "parent", Type: "string", Target: "tag.parent", Description: "父标签"},
			{Name: "type", Type: "string", Target: "tag.type", Description: "标签类型"},
			{Name: "sort-order", Type: "int", Target: "tag.sortOrder", Description: "排序值"},
		},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			args["tag"] = nestArgs(args, "tag.")
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("tag update", time.Now(), "update_tag", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "tag rename",
		summary:   "重命名标签",
		operation: schema.OpWrite,
		tools:     []string{"rename_tag"},
		positional: []schema.Param{
			{Name: "name", Required: true, Target: "name", Description: "原标签名"},
			{Name: "new-name", Required: true, Target: "new_name", Description: "新标签名"},
		},
		params: []schema.Param{
			{Name: "scope", Type: "int", Target: "scope", Description: "作用范围"},
		},
		notes:    "新标签名不得超过 64 字符, 且不能包含空白与 \\ / \" # : * ? < > | 这些字符.",
		examples: []string{"dida tag rename 重要 很重要 --dry-run"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("tag rename", time.Now(), "rename_tag", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "tag delete",
		summary:    "删除标签并解除其任务关联",
		operation:  schema.OpDestructive,
		tools:      []string{"delete_tag"},
		positional: []schema.Param{{Name: "name", Required: true, Target: "name", Description: "标签名"}},
		params: []schema.Param{
			{Name: "scope", Type: "int", Target: "scope", Description: "作用范围"},
		},
		notes:    "删除不可撤销, 需要 --yes. 标签会从所有任务上摘掉.",
		examples: []string{"dida tag delete 重要 --yes"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("tag delete", time.Now(), "delete_tag", args, output.Meta{})
		},
	})
}

// attachGroupCmds 挂载 group 命令组, 即清单分组.
func (rt *Runtime) attachGroupCmds(root *cobra.Command) {
	group := rt.group(root, "group", "清单分组操作")

	rt.attach(group, commandSpec{
		path:      "group list",
		summary:   "列出全部清单分组",
		operation: schema.OpRead,
		tools:     []string{"list_project_groups"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("group list", time.Now(), "list_project_groups", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "group create",
		summary:       "新建清单分组",
		operation:     schema.OpWrite,
		tools:         []string{"create_project_group"},
		allowBodyJSON: true,
		params: []schema.Param{
			{Name: "name", Type: "string", Required: true, Target: "project_group.name", Description: "分组名"},
			{Name: "sort-order", Type: "int", Target: "project_group.sortOrder", Description: "排序值"},
			{Name: "view-mode", Type: "string", Target: "project_group.viewMode", Description: "视图模式"},
			{Name: "show-all", Type: "bool", Target: "project_group.showAll", Description: "是否显示全部清单"},
		},
		examples: []string{"dida group create --name 工作 --dry-run"},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			args["project_group"] = nestArgs(args, "project_group.")
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("group create", time.Now(), "create_project_group", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "group update",
		summary:       "修改清单分组",
		operation:     schema.OpWrite,
		tools:         []string{"update_project_group"},
		allowBodyJSON: true,
		positional:    []schema.Param{{Name: "group-id", Required: true, Target: "project_group_id", Description: "分组 id"}},
		params: []schema.Param{
			{Name: "name", Type: "string", Target: "project_group.name", Description: "分组名"},
			{Name: "sort-order", Type: "int", Target: "project_group.sortOrder", Description: "排序值"},
			{Name: "view-mode", Type: "string", Target: "project_group.viewMode", Description: "视图模式"},
			{Name: "show-all", Type: "bool", Target: "project_group.showAll", Description: "是否显示全部清单"},
		},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			body := nestArgs(args, "project_group.")
			if len(body) == 0 {
				return apperr.Usage("group update 没有任何要修改的字段").
					WithHint("至少给出一个字段, 例如 --name 新名字")
			}
			args["project_group"] = body
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("group update", time.Now(), "update_project_group", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "group delete",
		summary:    "删除清单分组",
		operation:  schema.OpDestructive,
		tools:      []string{"delete_project_group"},
		positional: []schema.Param{{Name: "group-id", Required: true, Target: "project_group_id", Description: "分组 id"}},
		notes:      "删除不可撤销, 需要 --yes. 分组内的清单本身不会被删除.",
		examples:   []string{"dida group delete 6ab51c89e4b06220cd6f0525 --yes"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("group delete", time.Now(), "delete_project_group", args, output.Meta{})
		},
	})
}

// attachColumn 挂载 column 命令组, 即看板列.
func (rt *Runtime) attachColumn(root *cobra.Command) {
	group := rt.group(root, "column", "看板列操作")

	rt.attach(group, commandSpec{
		path:       "column list",
		summary:    "列出某个清单的看板列",
		operation:  schema.OpRead,
		tools:      []string{"list_columns"},
		positional: []schema.Param{{Name: "project-id", Required: true, Target: "project_id", Description: "清单 id"}},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("column list", time.Now(), "list_columns", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "column create",
		summary:       "在看板清单里新建一列",
		operation:     schema.OpWrite,
		tools:         []string{"create_column"},
		allowBodyJSON: true,
		positional:    []schema.Param{{Name: "project-id", Required: true, Target: "project_id", Description: "清单 id"}},
		params: []schema.Param{
			{Name: "name", Type: "string", Required: true, Target: "column.name", Description: "列名"},
			{Name: "sort-order", Type: "int", Target: "column.sortOrder", Description: "排序值"},
		},
		examples: []string{"dida column create inbox --name 待办 --dry-run"},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			args["column"] = nestArgs(args, "column.")
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("column create", time.Now(), "create_column", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "column update",
		summary:       "修改看板列",
		operation:     schema.OpWrite,
		tools:         []string{"update_column"},
		allowBodyJSON: true,
		positional: []schema.Param{
			{Name: "project-id", Required: true, Target: "project_id", Description: "清单 id"},
			{Name: "column-id", Required: true, Target: "column_id", Description: "列 id"},
		},
		params: []schema.Param{
			{Name: "name", Type: "string", Target: "column.name", Description: "列名"},
			{Name: "sort-order", Type: "int", Target: "column.sortOrder", Description: "排序值"},
		},
		transform: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			body := nestArgs(args, "column.")
			if len(body) == 0 {
				return apperr.Usage("column update 没有任何要修改的字段").
					WithHint("至少给出一个字段, 例如 --name 新列名")
			}
			body["id"] = stringArg(args, "column_id")
			args["column"] = body
			return nil
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("column update", time.Now(), "update_column", args, output.Meta{})
		},
	})
}

// attachComment 挂载 comment 命令组.
func (rt *Runtime) attachComment(root *cobra.Command) {
	group := rt.group(root, "comment", "任务评论操作")

	rt.attach(group, commandSpec{
		path:       "comment list",
		summary:    "列出一个任务的全部评论",
		operation:  schema.OpRead,
		tools:      []string{"get_comment"},
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id, 缺省时自动反查"},
		},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.resolveTaskProject(args, "project_id", "task_id")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("comment list", time.Now(), "get_comment", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "comment add",
		summary:    "给任务添加评论",
		operation:  schema.OpWrite,
		tools:      []string{"add_comment"},
		positional: []schema.Param{{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"}},
		params: []schema.Param{
			{Name: "text", Type: "string", Required: true, Target: "title", Description: "评论正文, 纯文本, 最长 1024 字符"},
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id, 缺省时自动反查"},
		},
		notes:    "官方把评论正文的字段命名为 title, 这里对外统一叫 --text.",
		examples: []string{"dida comment add 6ab51c89e4b06220cd6f0525 --text '已联系供应商' --dry-run"},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.resolveTaskProject(args, "project_id", "task_id")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("comment add", time.Now(), "add_comment", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:      "comment delete",
		summary:   "删除任务评论",
		operation: schema.OpDestructive,
		tools:     []string{"delete_comment"},
		positional: []schema.Param{
			{Name: "task-id", Required: true, Target: "task_id", Description: "任务 id"},
			{Name: "comment-id", Required: true, Target: "id", Description: "评论 id"},
		},
		params: []schema.Param{
			{Name: "project", Type: "string", Target: "project_id", Description: "任务所属清单 id, 缺省时自动反查"},
		},
		notes:    "删除不可撤销, 需要 --yes.",
		examples: []string{"dida comment delete 6ab51c89e4b06220cd6f0525 6ab51c8a --yes"},
		resolve: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.resolveTaskProject(args, "project_id", "task_id")
		},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("comment delete", time.Now(), "delete_comment", args, output.Meta{})
		},
	})
}

// attachCountdown 挂载 countdown 命令组.
func (rt *Runtime) attachCountdown(root *cobra.Command) {
	group := rt.group(root, "countdown", "倒数日操作")

	rt.attach(group, commandSpec{
		path:      "countdown list",
		summary:   "列出全部倒数日",
		operation: schema.OpRead,
		tools:     []string{"list_countdowns"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("countdown list", time.Now(), "list_countdowns", args, output.Meta{})
		},
	})
}
