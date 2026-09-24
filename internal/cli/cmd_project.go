package cli

import (
	"time"

	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/spf13/cobra"
)

// attachProject 挂载 project 命令组.
func (rt *Runtime) attachProject(root *cobra.Command) {
	group := rt.group(root, "project", "清单 (项目) 操作")

	rt.attach(group, commandSpec{
		path:      "project list",
		summary:   "列出全部清单",
		operation: schema.OpRead,
		tools:     []string{"list_projects"},
		notes:     "不传分页参数时结果里会额外包含一个 id 为 inbox 的虚拟清单, 可以直接用于任务操作.",
		params: []schema.Param{
			{Name: "limit", Type: "int", Target: "limit", Description: "返回条数上限"},
			{Name: "offset", Type: "int", Target: "offset", Description: "起始偏移"},
		},
		examples: []string{"dida project list --compact"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("project list", time.Now(), "list_projects", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "project get",
		summary:    "按 id 获取清单详情",
		operation:  schema.OpRead,
		tools:      []string{"get_project_by_id"},
		positional: []schema.Param{{Name: "project-id", Required: true, Target: "project_id", Description: "清单 id"}},
		examples:   []string{"dida project get inbox"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("project get", time.Now(), "get_project_by_id", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "project data",
		summary:    "一次取回清单详情与其全部未完成任务",
		operation:  schema.OpRead,
		tools:      []string{"get_project_with_undone_tasks"},
		notes:      "需要一次拿到某个清单全貌时用这个, 比先 list 再逐个 get 省一次往返.",
		positional: []schema.Param{{Name: "project-id", Required: true, Target: "project_id", Description: "清单 id"}},
		examples:   []string{"dida project data inbox --compact"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("project data", time.Now(), "get_project_with_undone_tasks", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:       "project members",
		summary:    "列出共享清单的成员",
		operation:  schema.OpRead,
		tools:      []string{"list_project_members"},
		notes:      "返回的 username 可以用于 dida task assign.",
		positional: []schema.Param{{Name: "project-id", Required: true, Target: "project_id", Description: "清单 id"}},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("project members", time.Now(), "list_project_members", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "project create",
		summary:       "新建清单",
		operation:     schema.OpWrite,
		tools:         []string{"create_project"},
		allowBodyJSON: true,
		params: []schema.Param{
			{Name: "name", Type: "string", Required: true, Target: "name", Description: "清单名"},
			{Name: "color", Type: "string", Target: "color", Description: "颜色, 例如 #4A90D9"},
			{Name: "view-mode", Type: "string", Target: "view_mode", Description: "视图模式, 取值为 list / kanban / timeline"},
			{Name: "kind", Type: "string", Target: "kind", Description: "清单类型, 取值为 TASK / NOTE"},
			{Name: "sort-order", Type: "int", Target: "sort_order", Description: "排序值"},
			{Name: "group-id", Type: "string", Target: "group_id", Description: "所属清单分组的 id"},
		},
		examples: []string{"dida project create --name 读书 --dry-run"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("project create", time.Now(), "create_project", args, output.Meta{})
		},
	})

	rt.attach(group, commandSpec{
		path:          "project update",
		summary:       "修改清单属性",
		operation:     schema.OpWrite,
		tools:         []string{"update_project"},
		allowBodyJSON: true,
		positional:    []schema.Param{{Name: "project-id", Required: true, Target: "project_id", Description: "清单 id"}},
		params: []schema.Param{
			{Name: "name", Type: "string", Target: "name", Description: "新的清单名"},
			{Name: "color", Type: "string", Target: "color", Description: "颜色"},
			{Name: "view-mode", Type: "string", Target: "view_mode", Description: "视图模式"},
			{Name: "kind", Type: "string", Target: "kind", Description: "清单类型"},
			{Name: "sort-order", Type: "int", Target: "sort_order", Description: "排序值"},
			{Name: "group-id", Type: "string", Target: "group_id", Description: "所属清单分组 id"},
			{Name: "closed", Type: "bool", Target: "closed", Description: "是否归档该清单"},
		},
		examples: []string{"dida project update inbox --name 新名字 --dry-run"},
		run: func(rt *Runtime, _ *cobra.Command, args map[string]any) error {
			return rt.callAndEmit("project update", time.Now(), "update_project", args, output.Meta{})
		},
	})
}
