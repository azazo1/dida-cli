package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/azazo1/dida-cli/internal/config"
)

// runCLI 在隔离的配置目录里完整执行一次 CLI, 返回退出码与解析后的信封.
//
// 这里刻意走 Execute 而不是直接调内部函数: 位置参数的声明方式与读取方式
// 是否一致, 只有把命令真正跑一遍才验得出来.
func runCLI(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(Options{
		Args:    args,
		Stdin:   strings.NewReader(""),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Version: "test",
	})
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("命令 %v 的输出不是 JSON 信封: %v\n输出: %s\nstderr: %s",
			args, err, stdout.String(), stderr.String())
	}
	return code, envelope
}

// isolateConfig 把配置目录指向临时目录, 避免测试碰到 user 的真实配置.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvToken, "")
}

// dataOf 取出信封的 data 字段.
func dataOf(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("信封里没有 data 对象: %#v", envelope)
	}
	return data
}

// TestConfigCommandsWork 是配置命令的端到端回归测试.
//
// 曾经有个 bug: config get 与 config set 的位置参数没有映射进请求体, 实现却
// 从请求体里取值, 结果永远读到空字符串, 报出 "unknown config key \"\"".
func TestConfigCommandsWork(t *testing.T) {
	isolateConfig(t)

	code, envelope := runCLI(t, "config", "get", "non_destructive")
	if code != 0 || envelope["ok"] != true {
		t.Fatalf("config get 失败: code=%d envelope=%#v", code, envelope)
	}
	if value := dataOf(t, envelope)["value"]; value != "false" {
		t.Fatalf("non_destructive 默认值 = %v, 期望 false", value)
	}

	code, envelope = runCLI(t, "config", "set", "non_destructive", "true")
	if code != 0 || envelope["ok"] != true {
		t.Fatalf("config set 失败: code=%d envelope=%#v", code, envelope)
	}
	if value := dataOf(t, envelope)["value"]; value != "true" {
		t.Fatalf("config set 回显 = %v, 期望 true", value)
	}

	// 重新执行一次, 确认取值真的落到了配置文件而不是只改了内存.
	code, envelope = runCLI(t, "config", "get", "non_destructive")
	if code != 0 {
		t.Fatalf("二次读取失败: code=%d", code)
	}
	if value := dataOf(t, envelope)["value"]; value != "true" {
		t.Fatalf("配置没有落盘, 读回 = %v", value)
	}
}

// TestConfigSetUnknownKeyIsRejected 确认错误路径给出的是可读的用法错误.
func TestConfigSetUnknownKeyIsRejected(t *testing.T) {
	isolateConfig(t)
	code, envelope := runCLI(t, "config", "set", "no_such_key", "true")
	if code != 2 {
		t.Fatalf("退出码 = %d, 期望 2 (用法错误)", code)
	}
	if envelope["ok"] != false {
		t.Fatal("非法配置项竟被接受")
	}
	errorBody, _ := envelope["error"].(map[string]any)
	if errorBody["type"] != "usage" {
		t.Fatalf("错误分类 = %v, 期望 usage", errorBody["type"])
	}
	if !strings.Contains(errorBody["message"].(string), "no_such_key") {
		t.Fatalf("错误信息没有指出具体的配置项名: %v", errorBody["message"])
	}
}

// TestLocalCommandsDoNotNeedToken 确认本地命令在没有口令时也能正常工作.
func TestLocalCommandsDoNotNeedToken(t *testing.T) {
	isolateConfig(t)
	cases := [][]string{
		{"config", "path"},
		{"config", "list"},
		{"version"},
		{"schema", "list"},
		{"schema", "show", "task", "create"},
	}
	// skill 刻意不套 JSON 信封, 由 TestSkillBypassesEnvelope 单独覆盖.
	for _, args := range cases {
		code, envelope := runCLI(t, args...)
		if code != 0 || envelope["ok"] != true {
			t.Errorf("命令 %v 失败: code=%d envelope=%#v", args, code, envelope)
		}
	}
}

// TestSchemaShowResolvesMultiWordCommand 确认多段命令路径能正确解析.
func TestSchemaShowResolvesMultiWordCommand(t *testing.T) {
	isolateConfig(t)
	code, envelope := runCLI(t, "schema", "show", "task", "create")
	if code != 0 {
		t.Fatalf("退出码 = %d, 期望 0", code)
	}
	if command := dataOf(t, envelope)["command"]; command != "task create" {
		t.Fatalf("解析出的命令 = %v, 期望 task create", command)
	}
}

// TestReadOnlyGateBlocksWritesEndToEnd 确认闸门在真实执行路径上生效.
func TestReadOnlyGateBlocksWritesEndToEnd(t *testing.T) {
	isolateConfig(t)
	code, envelope := runCLI(t, "--read-only", "task", "create", "--title", "x")
	if code != 7 {
		t.Fatalf("退出码 = %d, 期望 7 (被闸门拦下)", code)
	}
	errorBody, _ := envelope["error"].(map[string]any)
	if errorBody["type"] != "read_only" {
		t.Fatalf("错误分类 = %v, 期望 read_only", errorBody["type"])
	}
	// 提示里必须带上真实配置文件路径, 否则 user 不知道该去哪里改.
	hint, _ := errorBody["hint"].(string)
	if !strings.Contains(hint, config.ConfigPath()) {
		t.Fatalf("提示没有指出真实配置文件: %q", hint)
	}
}

// TestDryRunSurvivesGates 确认预览在只读模式下依然可用.
func TestDryRunSurvivesGates(t *testing.T) {
	isolateConfig(t)
	code, envelope := runCLI(t, "--read-only", "task", "create", "--title", "x", "--dry-run")
	if code != 0 {
		t.Fatalf("只读模式下 dry-run 被拦截: code=%d envelope=%#v", code, envelope)
	}
	meta, _ := envelope["meta"].(map[string]any)
	if meta["dry_run"] != true {
		t.Fatal("dry-run 没有生效")
	}
}

// TestSkillBypassesEnvelope 确认 skill 是唯一不套信封的命令.
func TestSkillBypassesEnvelope(t *testing.T) {
	isolateConfig(t)
	var stdout, stderr bytes.Buffer
	code := Execute(Options{
		Args:    []string{"skill", "--compact"},
		Stdin:   strings.NewReader(""),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Version: "test",
	})
	if code != 0 {
		t.Fatalf("skill 退出码 = %d", code)
	}
	if strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		t.Fatal("skill 不应当输出 JSON 信封")
	}
	if !strings.Contains(stdout.String(), "task create") {
		t.Fatal("精简索引里缺少命令")
	}
}
