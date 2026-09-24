package official

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockServer 是一个最小的 MCP 服务端, 用于在没有网络与真实口令的情况下
// 验证协议实现. 它按请求方法返回预设结果.
type mockServer struct {
	t *testing.T
	// withSession 为真时在握手响应里带上会话 id, 用于覆盖会话复用路径.
	withSession bool
	// callResult 是 tools/call 的返回值.
	callResult map[string]any
	// calls 记录收到的方法名, 便于断言握手只发生一次.
	calls []string
	// toolCount 是 tools/list 返回的工具数量.
	toolCount int
}

func (m *mockServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			m.t.Fatalf("读取请求体失败: %v", err)
		}
		var request struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			m.t.Fatalf("解析请求失败: %v", err)
		}
		m.calls = append(m.calls, request.Method)

		if m.withSession {
			w.Header().Set("Mcp-Session-Id", "session-abc")
		}
		switch request.Method {
		case "initialize":
			writeJSON(w, map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"serverInfo":      map[string]any{"name": "mock", "version": "0"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			tools := make([]map[string]any, 0, m.toolCount)
			for index := 0; index < m.toolCount; index++ {
				tools = append(tools, map[string]any{
					"name":        "tool_" + string(rune('a'+index)),
					"description": "mock tool",
					"inputSchema": map[string]any{"type": "object"},
				})
			}
			writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"tools": tools}})
		case "tools/call":
			writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": 3, "result": m.callResult})
		default:
			writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": 4, "result": map[string]any{}})
		}
	}
}

func writeJSON(w http.ResponseWriter, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func newMockClient(t *testing.T, server *mockServer) *Client {
	t.Helper()
	httpServer := httptest.NewServer(server.handler())
	t.Cleanup(httpServer.Close)
	return New(Options{
		Endpoint: httpServer.URL,
		Token:    "dp_mock_token",
		Timeout:  5 * time.Second,
	})
}

// TestHandshakeDoesNotDeadlock 是死锁回归测试.
//
// 早期实现里 post 会去抢握手流程已经持有的锁, 导致第一次请求必然自死锁,
// 因此这里对每一次握手路径都要真实跑通.
func TestHandshakeDoesNotDeadlock(t *testing.T) {
	for _, withSession := range []bool{false, true} {
		name := "无会话 id"
		if withSession {
			name = "带会话 id"
		}
		t.Run(name, func(t *testing.T) {
			server := &mockServer{t: t, withSession: withSession, toolCount: 2}
			client := newMockClient(t, server)
			done := make(chan error, 1)
			go func() {
				_, err := client.Tools(context.Background())
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("tools/list 失败: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("握手死锁: 10 秒内没有返回")
			}
			if count := countMethod(server.calls, "initialize"); count != 1 {
				t.Fatalf("initialize 调用次数 = %d, 期望 1", count)
			}
			if count := countMethod(server.calls, "tools/list"); count != 1 {
				t.Fatalf("tools/list 调用次数 = %d, 期望 1", count)
			}
		})
	}
}

// TestToolsCachedAcrossCalls 验证工具目录在进程内只拉取一次.
func TestToolsCachedAcrossCalls(t *testing.T) {
	server := &mockServer{t: t, toolCount: 3}
	client := newMockClient(t, server)
	for index := 0; index < 3; index++ {
		if _, err := client.Tools(context.Background()); err != nil {
			t.Fatalf("第 %d 次 tools/list 失败: %v", index, err)
		}
	}
	if count := countMethod(server.calls, "tools/list"); count != 1 {
		t.Fatalf("tools/list 调用次数 = %d, 期望 1", count)
	}
}

// TestCallUnwrapsStructuredContent 验证结构化结果的拆包.
func TestCallUnwrapsStructuredContent(t *testing.T) {
	server := &mockServer{
		t: t,
		callResult: map[string]any{
			"content":           []any{},
			"structuredContent": map[string]any{"result": []any{map[string]any{"id": "a"}}},
		},
	}
	client := newMockClient(t, server)
	result, err := client.Call(context.Background(), "list_projects", nil)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	items, ok := result.Data.([]any)
	if !ok {
		t.Fatalf("拆包结果类型 = %T, 期望数组", result.Data)
	}
	if len(items) != 1 {
		t.Fatalf("拆包结果长度 = %d, 期望 1", len(items))
	}
}

// TestCallPromotesIsError 验证服务端标记的错误不会被当成成功.
//
// 这是本项目最看重的一条: 只看 HTTP 状态或只看有没有 result 字段,
// 都会让失败静默通过.
func TestCallPromotesIsError(t *testing.T) {
	server := &mockServer{
		t: t,
		callResult: map[string]any{
			"isError": true,
			"content": []any{map[string]any{
				"type": "text",
				"text": "Error executing tool list_completed_tasks_by_date: 1 validation error",
			}},
		},
	}
	client := newMockClient(t, server)
	result, err := client.Call(context.Background(), "list_completed_tasks_by_date", nil)
	if err == nil {
		t.Fatal("服务端已标记 isError, 却返回了成功")
	}
	if result != nil {
		t.Fatalf("失败时不应返回结果, 实际得到 %#v", result)
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Fatalf("错误信息丢失了服务端原文: %v", err)
	}
}

// TestCallDetectsTextErrorWithoutFlag 验证没有 isError 标记时的兜底判断.
func TestCallDetectsTextErrorWithoutFlag(t *testing.T) {
	server := &mockServer{
		t: t,
		callResult: map[string]any{
			"content": []any{map[string]any{
				"type": "text",
				"text": "Error executing tool create_task: task title is empty",
			}},
		},
	}
	client := newMockClient(t, server)
	if _, err := client.Call(context.Background(), "create_task", nil); err == nil {
		t.Fatal("明显的错误文本没有被识别为失败")
	}
}

// TestCallKeepsPlainText 验证正常的纯文本结果不被误判成错误.
func TestCallKeepsPlainText(t *testing.T) {
	server := &mockServer{
		t: t,
		callResult: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "任务已创建"}},
		},
	}
	client := newMockClient(t, server)
	result, err := client.Call(context.Background(), "create_task", nil)
	if err != nil {
		t.Fatalf("正常文本被误判为错误: %v", err)
	}
	if !result.Unparsed {
		t.Fatal("纯文本结果应当标记为未解析")
	}
	if result.Data != "任务已创建" {
		t.Fatalf("文本内容 = %#v", result.Data)
	}
}

// TestCallSurfacesHTTPAuth 验证 401 被归为认证错误.
func TestCallSurfacesHTTPAuth(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer httpServer.Close()

	client := New(Options{Endpoint: httpServer.URL, Token: "dp_bad", Timeout: 5 * time.Second})
	_, err := client.Tools(context.Background())
	if err == nil {
		t.Fatal("401 没有被识别为错误")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("错误信息应当包含状态码: %v", err)
	}
}

// TestCallPromotesEmbeddedError 是业务级错误的回归测试.
//
// 实测发现官方会把业务失败包成 {"error": "..."} 放进 structuredContent,
// 既不设 isError 也不改 HTTP 状态. 这条路径漏了就会把失败当成功上报,
// 是整个项目最容易静默出错的地方.
func TestCallPromotesEmbeddedError(t *testing.T) {
	server := &mockServer{
		t: t,
		callResult: map[string]any{
			"content": []any{},
			"structuredContent": map[string]any{
				"result": map[string]any{"error": "endTime must not be in the future"},
			},
		},
	}
	client := newMockClient(t, server)
	result, err := client.Call(context.Background(), "create_focus", nil)
	if err == nil {
		t.Fatalf("业务错误被当成了成功: %#v", result)
	}
	if !strings.Contains(err.Error(), "endTime must not be in the future") {
		t.Fatalf("错误信息丢失了服务端原文: %v", err)
	}
}

// TestCallPromotesEmbeddedErrorInText 覆盖文本编码下的同一种业务错误.
func TestCallPromotesEmbeddedErrorInText(t *testing.T) {
	server := &mockServer{
		t: t,
		callResult: map[string]any{
			"content": []any{map[string]any{
				"type": "text",
				"text": `{"error":"task title is empty"}`,
			}},
		},
	}
	client := newMockClient(t, server)
	if _, err := client.Call(context.Background(), "create_task", nil); err == nil {
		t.Fatal("文本编码的业务错误没有被识别")
	}
}

// TestCallKeepsDataWithErrorField 确认正常数据里恰好带 error 字段时不会被误判.
func TestCallKeepsDataWithErrorField(t *testing.T) {
	server := &mockServer{
		t: t,
		callResult: map[string]any{
			"content": []any{},
			"structuredContent": map[string]any{
				"result": map[string]any{"id": "a", "error": "这是个正常字段"},
			},
		},
	}
	client := newMockClient(t, server)
	result, err := client.Call(context.Background(), "list_projects", nil)
	if err != nil {
		t.Fatalf("正常数据被误判成错误: %v", err)
	}
	record, ok := result.Data.(map[string]any)
	if !ok || record["id"] != "a" {
		t.Fatalf("数据被破坏: %#v", result.Data)
	}
}

func countMethod(calls []string, method string) int {
	count := 0
	for _, call := range calls {
		if call == method {
			count++
		}
	}
	return count
}
