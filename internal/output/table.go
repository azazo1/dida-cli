package output

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// renderTable 把信封渲染成人看的文本, 仅供人工阅读, agent 不应依赖此格式.
func (r *Renderer) renderTable(envelope Envelope) error {
	if !envelope.OK {
		if _, err := fmt.Fprintf(r.Writer, "失败 [%s] %s\n", envelope.Error.Type, envelope.Error.Message); err != nil {
			return err
		}
		if envelope.Error.Hint != "" {
			_, err := fmt.Fprintf(r.Writer, "建议: %s\n", envelope.Error.Hint)
			return err
		}
		return nil
	}
	rows, headers := tabulate(envelope.Data)
	if len(rows) == 0 {
		_, err := fmt.Fprintf(r.Writer, "%s: 无数据\n", envelope.Command)
		return err
	}
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = displayWidth(header)
	}
	for _, row := range rows {
		for index := range headers {
			if width := displayWidth(row[index]); width > widths[index] {
				widths[index] = width
			}
		}
	}
	var builder strings.Builder
	for index, header := range headers {
		builder.WriteString(pad(header, widths[index]))
		if index < len(headers)-1 {
			builder.WriteString("  ")
		}
	}
	builder.WriteString("\n")
	for _, row := range rows {
		for index := range headers {
			builder.WriteString(pad(row[index], widths[index]))
			if index < len(headers)-1 {
				builder.WriteString("  ")
			}
		}
		builder.WriteString("\n")
	}
	if envelope.Meta.Count > 0 {
		builder.WriteString(fmt.Sprintf("共 %d 条\n", envelope.Meta.Count))
	}
	_, err := fmt.Fprint(r.Writer, builder.String())
	return err
}

// tabulate 把任意结果压平成表格行, 只做浅层展开, 嵌套结构序列化为单行 JSON.
func tabulate(data any) ([][]string, []string) {
	switch value := data.(type) {
	case []any:
		return tabulateSlice(value)
	case []map[string]any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, item)
		}
		return tabulateSlice(items)
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		rows := make([][]string, 0, len(keys))
		for _, key := range keys {
			rows = append(rows, []string{key, scalarText(value[key])})
		}
		return rows, []string{"字段", "取值"}
	case nil:
		return nil, nil
	default:
		return [][]string{{scalarText(value)}}, []string{"结果"}
	}
}

func tabulateSlice(items []any) ([][]string, []string) {
	if len(items) == 0 {
		return nil, nil
	}
	headers := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		keys := make([]string, 0, len(record))
		for key := range record {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if !seen[key] {
				seen[key] = true
				headers = append(headers, key)
			}
		}
	}
	if len(headers) == 0 {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{scalarText(item)})
		}
		return rows, []string{"结果"}
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		record, ok := item.(map[string]any)
		row := make([]string, len(headers))
		if ok {
			for index, header := range headers {
				row[index] = scalarText(record[header])
			}
		} else {
			row[0] = scalarText(item)
		}
		rows = append(rows, row)
	}
	return rows, headers
}

func scalarText(value any) string {
	switch item := value.(type) {
	case nil:
		return ""
	case string:
		return item
	case bool:
		if item {
			return "true"
		}
		return "false"
	case float64:
		if item == float64(int64(item)) {
			return fmt.Sprintf("%d", int64(item))
		}
		return fmt.Sprintf("%g", item)
	case int:
		return fmt.Sprintf("%d", item)
	case int64:
		return fmt.Sprintf("%d", item)
	default:
		encoded, err := marshalCompact(item)
		if err != nil {
			return fmt.Sprintf("%v", item)
		}
		return encoded
	}
}

// marshalCompact 把嵌套结构压成单行 JSON, 供表格单元格展示.
func marshalCompact(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func displayWidth(text string) int {
	width := 0
	for _, char := range text {
		if char > 0x2E80 {
			width += 2
		} else {
			width++
		}
	}
	return width
}

func pad(text string, width int) string {
	padding := width - displayWidth(text)
	if padding <= 0 {
		return text
	}
	return text + strings.Repeat(" ", padding)
}
