package output

// Compact 按对象类型裁剪字段, 用于 --compact 模式节省 token.
//
// 裁剪规则只看对象的键签名, 不依赖任何类型信息, 因此对官方返回的
// 任意结构都安全: 认不出来的对象原样返回, 不做猜测性删除.
func Compact(data any) any {
	switch value := data.(type) {
	case []any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, Compact(item))
		}
		return items
	case map[string]any:
		return compactObject(value)
	default:
		return data
	}
}

func compactObject(value map[string]any) map[string]any {
	if fields, ok := classify(value); ok {
		trimmed := make(map[string]any, len(fields))
		for _, field := range fields {
			if item, exists := value[field]; exists {
				trimmed[field] = item
			}
		}
		return trimmed
	}
	// 容器对象: 递归裁剪内部的列表, 保留其余字段.
	trimmed := make(map[string]any, len(value))
	for key, item := range value {
		trimmed[key] = Compact(item)
	}
	return trimmed
}

// classify 通过键签名判断对象类型.
func classify(value map[string]any) ([]string, bool) {
	_, hasTitle := value["title"]
	_, hasProjectID := value["projectId"]
	_, hasStatus := value["status"]
	_, hasDue := value["dueDate"]
	if hasTitle && (hasProjectID || hasStatus || hasDue) {
		return taskFields, true
	}
	_, hasName := value["name"]
	_, hasKind := value["kind"]
	if hasName && hasKind {
		return projectFields, true
	}
	if hasName && !hasKind {
		return nameOnlyFields, true
	}
	return nil, false
}

var taskFields = []string{
	"id", "projectId", "title", "content", "desc", "status", "priority",
	"dueDate", "startDate", "isAllDay", "tags", "parentId", "columnId",
	"kind", "assigneeUsername", "etag",
}

var projectFields = []string{
	"id", "name", "kind", "groupId", "viewMode", "closed", "sortOrder",
}

var nameOnlyFields = []string{
	"id", "name", "type", "color", "sortOrder",
}
