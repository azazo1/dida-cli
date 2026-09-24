package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/azazo1/dida-cli/internal/config"
	"github.com/azazo1/dida-cli/internal/guard"
	"github.com/azazo1/dida-cli/internal/official"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/azazo1/dida-cli/internal/timeparse"
)

// errSilent 表示错误已经渲染成信封, 不需要再被上层打印一次.
var errSilent = errors.New("silent")

// Runtime 是所有命令共享的执行上下文.
type Runtime struct {
	// Config 是当前生效的配置.
	Config *config.Config
	// Mode 是当前生效的安全闸门.
	Mode guard.Mode
	// Logger 只写 stderr, 保证 stdout 的 JSON 通道干净.
	Logger *slog.Logger
	// Renderer 负责把结果渲染成信封.
	Renderer *output.Renderer
	// Registry 是命令契约表.
	Registry *schema.Registry
	// Resolver 是时间解析器.
	Resolver *timeparse.Resolver
	// Version 是程序版本, 由构建时注入.
	Version string
	// Compact 表示是否精简输出.
	Compact bool

	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	ctx      context.Context
	client   *official.Client
	exitCode int
}

// ExitCode 返回本进程应当使用的退出码.
func (rt *Runtime) ExitCode() int { return rt.exitCode }

// Client 返回官方 MCP 客户端, 首次调用时才解析口令并建立连接.
func (rt *Runtime) Client(ctx context.Context) (*official.Client, error) {
	if rt.client != nil {
		return rt.client, nil
	}
	token, status, err := config.ResolveToken()
	if err != nil {
		return nil, err
	}
	if status.Permissive {
		rt.Logger.Warn("口令文件权限过宽, 建议收紧为 0600", "path", status.Path, "mode", status.Mode)
	}
	endpoint := rt.Config.Endpoint
	if override := endpointOverride(); override != "" {
		endpoint = override
	}
	rt.client = official.New(official.Options{
		Endpoint:      endpoint,
		Token:         token,
		Timeout:       time.Duration(rt.Config.RequestTimeoutSeconds) * time.Second,
		Logger:        rt.Logger,
		ClientVersion: rt.Version,
		Cache: official.CacheOptions{
			Enabled: rt.Config.CacheEnabled,
			Path:    config.ToolCachePath(),
			TTL:     time.Duration(rt.Config.CacheTTLSeconds) * time.Second,
		},
	})
	rt.Logger.Debug("官方通道客户端就绪", "endpoint", endpoint, "token_source", status.Source)
	return rt.client, nil
}

// Emit 渲染一个成功信封.
func (rt *Runtime) Emit(command string, started time.Time, data any, meta output.Meta) error {
	meta.DurationMS = time.Since(started).Milliseconds()
	return rt.Renderer.Success(command, data, meta)
}

// EmitData 渲染命令的业务数据, 自动应用 --compact 并统计条数.
func (rt *Runtime) EmitData(command string, started time.Time, data any, meta output.Meta) error {
	if rt.Compact {
		data = output.Compact(data)
	}
	meta.Count = output.Count(data)
	return rt.Emit(command, started, data, meta)
}

// Fail 渲染一个失败信封并记录退出码, 返回 errSilent 以终止命令执行.
func (rt *Runtime) Fail(command string, started time.Time, err error) error {
	rt.exitCode = rt.Renderer.Failure(command, err, output.Meta{DurationMS: time.Since(started).Milliseconds()})
	// 失败一律留痕, 便于事后复现 agent 到底卡在哪一步.
	rt.Logger.Warn("命令失败", "command", command, "error", err)
	return errSilent
}

// Call 调用一个官方工具并把结果数据返回给调用方.
func (rt *Runtime) Call(ctx context.Context, tool string, args map[string]any) (any, error) {
	client, err := rt.Client(ctx)
	if err != nil {
		return nil, err
	}
	result, err := client.Call(ctx, tool, args)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

// CallResult 与 Call 相同, 但把完整结果返回, 便于上层处理未解析文本.
func (rt *Runtime) CallResult(ctx context.Context, tool string, args map[string]any) (*official.Result, error) {
	client, err := rt.Client(ctx)
	if err != nil {
		return nil, err
	}
	return client.Call(ctx, tool, args)
}

// Log 返回带命令上下文的日志器.
func (rt *Runtime) Log() *slog.Logger { return rt.Logger }
