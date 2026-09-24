package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/azazo1/dida-cli/internal/apperr"
)

// useTempDir 把配置目录指向临时目录, 保证测试不碰 user 的真实配置.
func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvConfigDir, dir)
	t.Setenv(EnvToken, "")
	return dir
}

func TestLoadWithoutFileReturnsDefaults(t *testing.T) {
	dir := useTempDir(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Version != CurrentVersion {
		t.Fatalf("版本 = %d, 期望 %d", cfg.Version, CurrentVersion)
	}
	if cfg.Endpoint != DefaultEndpoint {
		t.Fatalf("端点 = %q", cfg.Endpoint)
	}
	// 读配置不该产生副作用, 否则只读命令也会写盘.
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); !os.IsNotExist(err) {
		t.Fatal("读取配置时不应创建配置文件")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	useTempDir(t)
	cfg := Default()
	cfg.ReadOnly = true
	cfg.NonDestructive = true
	cfg.LogLevel = "debug"
	if err := Save(cfg); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	info, err := os.Stat(ConfigPath())
	if err != nil {
		t.Fatalf("配置文件不存在: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("配置文件权限 = %o, 期望 600", info.Mode().Perm())
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if !loaded.ReadOnly || !loaded.NonDestructive {
		t.Fatalf("闸门配置没有正确往返: %+v", loaded)
	}
	if loaded.LogLevel != "debug" {
		t.Fatalf("日志级别 = %q", loaded.LogLevel)
	}
}

// TestLoadMigratesOlderVersion 确认旧版本配置会被迁移并回写.
func TestLoadMigratesOlderVersion(t *testing.T) {
	useTempDir(t)
	// 模拟一份早期版本留下的配置文件: 版本号为 0, 且缺少后来才加的字段.
	legacy := "version = 0\nread_only = true\n"
	if err := os.WriteFile(ConfigPath(), []byte(legacy), 0o600); err != nil {
		t.Fatalf("写入旧配置失败: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Version != CurrentVersion {
		t.Fatalf("迁移后版本 = %d, 期望 %d", cfg.Version, CurrentVersion)
	}
	// 旧文件里已有的取值必须保留, 迁移不是重置.
	if !cfg.ReadOnly {
		t.Fatal("迁移把 user 已设的 read_only 弄丢了")
	}
	// 缺失字段要补上默认值, 否则后续请求会打到空端点上.
	if cfg.Endpoint != DefaultEndpoint {
		t.Fatalf("迁移后端点 = %q", cfg.Endpoint)
	}
	if cfg.RequestTimeoutSeconds != Default().RequestTimeoutSeconds {
		t.Fatalf("迁移后超时 = %d", cfg.RequestTimeoutSeconds)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("二次加载失败: %v", err)
	}
	if reloaded.Version != CurrentVersion {
		t.Fatalf("迁移结果没有被回写, 版本仍为 %d", reloaded.Version)
	}
}

// TestLoadRejectsNewerVersion 确认来自更新版本的配置不会被静默降级处理.
func TestLoadRejectsNewerVersion(t *testing.T) {
	useTempDir(t)
	content := "version = " + itoa(CurrentVersion+1) + "\n"
	if err := os.WriteFile(ConfigPath(), []byte(content), 0o600); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("更新版本的配置本应被拒绝")
	}
}

func TestFieldsRoundTripOwnValues(t *testing.T) {
	useTempDir(t)
	cfg := Default()
	for _, field := range Fields() {
		// 允许空默认值: 例如 default_project 为空表示"未指定时用收集箱".
		value := field.Get(cfg)
		if err := field.Set(cfg, value); err != nil {
			t.Fatalf("配置项 %q 无法写回自己的取值 %q: %v", field.Name, value, err)
		}
		if got := field.Get(cfg); got != value {
			t.Fatalf("配置项 %q 往返后取值 = %q, 期望 %q", field.Name, got, value)
		}
	}
	if _, err := FindField("no_such_key"); err == nil {
		t.Fatal("未知配置项本应被拒绝")
	}
}

func TestSaveTokenRejectsForeignToken(t *testing.T) {
	useTempDir(t)
	if err := SaveToken("not-a-dida-token"); err == nil {
		t.Fatal("非 dp_ 前缀的口令本应被拒绝")
	}
	if err := SaveToken("   "); err == nil {
		t.Fatal("空口令本应被拒绝")
	}
}

func TestTokenFileLifecycle(t *testing.T) {
	dir := useTempDir(t)
	if err := SaveToken("dp_abcdefghijklmnop"); err != nil {
		t.Fatalf("保存口令失败: %v", err)
	}
	info, err := os.Stat(TokenPath())
	if err != nil {
		t.Fatalf("口令文件不存在: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("口令文件权限 = %o, 期望 600", info.Mode().Perm())
	}
	if dirInfo, err := os.Stat(dir); err == nil && dirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("配置目录权限过宽: %o", dirInfo.Mode().Perm())
	}

	token, status, err := ResolveToken()
	if err != nil {
		t.Fatalf("解析口令失败: %v", err)
	}
	if token != "dp_abcdefghijklmnop" {
		t.Fatalf("口令 = %q", token)
	}
	if status.Source != TokenSourceFile {
		t.Fatalf("口令来源 = %q, 期望 file", status.Source)
	}
	if status.Preview == token {
		t.Fatal("预览没有脱敏")
	}
	if status.Permissive {
		t.Fatal("0600 的文件不该被判定为权限过宽")
	}

	if err := ClearToken(); err != nil {
		t.Fatalf("清除口令失败: %v", err)
	}
	if _, err := os.Stat(TokenPath()); !os.IsNotExist(err) {
		t.Fatal("口令文件应当已被删除")
	}
	// 重复清除不应报错, 否则脚本里不好收尾.
	if err := ClearToken(); err != nil {
		t.Fatalf("重复清除口令报错: %v", err)
	}
}

func TestEnvTokenWins(t *testing.T) {
	useTempDir(t)
	if err := SaveToken("dp_from_file_token"); err != nil {
		t.Fatalf("保存口令失败: %v", err)
	}
	t.Setenv(EnvToken, "dp_from_env_token")
	token, status, err := ResolveToken()
	if err != nil {
		t.Fatalf("解析口令失败: %v", err)
	}
	if token != "dp_from_env_token" {
		t.Fatalf("口令 = %q, 环境变量应当优先", token)
	}
	if status.Source != TokenSourceEnv {
		t.Fatalf("口令来源 = %q, 期望 env", status.Source)
	}
}

// TestMissingTokenReportsHowToFix 确认口令缺失时给出的补救提示是可直接照做的.
//
// 提示走 JSON 信封的 error.hint 字段, 而不是错误文本本身.
func TestMissingTokenReportsHowToFix(t *testing.T) {
	useTempDir(t)
	_, _, err := ResolveToken()
	if err == nil {
		t.Fatal("没有口令时本应报错")
	}
	kind, _, hint := apperr.Detail(err)
	if kind != apperr.KindAuth {
		t.Fatalf("错误分类 = %v, 期望 auth", kind)
	}
	if !contains(hint, "auth login") {
		t.Fatalf("提示没有给出补救命令: %q", hint)
	}
}

func TestParseTokenContent(t *testing.T) {
	content := "# 注释行\n\ndp_real_token\n"
	if token := parseTokenContent(content); token != "dp_real_token" {
		t.Fatalf("解析结果 = %q", token)
	}
	if token := parseTokenContent("\n\n# 只有注释\n"); token != "" {
		t.Fatalf("只有注释时应返回空, 实际 %q", token)
	}
}

func TestRedactToken(t *testing.T) {
	if got := RedactToken("dp_short"); got != "***" {
		t.Fatalf("短口令应整体打码, 实际 %q", got)
	}
	got := RedactToken("dp_abcdefghijklmnop")
	if got == "dp_abcdefghijklmnop" {
		t.Fatal("长口令没有被脱敏")
	}
	if !contains(got, "...") {
		t.Fatalf("脱敏结果应当包含省略号, 实际 %q", got)
	}
}

func contains(text, part string) bool {
	for index := 0; index+len(part) <= len(text); index++ {
		if text[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
