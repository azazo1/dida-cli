package official

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CacheOptions 描述工具目录的本地缓存.
//
// 工具目录是低频变化数据, 缓存它可以省掉每次 dida tool call 之前的一次
// 额外往返. 缓存只保存工具契约, 不含任何账号数据.
type CacheOptions struct {
	// Enabled 表示是否启用缓存.
	Enabled bool
	// Path 是缓存文件路径.
	Path string
	// TTL 是缓存存活时长, 非正数表示永不过期.
	TTL time.Duration
}

type cachedTools struct {
	SavedAt  time.Time `json:"saved_at"`
	Endpoint string    `json:"endpoint"`
	Tools    []Tool    `json:"tools"`
}

// loadCachedTools 读取缓存, 命中时返回工具目录.
func loadCachedTools(opts CacheOptions, endpoint string) ([]Tool, bool) {
	if !opts.Enabled || opts.Path == "" {
		return nil, false
	}
	raw, err := os.ReadFile(opts.Path)
	if err != nil {
		return nil, false
	}
	var cached cachedTools
	if err := json.Unmarshal(raw, &cached); err != nil {
		return nil, false
	}
	// 端点换了就作废, 否则会把另一个环境的工具目录当成当前的用.
	if cached.Endpoint != endpoint || len(cached.Tools) == 0 {
		return nil, false
	}
	if opts.TTL > 0 && time.Since(cached.SavedAt) > opts.TTL {
		return nil, false
	}
	return cached.Tools, true
}

// saveCachedTools 写入缓存, 失败只影响性能不影响功能.
func saveCachedTools(opts CacheOptions, endpoint string, tools []Tool) error {
	if !opts.Enabled || opts.Path == "" || len(tools) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(opts.Path), 0o700); err != nil {
		return err
	}
	encoded, err := json.Marshal(cachedTools{
		SavedAt:  time.Now(),
		Endpoint: endpoint,
		Tools:    tools,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(opts.Path, encoded, 0o600)
}
