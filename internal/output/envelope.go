// Package output 负责把命令结果渲染成稳定的 JSON 信封或人类可读表格.
//
// JSON 信封是 agent 的主通道, 成功与失败使用同一形状, 便于统一解析.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/azazo1/dida-cli/internal/apperr"
)

// Format 是输出格式.
type Format string

const (
	// FormatAuto 在 stdout 是终端时用表格, 否则用 JSON.
	FormatAuto Format = "auto"
	// FormatJSON 强制输出 JSON 信封.
	FormatJSON Format = "json"
	// FormatTable 强制输出人类可读表格.
	FormatTable Format = "table"
)

// ParseFormat 解析输出格式文本.
func ParseFormat(text string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "", "auto":
		return FormatAuto, nil
	case "json":
		return FormatJSON, nil
	case "table", "text":
		return FormatTable, nil
	default:
		return "", apperr.Usagef("unknown output format %q, expected auto, json or table", text)
	}
}

// Envelope 是统一的 JSON 输出信封.
type Envelope struct {
	OK      bool       `json:"ok"`
	Command string     `json:"command"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
	Meta    Meta       `json:"meta"`
}

// ErrorBody 描述失败原因, hint 是一条可以直接照做的修复建议.
type ErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Meta 携带与结果本身无关但影响解读的执行元信息.
type Meta struct {
	Count      int    `json:"count,omitempty"`
	ReadOnly   bool   `json:"read_only"`
	DryRun     bool   `json:"dry_run,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Unparsed   bool   `json:"unparsed_text,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// Renderer 持有渲染所需的固定信息.
type Renderer struct {
	// Writer 是渲染目标, 通常是 stdout.
	Writer io.Writer
	// Format 是解析后的输出格式.
	Format Format
	// TTY 表示渲染目标是否为终端, 仅在 FormatAuto 时参与决策.
	TTY bool
	// ReadOnly 表示当前是否处于只读模式, 会写进每个信封的 meta.
	ReadOnly bool
}

// Success 渲染一个成功信封.
func (r *Renderer) Success(command string, data any, meta Meta) error {
	meta.ReadOnly = r.ReadOnly
	envelope := Envelope{OK: true, Command: command, Data: data, Meta: meta}
	return r.render(envelope)
}

// Failure 渲染一个失败信封, 返回错误对应的退出码.
func (r *Renderer) Failure(command string, err error, meta Meta) int {
	kind, message, hint := apperr.Detail(err)
	meta.ReadOnly = r.ReadOnly
	envelope := Envelope{
		OK:      false,
		Command: command,
		Error:   &ErrorBody{Type: string(kind), Message: message, Hint: hint},
		Meta:    meta,
	}
	// 失败信封的渲染错误只能被忽略, 否则会覆盖掉真正有价值的错误信息.
	_ = r.render(envelope)
	return kind.ExitCode()
}

func (r *Renderer) render(envelope Envelope) error {
	if r.Writer == nil {
		return fmt.Errorf("output writer is nil")
	}
	if r.useTable() {
		return r.renderTable(envelope)
	}
	encoder := json.NewEncoder(r.Writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(envelope)
}

func (r *Renderer) useTable() bool {
	switch r.Format {
	case FormatTable:
		return true
	case FormatJSON:
		return false
	default:
		return r.TTY
	}
}

// Count 是给命令实现用的辅助函数, 统计结果条数写入 meta.count.
func Count(data any) int {
	switch value := data.(type) {
	case nil:
		return 0
	case []any:
		return len(value)
	case []map[string]any:
		return len(value)
	case map[string]any:
		if items, ok := value["tasks"].([]any); ok {
			return len(items)
		}
		if items, ok := value["projects"].([]any); ok {
			return len(items)
		}
		return 1
	default:
		return 1
	}
}
