package official

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/azazo1/dida-cli/internal/apperr"
	"github.com/azazo1/dida-cli/internal/logging"
)

// Options 是构造客户端的参数.
type Options struct {
	Endpoint string
	Token    string
	Timeout  time.Duration
	Logger   *slog.Logger
	// ClientVersion 会随 clientInfo 一起上报, 便于服务端侧排查.
	ClientVersion string
	// Cache 描述工具目录的本地缓存策略.
	Cache CacheOptions
}

// Client 是官方 MCP 通道的客户端.
//
// 客户端是惰性握手的: 只有真正需要发请求时才做 initialize,
// 这样 --dry-run 和参数校验失败都不会产生任何网络流量.
type Client struct {
	endpoint      string
	token         string
	http          *http.Client
	logger        *slog.Logger
	clientVersion string
	cache         CacheOptions

	mu          sync.Mutex
	sessionMu   sync.RWMutex
	sessionID   string
	initialized bool
	nextID      int64
	tools       []Tool
}

// New 构造一个客户端.
func New(opts Options) *Client {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	logger := opts.Logger
	if logger == nil {
		logger = logging.Discard()
	}
	version := opts.ClientVersion
	if version == "" {
		version = "dev"
	}
	return &Client{
		endpoint:      endpoint,
		token:         strings.TrimSpace(opts.Token),
		http:          &http.Client{Timeout: timeout},
		logger:        logger,
		clientVersion: version,
		cache:         opts.Cache,
	}
}

// Endpoint 返回实际使用的端点地址.
func (c *Client) Endpoint() string { return c.endpoint }

// TokenPreview 返回脱敏后的口令预览.
func (c *Client) TokenPreview() string { return logging.Redact(c.token) }

// Initialize 执行 MCP 握手, 重复调用只会生效一次.
func (c *Client) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initializeLocked(ctx)
}

func (c *Client) initializeLocked(ctx context.Context) error {
	if c.initialized {
		return nil
	}
	started := time.Now()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.takeIDLocked(),
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": initializeProtocolVersion,
			"clientInfo": map[string]any{
				"name":    "dida-cli",
				"version": c.clientVersion,
			},
			"capabilities": map[string]any{},
		},
	}
	response, headers, err := c.post(ctx, payload, false)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return apperr.Upstream("mcp initialize rejected: " + response.Error.Message)
	}
	if session := headerValue(headers, "Mcp-Session-Id"); session != "" {
		c.sessionMu.Lock()
		c.sessionID = session
		c.sessionMu.Unlock()
	}
	c.initialized = true
	c.logger.Debug("mcp 握手完成",
		"endpoint", c.endpoint,
		"session", c.sessionID != "",
		"duration_ms", time.Since(started).Milliseconds(),
	)

	// 握手后的 initialized 通知是协议要求, 失败不致命, 但要在日志里留痕.
	notify := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}
	if _, _, err := c.post(ctx, notify, true); err != nil {
		c.logger.Warn("initialized 通知发送失败", "error", err)
	}
	return nil
}

// Tools 返回官方工具目录.
//
// 取数顺序是进程内缓存, 本地文件缓存, 最后才是网络. 工具目录变化很慢,
// 这样可以让高频的 dida tool call 不必每次都多打一个来回.
func (c *Client) Tools(ctx context.Context) ([]Tool, error) {
	c.mu.Lock()
	if c.tools != nil {
		cached := c.tools
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	if cached, ok := loadCachedTools(c.cache, c.endpoint); ok {
		c.logger.Debug("工具目录命中本地缓存", "count", len(cached))
		c.mu.Lock()
		c.tools = cached
		c.mu.Unlock()
		return cached, nil
	}

	if err := c.Initialize(ctx); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.takeID(),
		"method":  "tools/list",
		"params":  map[string]any{},
	}
	response, _, err := c.post(ctx, payload, true)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, apperr.Upstream("mcp tools/list rejected: " + response.Error.Message)
	}
	rawTools, _ := response.Result["tools"].([]any)
	tools := make([]Tool, 0, len(rawTools))
	for _, raw := range rawTools {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tools = append(tools, Tool{
			Name:        stringField(item, "name"),
			Description: stringField(item, "description"),
			InputSchema: mapField(item, "inputSchema"),
			Annotations: annotationsField(item, "annotations"),
		})
	}
	c.mu.Lock()
	c.tools = tools
	c.mu.Unlock()
	if err := saveCachedTools(c.cache, c.endpoint, tools); err != nil {
		// 缓存写不进去只是少了一层加速, 不影响功能.
		c.logger.Debug("工具目录缓存写入失败", "error", err)
	}
	c.logger.Debug("工具目录已拉取", "count", len(tools))
	return tools, nil
}

// ToolByName 按名字查找工具.
func (c *Client) ToolByName(ctx context.Context, name string) (*Tool, error) {
	tools, err := c.Tools(ctx)
	if err != nil {
		return nil, err
	}
	for _, tool := range tools {
		if tool.Name == name {
			found := tool
			return &found, nil
		}
	}
	return nil, apperr.NotFound(fmt.Sprintf("no official tool named %q", name)).
		WithHint("run: dida tool list")
}

// Call 调用一个官方工具.
func (c *Client) Call(ctx context.Context, name string, args map[string]any) (*Result, error) {
	if args == nil {
		args = map[string]any{}
	}
	if err := c.Initialize(ctx); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.takeID(),
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	}
	started := time.Now()
	c.logger.Debug("调用官方工具", "tool", name, "args", logging.Keys(args))
	response, _, err := c.post(ctx, payload, true)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, apperr.Upstream("mcp tools/call rejected: " + response.Error.Message)
	}
	result, err := unwrap(response.Result)
	if err != nil {
		c.logger.Debug("官方工具返回错误", "tool", name, "duration_ms", time.Since(started).Milliseconds())
		return nil, err
	}
	c.logger.Debug("官方工具调用完成",
		"tool", name,
		"duration_ms", time.Since(started).Milliseconds(),
		"unparsed", result.Unparsed,
	)
	return result, nil
}

// post 发送一个 JSON-RPC 请求.
func (c *Client) post(ctx context.Context, payload map[string]any, withProtocolHeader bool) (rpcResponse, http.Header, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return rpcResponse{}, nil, apperr.Wrap(apperr.KindGeneral, err, "encode request failed")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return rpcResponse{}, nil, apperr.Wrap(apperr.KindGeneral, err, "build request failed")
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("User-Agent", "dida-cli/"+c.clientVersion)
	if withProtocolHeader {
		request.Header.Set("MCP-Protocol-Version", requestProtocolVersion)
	}
	// 会话 id 用独立的锁保护: post 会在持有 mu 的握手流程里被调用,
	// 如果这里再去抢 mu 就会自死锁.
	c.sessionMu.RLock()
	session := c.sessionID
	c.sessionMu.RUnlock()
	if session != "" {
		request.Header.Set("Mcp-Session-Id", session)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return rpcResponse{}, nil, apperr.Wrap(apperr.KindUpstream, err, "mcp request failed").
			WithHintf("check network access to %s", c.endpoint)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return rpcResponse{}, response.Header, apperr.Auth(fmt.Sprintf("mcp rejected the token with HTTP %d", response.StatusCode)).
			WithHint("the token may be expired or revoked, run: printf 'dp_...' | dida auth login --token-stdin")
	}
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return rpcResponse{}, response.Header, apperr.Upstream(fmt.Sprintf("mcp returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body))))
	}
	// 通知类请求没有 id, 服务端返回 202 且没有正文, 属于正常情况.
	if payload["id"] == nil && response.StatusCode == http.StatusAccepted {
		return rpcResponse{}, response.Header, nil
	}
	parsed, err := parseResponse(response)
	if err != nil {
		return rpcResponse{}, response.Header, err
	}
	return parsed, response.Header, nil
}

func (c *Client) takeID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.takeIDLocked()
}

func (c *Client) takeIDLocked() int64 {
	c.nextID++
	return c.nextID
}

type rpcResponse struct {
	Result map[string]any `json:"result"`
	Error  *rpcError      `json:"error"`
}

type rpcError struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

// parseResponse 兼容纯 JSON 与 SSE 两种响应编码.
func parseResponse(response *http.Response) (rpcResponse, error) {
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		return parseSSE(response.Body)
	}
	var parsed rpcResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return rpcResponse{}, apperr.Wrap(apperr.KindUpstream, err, "decode mcp response failed")
	}
	return parsed, nil
}

// parseSSE 从事件流里取出第一条完整的 JSON-RPC 消息.
func parseSSE(body io.Reader) (rpcResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	chunks := make([]string, 0, 8)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(chunks) == 0 {
				continue
			}
			var parsed rpcResponse
			if err := json.Unmarshal([]byte(strings.Join(chunks, "\n")), &parsed); err == nil {
				return parsed, nil
			}
			chunks = chunks[:0]
			continue
		}
		if strings.HasPrefix(line, "data:") {
			chunks = append(chunks, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return rpcResponse{}, apperr.Wrap(apperr.KindUpstream, err, "read mcp event stream failed")
	}
	return rpcResponse{}, apperr.Upstream("mcp returned an empty event stream")
}

func headerValue(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	if value := headers.Get(name); value != "" {
		return value
	}
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func stringField(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func mapField(item map[string]any, key string) map[string]any {
	value, _ := item[key].(map[string]any)
	return value
}

func annotationsField(item map[string]any, key string) Annotations {
	raw, ok := item[key].(map[string]any)
	if !ok {
		return Annotations{}
	}
	var annotations Annotations
	if value, ok := raw["readOnlyHint"].(bool); ok {
		annotations.ReadOnlyHint = &value
	}
	if value, ok := raw["destructiveHint"].(bool); ok {
		annotations.DestructiveHint = &value
	}
	if value, ok := raw["idempotentHint"].(bool); ok {
		annotations.IdempotentHint = &value
	}
	if value, ok := raw["openWorldHint"].(bool); ok {
		annotations.OpenWorldHint = &value
	}
	return annotations
}
