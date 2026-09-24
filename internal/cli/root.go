package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/config"
	"github.com/azazo1/dida-cli/internal/guard"
	"github.com/azazo1/dida-cli/internal/logging"
	"github.com/azazo1/dida-cli/internal/output"
	"github.com/azazo1/dida-cli/internal/schema"
	"github.com/azazo1/dida-cli/internal/timeparse"
	"github.com/spf13/cobra"
)

// Options 是启动 CLI 所需的全部外部依赖, 全部显式传入以便测试替换.
type Options struct {
	Args    []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Version string
}

// Execute 是 CLI 的唯一入口, 返回进程退出码.
func Execute(opts Options) int {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	rt := &Runtime{
		Config:   config.Default(),
		Logger:   logging.Discard(),
		Registry: schema.NewRegistry(),
		Version:  opts.Version,
		Stdout:   opts.Stdout,
		Stderr:   opts.Stderr,
		Stdin:    opts.Stdin,
		ctx:      context.Background(),
	}
	// 先给一个兜底渲染器, 保证预处理阶段就失败时也能输出结构化错误.
	rt.Renderer = &output.Renderer{Writer: opts.Stdout, Format: output.FormatJSON}

	root := newRootCommand(rt)
	root.SetArgs(opts.Args)
	root.SetIn(opts.Stdin)
	root.SetOut(opts.Stdout)
	root.SetErr(opts.Stderr)

	if err := root.Execute(); err != nil {
		if errors.Is(err, errSilent) {
			return rt.ExitCode()
		}
		// 走到这里的是 cobra 层面的错误, 例如未知命令或未知参数.
		kind := apperr.KindOf(err)
		if kind == apperr.KindGeneral {
			err = apperr.Wrap(apperr.KindUsage, err, err.Error())
		}
		rt.Renderer.Failure(commandPath(root), err, output.Meta{})
		return apperr.ExitCodeOf(err)
	}
	return rt.ExitCode()
}

func newRootCommand(rt *Runtime) *cobra.Command {
	var (
		format         string
		jsonOutput     bool
		readOnly       bool
		nonDestructive bool
		logLevel       string
		configDir      string
		timeout        int
		compact        bool
	)

	root := &cobra.Command{
		Use:   "dida",
		Short: "面向 agent 的滴答清单命令行工具",
		Long: `dida 通过官方 MCP 通道操作滴答清单, 默认输出稳定的 JSON 信封.

给 agent 的使用建议:
  1. 先执行 dida skill --compact 获取命令索引, 或 dida skill 获取完整指南.
  2. 每条命令的参数契约用 dida schema show <command> 查询.
  3. 所有写命令都支持 --dry-run, 会先输出将要发送的请求体.
  4. 只读模式与非破坏性模式可以在配置文件里长期打开, 命令行无法覆盖.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       rt.Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return rt.bootstrap(bootstrapOptions{
				format:         format,
				jsonOutput:     jsonOutput,
				readOnly:       readOnly,
				nonDestructive: nonDestructive,
				logLevel:       logLevel,
				configDir:      configDir,
				timeout:        timeout,
				compact:        compact,
			})
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")

	flags := root.PersistentFlags()
	flags.StringVar(&format, "format", "auto", "输出格式, 取值为 auto / json / table")
	flags.BoolVar(&jsonOutput, "json", false, "等价于 --format json")
	flags.BoolVar(&readOnly, "read-only", false, "只读模式, 拒绝一切对账户的写操作")
	flags.BoolVar(&nonDestructive, "non-destructive", false, "非破坏性模式, 拒绝删除类操作")
	flags.StringVar(&logLevel, "log-level", "", "日志级别, 取值为 debug / info / warn / error")
	flags.StringVar(&configDir, "config-dir", "", "覆盖配置目录, 等价于设置 DIDA_CONFIG_DIR")
	flags.IntVar(&timeout, "timeout", 0, "单次上游请求超时秒数, 覆盖配置文件取值")
	flags.BoolVar(&compact, "compact", false, "精简输出, 只保留关键字段以节省 token")

	rt.attachGroups(root)
	return root
}

type bootstrapOptions struct {
	format         string
	jsonOutput     bool
	readOnly       bool
	nonDestructive bool
	logLevel       string
	configDir      string
	timeout        int
	compact        bool
}

// bootstrap 在命令真正执行前完成配置加载, 闸门解析与渲染器构造.
func (rt *Runtime) bootstrap(opts bootstrapOptions) error {
	if opts.configDir != "" {
		if err := os.Setenv(config.EnvConfigDir, opts.configDir); err != nil {
			return apperr.Wrap(apperr.KindGeneral, err, "设置配置目录失败")
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if opts.timeout > 0 {
		cfg.RequestTimeoutSeconds = opts.timeout
	}
	rt.Config = cfg
	rt.Mode = guard.Resolve(cfg, opts.readOnly, opts.nonDestructive)

	level := opts.logLevel
	if level == "" {
		level = cfg.LogLevel
	}
	rt.Logger = logging.New(logging.Options{Level: level, Writer: rt.Stderr})

	parsedFormat, err := output.ParseFormat(opts.format)
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		parsedFormat = output.FormatJSON
	}
	rt.Renderer = &output.Renderer{
		Writer:   rt.Stdout,
		Format:   parsedFormat,
		TTY:      isTerminal(rt.Stdout),
		ReadOnly: rt.Mode.ReadOnly.Enabled,
	}
	rt.Compact = opts.compact

	resolver, err := timeparse.New(cfg.Timezone)
	if err != nil {
		return err
	}
	rt.Resolver = resolver

	rt.Logger.Debug("运行环境就绪",
		"version", rt.Version,
		"config_dir", config.Dir(),
		"read_only", rt.Mode.ReadOnly.Enabled,
		"non_destructive", rt.Mode.NonDestructive.Enabled,
	)
	return nil
}

// attachGroups 挂载全部命令组.
func (rt *Runtime) attachGroups(root *cobra.Command) {
	rt.attachAuth(root)
	rt.attachConfig(root)
	rt.attachDoctor(root)
	rt.attachSkill(root)
	rt.attachSchema(root)
	rt.attachProject(root)
	rt.attachTask(root)
	rt.attachHabit(root)
	rt.attachFocus(root)
	rt.attachTag(root)
	rt.attachGroupCmds(root)
	rt.attachColumn(root)
	rt.attachComment(root)
	rt.attachCountdown(root)
	rt.attachView(root)
	rt.attachTool(root)
	rt.attachVersion(root)
}

// groupAnnotation 是打在纯归类命令上的标记.
//
// 分组命令只负责打印帮助, 不是可调用的能力, 因此不登记契约;
// 契约一致性测试依靠这个标记把它们排除在外.
const groupAnnotation = "dida.io/group"

// group 创建一个只用于归类的命令组.
func (rt *Runtime) group(parent *cobra.Command, use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{groupAnnotation: "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	parent.AddCommand(cmd)
	return cmd
}

func commandPath(root *cobra.Command) string {
	parts := strings.Fields(root.CommandPath())
	if len(parts) <= 1 {
		return "dida"
	}
	return strings.Join(parts[1:], " ")
}

// isTerminal 判断输出目标是不是终端.
func isTerminal(writer io.Writer) bool { return isCharDevice(writer) }

// isTerminalReader 判断输入来源是不是终端.
func isTerminalReader(reader io.Reader) bool { return isCharDevice(reader) }

func isCharDevice(value any) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
