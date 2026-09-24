package guard

import (
	"errors"
	"testing"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/config"
	"github.com/azazo1/dida-cli/internal/official"
)

func boolPtr(value bool) *bool { return &value }

func TestResolveCombinesAllSources(t *testing.T) {
	cases := []struct {
		name           string
		configValue    bool
		envValue       string
		flagValue      bool
		wantEnabled    bool
		wantSourcePart string
	}{
		{name: "全部关闭", wantEnabled: false},
		{name: "来自配置", configValue: true, wantEnabled: true, wantSourcePart: "config"},
		{name: "来自环境变量", envValue: "1", wantEnabled: true, wantSourcePart: "env:" + config.EnvReadOnly},
		{name: "来自命令行", flagValue: true, wantEnabled: true, wantSourcePart: "flag"},
		{name: "环境变量写 0 不生效", envValue: "0", wantEnabled: false},
		{name: "配置与环境变量叠加", configValue: true, envValue: "true", flagValue: true, wantEnabled: true, wantSourcePart: "config+"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv(config.EnvReadOnly, item.envValue)
			cfg := config.Default()
			cfg.ReadOnly = item.configValue
			mode := Resolve(cfg, item.flagValue, false)
			if mode.ReadOnly.Enabled != item.wantEnabled {
				t.Fatalf("闸门状态 = %v, 期望 %v", mode.ReadOnly.Enabled, item.wantEnabled)
			}
			if item.wantSourcePart != "" {
				if mode.ReadOnly.Source == "" {
					t.Fatal("闸门来源不应为空")
				}
				if !contains(mode.ReadOnly.Source, item.wantSourcePart) {
					t.Fatalf("闸门来源 = %q, 期望包含 %q", mode.ReadOnly.Source, item.wantSourcePart)
				}
			}
		})
	}
}

// TestResolveHasNoOffSwitch 确认闸门只能收紧不能放松.
//
// 如果命令行能把配置里打开的闸门关掉, 那么 agent 就可以自我提权, 这道闸
// 对 agent 而言等于不存在.
func TestResolveHasNoOffSwitch(t *testing.T) {
	t.Setenv(config.EnvReadOnly, "")
	cfg := config.Default()
	cfg.ReadOnly = true
	mode := Resolve(cfg, false, false)
	if !mode.ReadOnly.Enabled {
		t.Fatal("配置里打开的闸门被命令行关掉了")
	}
}

func TestCheckBlocksWritesInReadOnly(t *testing.T) {
	mode := Mode{ReadOnly: Gate{Enabled: true, Source: "config"}, ConfigPath: "/tmp/config.toml"}

	if err := mode.Check(Operation{ReadOnly: true}, "dida task list"); err != nil {
		t.Fatalf("只读操作被拦截: %v", err)
	}
	err := mode.Check(Operation{ReadOnly: false}, "dida task create")
	if err == nil {
		t.Fatal("只读模式下写操作没有被拦截")
	}
	if apperr.KindOf(err) != apperr.KindReadOnly {
		t.Fatalf("错误分类 = %v, 期望 read_only", apperr.KindOf(err))
	}
	if apperr.ExitCodeOf(err) != 7 {
		t.Fatalf("退出码 = %d, 期望 7", apperr.ExitCodeOf(err))
	}
}

func TestCheckBlocksDestructiveOnly(t *testing.T) {
	mode := Mode{NonDestructive: Gate{Enabled: true, Source: "flag"}, ConfigPath: "/tmp/config.toml"}

	if err := mode.Check(Operation{ReadOnly: false, Destructive: false}, "dida task create"); err != nil {
		t.Fatalf("非破坏性写入被拦截: %v", err)
	}
	err := mode.Check(Operation{ReadOnly: false, Destructive: true}, "dida task delete")
	if err == nil {
		t.Fatal("非破坏性模式下删除操作没有被拦截")
	}
	if apperr.KindOf(err) != apperr.KindNonDestructive {
		t.Fatalf("错误分类 = %v, 期望 non_destructive", apperr.KindOf(err))
	}
}

// TestCheckReportsReadOnlyFirst 确认两道闸门同时开启时报出更严格的那道.
func TestCheckReportsReadOnlyFirst(t *testing.T) {
	mode := Mode{
		ReadOnly:       Gate{Enabled: true, Source: "config"},
		NonDestructive: Gate{Enabled: true, Source: "config"},
	}
	err := mode.Check(Operation{ReadOnly: false, Destructive: true}, "dida task delete")
	if apperr.KindOf(err) != apperr.KindReadOnly {
		t.Fatalf("错误分类 = %v, 期望先报 read_only", apperr.KindOf(err))
	}
}

func TestCheckToolUsesAnnotations(t *testing.T) {
	mode := Mode{ReadOnly: Gate{Enabled: true, Source: "config"}, ConfigPath: "/tmp/config.toml"}

	readTool := official.Tool{Name: "list_projects", Annotations: official.Annotations{ReadOnlyHint: boolPtr(true)}}
	if err := mode.CheckTool(readTool, "dida tool call list_projects"); err != nil {
		t.Fatalf("只读工具被拦截: %v", err)
	}

	writeTool := official.Tool{Name: "create_task", Annotations: official.Annotations{ReadOnlyHint: boolPtr(false)}}
	if err := mode.CheckTool(writeTool, "dida tool call create_task"); err == nil {
		t.Fatal("写工具没有被拦截")
	}
}

// TestCheckToolIsConservativeWithoutAnnotations 确认注解缺失时按最保守处理.
func TestCheckToolIsConservativeWithoutAnnotations(t *testing.T) {
	mode := Mode{ReadOnly: Gate{Enabled: true, Source: "config"}}
	unknown := official.Tool{Name: "mystery_tool"}
	if err := mode.CheckTool(unknown, "dida tool call mystery_tool"); err == nil {
		t.Fatal("注解缺失的工具在只读模式下被放行了")
	}
	if !unknown.Destructive() {
		t.Fatal("注解缺失的工具应当被保守地视为破坏性")
	}
}

func TestRequireConfirm(t *testing.T) {
	destructive := Operation{ReadOnly: false, Destructive: true}
	err := RequireConfirm(destructive, "dida task delete", false)
	if err == nil {
		t.Fatal("破坏性操作没有确认就放行了")
	}
	if apperr.KindOf(err) != apperr.KindConfirm {
		t.Fatalf("错误分类 = %v, 期望 confirmation_required", apperr.KindOf(err))
	}
	if apperr.ExitCodeOf(err) != 6 {
		t.Fatalf("退出码 = %d, 期望 6", apperr.ExitCodeOf(err))
	}
	if err := RequireConfirm(destructive, "dida task delete", true); err != nil {
		t.Fatalf("确认后仍被拦截: %v", err)
	}
	if err := RequireConfirm(Operation{ReadOnly: false}, "dida task create", false); err != nil {
		t.Fatalf("非破坏性操作不应当要求确认: %v", err)
	}
}

// TestHintPointsAtRealConfigFile 确认错误提示能指到真实配置文件.
func TestHintPointsAtRealConfigFile(t *testing.T) {
	mode := Mode{ReadOnly: Gate{Enabled: true, Source: "flag"}, ConfigPath: "/somewhere/config.toml"}
	err := mode.Check(Operation{}, "dida task create")
	var target *apperr.Error
	if !errors.As(err, &target) {
		t.Fatalf("错误类型 = %T", err)
	}
	if !contains(target.Hint, "/somewhere/config.toml") {
		t.Fatalf("提示没有指出真实配置文件: %q", target.Hint)
	}
	if !contains(target.Hint, "flag") {
		t.Fatalf("提示没有指出闸门来源: %q", target.Hint)
	}
}

func contains(text, part string) bool {
	return len(part) == 0 || (len(text) >= len(part) && indexOf(text, part) >= 0)
}

func indexOf(text, part string) int {
	for index := 0; index+len(part) <= len(text); index++ {
		if text[index:index+len(part)] == part {
			return index
		}
	}
	return -1
}
