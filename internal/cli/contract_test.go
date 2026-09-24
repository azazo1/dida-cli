package cli

import (
	"context"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/azazo1/dida-cli/internal/config"
	"github.com/azazo1/dida-cli/internal/logging"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/azazo1/dida-cli/internal/skill"
	"github.com/spf13/cobra"
)

// buildTestRoot 构造一棵完整的命令树用于结构断言, 不执行任何命令.
func buildTestRoot(t *testing.T) (*Runtime, *cobra.Command) {
	t.Helper()
	rt := &Runtime{
		Config:   config.Default(),
		Logger:   logging.Discard(),
		Registry: schema.NewRegistry(),
		Stdout:   io.Discard,
		Stderr:   io.Discard,
		Stdin:    strings.NewReader(""),
		ctx:      context.Background(),
	}
	root := newRootCommand(rt)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return rt, root
}

// collectCommandPaths 收集命令树里全部可调用命令的路径.
//
// 纯归类命令只打印帮助, 不算能力, 因此按标记排除.
func collectCommandPaths(root *cobra.Command) map[string]*cobra.Command {
	paths := map[string]*cobra.Command{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			// help 与 completion 由 cobra 自动注入, 不属于本项目的契约范围.
			if child.Name() == "help" || child.Name() == "completion" {
				continue
			}
			if child.Annotations[groupAnnotation] != "true" && (child.RunE != nil || child.Run != nil) {
				path := strings.TrimPrefix(strings.TrimSpace(child.CommandPath()), "dida ")
				paths[path] = child
			}
			walk(child)
		}
	}
	walk(root)
	return paths
}

// TestEveryContractHasCommand 确认契约表与命令树一一对应.
//
// 这两份数据一旦漂移, agent 按 schema 调命令就会踩空, 所以必须锁死.
func TestEveryContractHasCommand(t *testing.T) {
	rt, root := buildTestRoot(t)
	paths := collectCommandPaths(root)

	for _, entry := range rt.Registry.All() {
		if _, ok := paths[entry.Command]; !ok {
			t.Errorf("契约 %q 在命令树里不存在", entry.Command)
		}
	}
	// 反向检查: 命令树里不该出现没登记契约的命令.
	registered := map[string]bool{}
	for _, entry := range rt.Registry.All() {
		registered[entry.Command] = true
	}
	for path := range paths {
		if !registered[path] {
			t.Errorf("命令 %q 没有登记契约", path)
		}
	}
}

// TestWriteCommandsSupportDryRun 确认所有写命令都能先预览.
func TestWriteCommandsSupportDryRun(t *testing.T) {
	rt, root := buildTestRoot(t)
	paths := collectCommandPaths(root)

	for _, entry := range rt.Registry.All() {
		cmd := paths[entry.Command]
		if cmd == nil {
			continue
		}
		hasDryRun := cmd.Flags().Lookup("dry-run") != nil
		wantsDryRun := entry.Operation != schema.OpRead
		if hasDryRun != wantsDryRun {
			t.Errorf("%s: dry-run flag 存在 = %v, 期望 %v", entry.Command, hasDryRun, wantsDryRun)
		}
		hasYes := cmd.Flags().Lookup("yes") != nil
		wantsYes := entry.Operation == schema.OpDestructive || entry.Operation == schema.OpDynamic
		if hasYes != wantsYes {
			t.Errorf("%s: yes flag 存在 = %v, 期望 %v", entry.Command, hasYes, wantsYes)
		}
	}
}

// TestDestructiveCommandsDeclareConfirmation 确认破坏性命令都要求确认.
func TestDestructiveCommandsDeclareConfirmation(t *testing.T) {
	rt, _ := buildTestRoot(t)
	for _, entry := range rt.Registry.All() {
		if entry.Operation != schema.OpDestructive {
			continue
		}
		if !entry.NeedsConfirm {
			t.Errorf("破坏性命令 %q 没有声明需要确认", entry.Command)
		}
		if !entry.SupportsDryRun {
			t.Errorf("破坏性命令 %q 没有声明支持 dry-run", entry.Command)
		}
	}
}

// TestNoDuplicateCommands 确认没有两条契约指向同一个命令路径.
func TestNoDuplicateCommands(t *testing.T) {
	rt, _ := buildTestRoot(t)
	seen := map[string]bool{}
	for _, entry := range rt.Registry.All() {
		if seen[entry.Command] {
			t.Errorf("命令契约重复: %q", entry.Command)
		}
		seen[entry.Command] = true
	}
}

// TestSkillGuideCoversEveryGroup 确认使用指南覆盖了命令树的每一个顶层组.
//
// 指南是 agent 了解这个工具的第一入口, 缺了某个组会让能力被埋没.
func TestSkillGuideCoversEveryGroup(t *testing.T) {
	_, root := buildTestRoot(t)

	groups := make([]string, 0, 16)
	for _, child := range root.Commands() {
		if child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		groups = append(groups, child.Name())
	}
	sort.Strings(groups)

	for _, group := range groups {
		if !strings.Contains(skill.Guide, group) {
			t.Errorf("使用指南里没有提到命令组 %q", group)
		}
	}
}

// TestCompactIndexListsAllCommands 确认精简索引来自契约表且不遗漏命令.
func TestCompactIndexListsAllCommands(t *testing.T) {
	rt, _ := buildTestRoot(t)
	index := compactIndex(rt)
	for _, entry := range rt.Registry.All() {
		if !strings.Contains(index, entry.Command) {
			t.Errorf("精简索引里缺少命令 %q", entry.Command)
		}
	}
}

// TestSkillGuideStaysOutOfJSONEnvelope 确认 skill 命令的输出契约是刻意例外.
func TestSkillGuideStaysOutOfJSONEnvelope(t *testing.T) {
	if strings.HasPrefix(strings.TrimSpace(skill.Guide), "{") {
		t.Fatal("使用指南应当是 Markdown 文档而不是 JSON")
	}
	if !strings.Contains(skill.Guide, "# dida 使用指南") {
		t.Fatal("使用指南缺少标题")
	}
}

// TestFlagParamsAndPositionalsAreSeparated 确认位置参数不会被注册成 flag.
func TestFlagParamsAndPositionalsAreSeparated(t *testing.T) {
	rt, root := buildTestRoot(t)
	paths := collectCommandPaths(root)

	for _, entry := range rt.Registry.All() {
		cmd := paths[entry.Command]
		if cmd == nil {
			continue
		}
		for _, param := range entry.Params {
			flag := cmd.Flags().Lookup(param.Name)
			if param.Positional && flag != nil {
				t.Errorf("%s: 位置参数 %q 被错误地注册成了 flag", entry.Command, param.Name)
			}
			if !param.Positional && flag == nil {
				t.Errorf("%s: 参数 %q 没有注册成 flag", entry.Command, param.Name)
			}
		}
	}
}
