// Package schema 维护命令契约表.
//
// 契约表是 agent 理解这个 CLI 的唯一权威来源: 每条命令对应哪个官方工具,
// 属于读还是写, 需要哪些参数, 是否支持 dry-run, 都在这里声明一次,
// 命令实现与 schema 输出共用同一份数据, 不允许各写一份.
package schema

import (
	"sort"
	"strings"

	"github.com/azazo1/dida-cli/internal/apperr"
)

// Operation 是命令对账户数据的影响类型.
type Operation string

const (
	// OpRead 表示只读.
	OpRead Operation = "read"
	// OpWrite 表示可逆写入.
	OpWrite Operation = "write"
	// OpDestructive 表示破坏性操作.
	OpDestructive Operation = "destructive"
	// OpDynamic 表示操作性质要到运行时才能确定.
	//
	// 只有 dida tool call 用这个取值: 它调用哪个官方工具由参数决定,
	// 因此闸门必须依据工具自身的注解在运行时判定.
	OpDynamic Operation = "dynamic"
)

// Param 描述一个命令参数.
type Param struct {
	// Name 是对外参数名, 同时用作 flag 名.
	Name string `json:"name"`
	// Type 是参数类型, 取值为 string / int / bool / json / string-list.
	Type string `json:"type"`
	// Required 表示是否必填.
	Required bool `json:"required,omitempty"`
	// Default 是默认值的文本形式.
	Default string `json:"default,omitempty"`
	// Description 是参数说明.
	Description string `json:"description,omitempty"`
	// Target 是发给官方工具时的参数名, 为空表示该参数不进入请求体.
	Target string `json:"target,omitempty"`
	// Positional 表示该参数是位置参数而不是 flag.
	Positional bool `json:"positional,omitempty"`
}

// FlagParams 返回需要注册为 flag 的参数.
func (e Entry) FlagParams() []Param {
	params := make([]Param, 0, len(e.Params))
	for _, param := range e.Params {
		if !param.Positional {
			params = append(params, param)
		}
	}
	return params
}

// PositionalParams 返回位置参数.
func (e Entry) PositionalParams() []Param {
	params := make([]Param, 0, len(e.Params))
	for _, param := range e.Params {
		if param.Positional {
			params = append(params, param)
		}
	}
	return params
}

// Entry 是一条命令契约.
type Entry struct {
	// Command 是完整命令路径, 例如 "task create".
	Command string `json:"command"`
	// Summary 是一句话说明.
	Summary string `json:"summary"`
	// Operation 是操作类型.
	Operation Operation `json:"operation"`
	// Tools 是该命令调用的官方工具名, 聚合命令可能有多个.
	Tools []string `json:"tools,omitempty"`
	// Params 是参数表.
	Params []Param `json:"params,omitempty"`
	// SupportsDryRun 表示写命令是否支持先预览请求体.
	SupportsDryRun bool `json:"dry_run,omitempty"`
	// NeedsConfirm 表示破坏性命令是否需要 --yes.
	NeedsConfirm bool `json:"confirmation_required,omitempty"`
	// Notes 是值得 agent 知道的补充信息.
	Notes string `json:"notes,omitempty"`
	// Examples 是可直接复制的示例.
	Examples []string `json:"examples,omitempty"`
}

// Registry 是契约表的集合.
type Registry struct {
	entries map[string]Entry
	order   []string
}

// NewRegistry 构造一个空契约表.
func NewRegistry() *Registry {
	return &Registry{entries: map[string]Entry{}}
}

// Add 登记一条契约, 命令路径重复时直接报错, 避免出现两套定义.
func (r *Registry) Add(entry Entry) {
	key := strings.TrimSpace(entry.Command)
	if _, exists := r.entries[key]; exists {
		panic("schema: duplicate command contract: " + key)
	}
	r.entries[key] = entry
	r.order = append(r.order, key)
}

// Get 按命令路径取契约.
func (r *Registry) Get(command string) (Entry, error) {
	key := strings.Join(strings.Fields(command), " ")
	if entry, ok := r.entries[key]; ok {
		return entry, nil
	}
	return Entry{}, apperr.NotFound("no command contract named " + key).
		WithHint("run: dida schema list")
}

// All 返回全部契约, 按命令路径排序.
func (r *Registry) All() []Entry {
	keys := make([]string, len(r.order))
	copy(keys, r.order)
	sort.Strings(keys)
	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, r.entries[key])
	}
	return entries
}

// Len 返回契约条数.
func (r *Registry) Len() int { return len(r.entries) }
