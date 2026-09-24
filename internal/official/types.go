// Package official 封装滴答清单官方 MCP 通道.
//
// 这一层只做三件事: 按 MCP 协议说话, 把工具目录取回来, 把工具调用结果
// 拆成"可用的数据"或"明确的错误". 业务语义由上层命令负责.
package official

import (
	"encoding/json"
	"strings"

	"github.com/azazo1/dida-cli/internal/apperr"
)

// DefaultEndpoint 是官方 MCP 端点.
const DefaultEndpoint = "https://mcp.dida365.com"

// 协议版本: 握手用较新的版本, 后续请求头带上服务端已接受的版本.
const (
	initializeProtocolVersion = "2025-03-26"
	requestProtocolVersion    = "2024-11-05"
)

// Tool 是官方 MCP 暴露的一个工具.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Annotations Annotations    `json:"annotations,omitempty"`
}

// Annotations 是 MCP 标准工具注解.
//
// 字段使用指针类型, 目的是区分"服务端明确说 false"和"服务端没说".
// 只读模式的判定依赖这个区别: 没说清楚的工具一律按写操作对待.
type Annotations struct {
	ReadOnlyHint    *bool `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool `json:"openWorldHint,omitempty"`
}

// ReadOnly 返回工具的只读属性, 第二个返回值为 false 表示服务端未声明.
func (t Tool) ReadOnly() (value bool, declared bool) {
	if t.Annotations.ReadOnlyHint == nil {
		return false, false
	}
	return *t.Annotations.ReadOnlyHint, true
}

// Destructive 返回工具的破坏性属性, 未声明时按 true 保守处理.
func (t Tool) Destructive() bool {
	if t.Annotations.DestructiveHint == nil {
		return true
	}
	return *t.Annotations.DestructiveHint
}

// RequiredArgs 返回工具声明的必填参数名.
func (t Tool) RequiredArgs() []string {
	raw, ok := t.InputSchema["required"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name, ok := item.(string); ok {
			names = append(names, name)
		}
	}
	return names
}

// ArgNames 返回工具声明的全部参数名.
func (t Tool) ArgNames() []string {
	properties, ok := t.InputSchema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	return names
}

// Result 是一次工具调用的结果.
type Result struct {
	// Data 是拆包后的业务数据.
	Data any `json:"data,omitempty"`
	// RawText 是服务端返回的原始文本, 仅在结果不是结构化 JSON 时有值.
	RawText string `json:"raw_text,omitempty"`
	// Unparsed 表示 Data 其实是没能解析成 JSON 的纯文本, agent 需要自己判断.
	Unparsed bool `json:"unparsed,omitempty"`
}

// unwrap 把 MCP 工具返回值拆成业务数据.
//
// 实测到的四种形状:
//   - 成功: {"content": [], "structuredContent": {"result": [...]}}
//   - 工具级错误: {"isError": true, "content": [{"type": "text", "text": "Error executing tool ..."}]}
//   - 业务级错误: {"content": [], "structuredContent": {"result": {"error": "endTime must not be in the future"}}}
//   - 纯文本: {"content": [{"type": "text", "text": "..."}]}
//
// 第三种最阴险: 服务端既没有标记 isError, 也不是纯文本, HTTP 状态也是 200,
// 只按前两种判断就会把失败当成功上报.
func unwrap(result map[string]any) (*Result, error) {
	if flagged, ok := result["isError"].(bool); ok && flagged {
		return nil, apperr.Upstream(extractErrorText(result)).
			WithHint("the upstream tool rejected this call, check the arguments with: dida tool show <tool>")
	}
	if structured, ok := result["structuredContent"]; ok && structured != nil {
		data := stripEnvelope(structured)
		if err := embeddedError(data); err != nil {
			return nil, err
		}
		return &Result{Data: data}, nil
	}
	if content, ok := result["content"].([]any); ok {
		for _, item := range content {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text, ok := block["text"].(string)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			var decoded any
			if err := json.Unmarshal([]byte(text), &decoded); err == nil {
				data := stripEnvelope(decoded)
				if err := embeddedError(data); err != nil {
					return nil, err
				}
				return &Result{Data: data}, nil
			}
			// 没能解析成 JSON 的纯文本: 先判断它是不是伪装成成功的错误.
			if looksLikeError(text) {
				return nil, apperr.Upstream(strings.TrimSpace(text)).
					WithHint("the upstream tool returned an error inside a successful response")
			}
			return &Result{Data: strings.TrimSpace(text), RawText: text, Unparsed: true}, nil
		}
	}
	return &Result{Data: stripEnvelope(result)}, nil
}

// embeddedError 判断拆包后的数据其实是一个业务错误对象.
//
// 只认"整个结果就是 {error: ...}"这一种确定形态, 避免把正常数据里恰好的
// error 字段误判成失败.
func embeddedError(data any) error {
	item, ok := data.(map[string]any)
	if !ok || len(item) != 1 {
		return nil
	}
	raw, ok := item["error"]
	if !ok {
		return nil
	}
	message := ""
	switch value := raw.(type) {
	case string:
		message = strings.TrimSpace(value)
	case map[string]any:
		if text, ok := value["message"].(string); ok {
			message = strings.TrimSpace(text)
		}
	}
	if message == "" {
		return nil
	}
	return apperr.Upstream(message).
		WithHint("the upstream tool reported this error inside a successful response")
}

// extractErrorText 从报错响应里取出最有信息量的一段文本.
func extractErrorText(result map[string]any) string {
	content, ok := result["content"].([]any)
	if !ok {
		return "upstream tool reported an error"
	}
	parts := make([]string, 0, len(content))
	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
			parts = append(parts, strings.TrimSpace(text))
		}
	}
	if len(parts) == 0 {
		return "upstream tool reported an error"
	}
	return strings.Join(parts, "\n")
}

// stripEnvelope 反复剥掉只有单个 result 或 results 键的外壳.
//
// 服务端习惯把真正的数据包一层 {"result": ...}, 这层壳对 agent 没有意义.
func stripEnvelope(value any) any {
	for {
		item, ok := value.(map[string]any)
		if !ok || len(item) != 1 {
			return value
		}
		next, ok := item["result"]
		if !ok {
			next, ok = item["results"]
		}
		if !ok {
			return value
		}
		value = next
	}
}

// looksLikeError 判断一段纯文本是否其实是错误信息.
//
// 只在服务端既没给 structuredContent 也没给 isError 时才会走到这里,
// 属于兜底判断, 因此只认高置信度的错误特征, 避免把正常文本误判成错误.
func looksLikeError(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "{") {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			_, hasError := decoded["error"]
			return hasError
		}
	}
	lowered := strings.ToLower(trimmed)
	for _, marker := range []string{
		"error executing tool",
		"traceback (most recent call last)",
		"validation error for",
		"pydantic",
	} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	for _, prefix := range []string{
		"error:", "error ", "exception:", "failed to", "invalid ", "unauthorized", "forbidden",
	} {
		if strings.HasPrefix(lowered, prefix) {
			return true
		}
	}
	return false
}
