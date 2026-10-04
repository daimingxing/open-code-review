// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Provider adapts a single MCP tool to the tool.Provider interface.
type Provider struct {
	toolName string
	client   *Client
	observe  ObservationHandler
}

// ObservationHandler 在调用方启用观测时接收 MCP 工具结果和 allowlist 可用性信号。 // allow-non-english: Chinese contract comment required by repository instructions
type ObservationHandler func(serverName, name string, args map[string]any, result string, err error, serviceError bool)

func (p *Provider) Tool() tool.Tool {
	return tool.Dynamic(p.toolName)
}

func (p *Provider) Execute(ctx context.Context, args map[string]any) (string, error) {
	result, actualResult, err, serviceError := p.client.CallToolForObservation(ctx, p.toolName, args)
	if p.observe != nil {
		p.observe(p.client.Name(), p.toolName, args, actualResult, err, serviceError)
	}
	return result, err
}

// RegisterAll 将 MCP 客户端工具注册到工具表。 // allow-non-english: Chinese API contract comment required by repository instructions
// allowedTools 非空时只注册列出的工具；未注册项会告警并通知可选观测器。 // allow-non-english: Chinese API contract comment required by repository instructions
// 可选观测器接收实际 MCP 返回值，不改变工具返回给模型的文本。 // allow-non-english: Chinese API contract comment required by repository instructions
func RegisterAll(reg *tool.Registry, c *Client, allowedTools []string, observers ...ObservationHandler) {
	var observe ObservationHandler
	if len(observers) > 0 {
		observe = observers[0]
	}
	allowed := make(map[string]struct{}, len(allowedTools))
	for _, name := range allowedTools {
		allowed[name] = struct{}{}
	}
	filtering := len(allowed) > 0

	registered := make(map[string]struct{})
	for _, t := range c.Tools() {
		if filtering {
			if _, ok := allowed[t.Name]; !ok {
				continue
			}
		}
		if tool.IsReserved(t.Name) {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP server %q tool %q conflicts with built-in tool, skipping\n", c.Name(), t.Name)
			continue
		}
		if _, exists := reg.Get(t.Name); exists {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP server %q tool %q conflicts with already-registered tool, skipping\n", c.Name(), t.Name)
			continue
		}
		reg.Register(&Provider{
			toolName: t.Name,
			client:   c,
			observe:  observe,
		})
		registered[t.Name] = struct{}{}
	}

	for name := range allowed {
		if _, ok := registered[name]; !ok {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP server %q allowed tool %q was not registered\n", c.Name(), name)
			if observe != nil {
				observe(c.Name(), "server_unavailable", map[string]any{"stage": "tool_unavailable", "tool": name}, "", nil, false)
			}
		}
	}
}

// ToToolDef converts an MCP tool definition to an llm.ToolDef.
func ToToolDef(t *mcp.Tool) llm.ToolDef {
	params := map[string]any{"type": "object"}

	switch schema := t.InputSchema.(type) {
	case map[string]any:
		for k, v := range schema {
			params[k] = v
		}
		if _, ok := params["type"]; !ok {
			params["type"] = "object"
		}
	case nil:
		// No schema — keep the default {"type": "object"}.
	default:
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP tool %q has unexpected InputSchema type %T, using empty object schema\n", t.Name, t.InputSchema)
	}

	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		},
	}
}

// CollectToolDefs gathers tool definitions from MCP clients, filtering out
// tools that conflict with built-in tools or were not successfully registered.
func CollectToolDefs(clients []*Client, reg *tool.Registry) []llm.ToolDef {
	var defs []llm.ToolDef
	seen := make(map[string]struct{})
	for _, c := range clients {
		for _, t := range c.Tools() {
			if tool.IsReserved(t.Name) {
				continue
			}
			if _, exists := reg.Get(t.Name); !exists {
				continue
			}
			if _, dup := seen[t.Name]; dup {
				continue
			}
			seen[t.Name] = struct{}{}
			defs = append(defs, ToToolDef(t))
		}
	}
	return defs
}
