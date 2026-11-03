// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHeaderTransport(t *testing.T) {
	headers := map[string]string{
		"Authorization": "Bearer test-token",
		"X-Custom":      "custom-value",
	}
	transport := &headerTransport{
		base:       http.DefaultTransport,
		headers:    headers,
		serverName: "test-server",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
		}
		if got := r.Header.Get("X-Custom"); got != "custom-value" {
			t.Errorf("X-Custom = %q, want %q", got, "custom-value")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &http.Client{Transport: transport}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHeaderTransport_401(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	transport := &headerTransport{
		base:       http.DefaultTransport,
		headers:    map[string]string{"Authorization": "Bearer bad-token"},
		serverName: "auth-server",
	}
	client := &http.Client{Transport: transport}
	_, err := client.Get(ts.URL)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "401 Unauthorized") || !strings.Contains(got, "auth-server") {
		t.Errorf("error = %q, want mention of 401 and server name", got)
	}
}

func TestHeaderTransport_403(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	transport := &headerTransport{
		base:       http.DefaultTransport,
		headers:    map[string]string{"Authorization": "Bearer limited-token"},
		serverName: "perm-server",
	}
	client := &http.Client{Transport: transport}
	_, err := client.Get(ts.URL)
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "403 Forbidden") || !strings.Contains(got, "perm-server") {
		t.Errorf("error = %q, want mention of 403 and server name", got)
	}
}

func TestNewRemoteClient_HeaderExpandsToEmpty(t *testing.T) {
	t.Setenv("OCR_TEST_EMPTY_VAR", "")

	_, err := NewRemoteClient(
		context.Background(),
		"test-srv",
		"http://localhost:9999/mcp",
		map[string]string{"Authorization": "$OCR_TEST_EMPTY_VAR"},
		"v0.0.1-test",
	)
	if err == nil {
		t.Fatal("expected error when header expands to empty, got nil")
	}
	if !strings.Contains(err.Error(), "expanded to empty") {
		t.Errorf("error = %q, want mention of 'expanded to empty'", err.Error())
	}
	if !strings.Contains(err.Error(), "Authorization") {
		t.Errorf("error = %q, want mention of header name 'Authorization'", err.Error())
	}
}

// TestNewRemoteClient_ConnectFailure exercises the success path of header
// expansion (non-empty value) plus HTTP client / transport construction, then
// the connect-failure return when the endpoint refuses the connection.
func TestNewRemoteClient_ConnectFailure(t *testing.T) {
	t.Setenv("OCR_TEST_TOKEN", "secret-value")

	_, err := NewRemoteClient(
		context.Background(),
		"test-srv",
		// Port 1 is reserved and refuses connections immediately.
		"http://127.0.0.1:1/mcp",
		map[string]string{"Authorization": "Bearer $OCR_TEST_TOKEN"},
		"v0.0.1-test",
	)
	if err == nil {
		t.Fatal("expected error when the endpoint refuses the connection, got nil")
	}
	if !strings.Contains(err.Error(), "connect to remote MCP server") {
		t.Errorf("error = %q, want mention of 'connect to remote MCP server'", err.Error())
	}
}

func TestNewRemoteClient_HeaderExpandsUnsetVar(t *testing.T) {
	t.Setenv("OCR_TEST_UNSET_MARKER", "")
	// Ensure the variable is truly unset (Setenv("", "") sets it to empty;
	// os.Expand with os.Getenv returns "" for both unset and empty).
	// The point: $OCR_TEST_NONEXISTENT_VAR_XYZ is never set.

	_, err := NewRemoteClient(
		context.Background(),
		"test-srv",
		"http://localhost:9999/mcp",
		map[string]string{"X-Token": "$OCR_TEST_NONEXISTENT_VAR_XYZ"},
		"v0.0.1-test",
	)
	if err == nil {
		t.Fatal("expected error when header references unset env var, got nil")
	}
	if !strings.Contains(err.Error(), "expanded to empty") {
		t.Errorf("error = %q, want mention of 'expanded to empty'", err.Error())
	}
	if !strings.Contains(err.Error(), "X-Token") {
		t.Errorf("error = %q, want mention of header name 'X-Token'", err.Error())
	}
}

func TestContentToText_SingleText(t *testing.T) {
	contents := []mcp.Content{
		&mcp.TextContent{Text: "hello"},
	}
	got := contentToText(contents)
	if got != "hello" {
		t.Errorf("contentToText() = %q, want %q", got, "hello")
	}
}

func TestContentToText_MultipleText(t *testing.T) {
	contents := []mcp.Content{
		&mcp.TextContent{Text: "line1"},
		&mcp.TextContent{Text: "line2"},
	}
	got := contentToText(contents)
	want := "line1\nline2"
	if got != want {
		t.Errorf("contentToText() = %q, want %q", got, want)
	}
}

func TestContentToText_Empty(t *testing.T) {
	got := contentToText(nil)
	if got != "" {
		t.Errorf("contentToText(nil) = %q, want empty", got)
	}

	got = contentToText([]mcp.Content{})
	if got != "" {
		t.Errorf("contentToText([]) = %q, want empty", got)
	}
}

func TestContentToText_UnsupportedType(t *testing.T) {
	contents := []mcp.Content{
		&mcp.ImageContent{MIMEType: "image/png", Data: []byte("data")},
	}
	got := contentToText(contents)
	if got == "" {
		t.Error("expected non-empty for unsupported type")
	}
}

func TestClient_NameAndTools(t *testing.T) {
	tools := []*mcp.Tool{{Name: "t1"}, {Name: "t2"}}
	c := &Client{name: "test-srv", tools: tools}

	if c.Name() != "test-srv" {
		t.Errorf("Name() = %q, want %q", c.Name(), "test-srv")
	}
	if len(c.Tools()) != 2 {
		t.Errorf("Tools() len = %d, want 2", len(c.Tools()))
	}
}

func TestProvider_Execute_Integration(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "v0.0.1"},
		nil,
	)
	server.AddTool(
		&mcp.Tool{
			Name:        "greet",
			Description: "Greets",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "hello " + args.Name}},
			}, nil
		},
	)
	server.AddTool(
		&mcp.Tool{Name: "fail", InputSchema: map[string]any{"type": "object"}},
		func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "controlled failure"}}}, nil
		},
	)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	sdkClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v0.0.1"},
		nil,
	)
	session, err := sdkClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	c := &Client{name: "test-srv", session: session, tools: toolsResult.Tools}
	defer func() { _ = c.Close() }()

	p := &Provider{toolName: "greet", client: c}
	result, err := p.Execute(ctx, map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello world" {
		t.Errorf("Execute result = %q, want %q", result, "hello world")
	}
	failing := &Provider{toolName: "fail", client: c}
	result, err = failing.Execute(ctx, nil)
	if err != nil || !strings.Contains(result, "controlled failure") {
		t.Fatalf("service error lost: %q %v", result, err)
	}

}

func TestMCPArgumentFeedbackPreservesServiceBoundary(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "argument-test", Version: "1"}, nil)
	for _, name := range []string{"closed", "open", "pattern"} {
		schema := map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}
		if name != "open" {
			schema["additionalProperties"] = false
		}
		if name == "pattern" {
			schema["patternProperties"] = map[string]any{"^x_": map[string]any{"type": "number"}}
		}
		server.AddTool(&mcp.Tool{Name: name, InputSchema: schema}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			var args map[string]any
			_ = json.Unmarshal(req.Params.Arguments, &args)
			if args["path"] == "outside" {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Access denied - outside allowed directories"}}}, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(req.Params.Arguments)}}}, nil
		})
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	sdkClient := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := sdkClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{name: "knowledge", session: cs, tools: listed.Tools}
	provider := func(name string) *Provider { return &Provider{toolName: name, client: c} }

	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"closed", map[string]any{}, "path"},
		{"closed", map[string]any{"path": 42}, "path"},
		{"closed", map[string]any{"path": "private-token-value", "start_line": 3}, "start_line"},
		{"unknown", map[string]any{}, "not available"},
	} {
		result, err := provider(tc.name).Execute(ctx, tc.args)
		if err != nil || !strings.Contains(result, tc.want) || strings.Contains(result, "private-token-value") {
			t.Fatalf("result=%q err=%v", result, err)
		}
		if calls.Load() != 0 {
			t.Fatal("local invalid call reached service")
		}
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"closed", map[string]any{"path": "knowledge.md"}, "knowledge.md"},
		{"open", map[string]any{"path": "knowledge.md", "start_line": 3}, "start_line"},
		{"pattern", map[string]any{"path": "knowledge.md", "x_offset": 3}, "x_offset"},
		{"closed", map[string]any{"path": "outside"}, "Access denied"},
	} {
		result, err := provider(tc.name).Execute(ctx, tc.args)
		if err != nil || !strings.Contains(result, tc.want) {
			t.Fatalf("result=%q err=%v", result, err)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestMCPArgumentDeclaredConstraints(t *testing.T) {
	client := newTestClient("test", []*mcp.Tool{{Name: "read", InputSchema: map[string]any{
		"type": "object", "properties": map[string]any{
			"mode":  map[string]any{"type": "string", "enum": []string{"head", "tail"}},
			"count": map[string]any{"type": "integer", "minimum": 1},
		},
	}}})
	for _, args := range []map[string]any{{"mode": "middle"}, {"count": float64(0)}, {"count": 1.5}} {
		if feedback := client.validateArguments("read", args); !strings.Contains(feedback, "no service call") {
			t.Fatal(feedback)
		}
	}
	if feedback := client.validateArguments("read", map[string]any{"mode": "head", "count": float64(1), "extension": true}); feedback != "" {
		t.Fatal(feedback)
	}
}

func TestNewClient_Integration(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "v0.0.1"},
		nil,
	)
	server.AddTool(
		&mcp.Tool{
			Name:        "echo",
			Description: "Echoes input",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			},
		},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + args.Message}},
			}, nil
		},
	)

	server.AddTool(
		&mcp.Tool{
			Name:        "fail",
			Description: "Always fails",
			InputSchema: map[string]any{"type": "object"},
		},
		func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "something went wrong"}},
			}, nil
		},
	)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	sdkClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v0.0.1"},
		nil,
	)
	session, err := sdkClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	c := &Client{
		name:    "test-srv",
		session: session,
		tools:   toolsResult.Tools,
	}
	defer func() { _ = c.Close() }()

	if c.Name() != "test-srv" {
		t.Errorf("Name() = %q", c.Name())
	}
	if len(c.Tools()) != 2 {
		t.Errorf("Tools() len = %d, want 2", len(c.Tools()))
	}

	t.Run("CallTool_success", func(t *testing.T) {
		result, err := c.CallTool(ctx, "echo", map[string]any{"message": "hi"})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result != "echo: hi" {
			t.Errorf("CallTool result = %q, want %q", result, "echo: hi")
		}
	})

	t.Run("CallTool_error_result", func(t *testing.T) {
		result, err := c.CallTool(ctx, "fail", nil)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result == "" {
			t.Error("expected non-empty error message")
		}
		if !strings.Contains(result, "something went wrong") {
			t.Fatal(result)
		}

	})

	t.Run("Close", func(t *testing.T) {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}
