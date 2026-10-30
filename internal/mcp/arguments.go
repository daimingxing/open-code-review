// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// argumentError 是本地调用反馈，不能记成实际 MCP 读取或服务故障。 // allow-non-english: 用户要求中文代码注释
type argumentError struct{ feedback string }

func (e *argumentError) Error() string { return e.feedback }

func (c *Client) validateArguments(name string, args map[string]any) string {
	for _, t := range c.tools {
		if t.Name != name {
			continue
		}
		if t.InputSchema == nil {
			return ""
		}
		data, err := json.Marshal(t.InputSchema)
		if err != nil {
			return ""
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(data, &schema); err != nil {
			return ""
		}
		// 无法本地解释的声明仍交给服务；不下载远程引用或猜测额外约束。 // allow-non-english: 用户要求中文代码注释
		if schema.Schema != "" && schema.Schema != "https://json-schema.org/draft/2020-12/schema" && schema.Schema != "http://json-schema.org/draft-07/schema#" && schema.Schema != "https://json-schema.org/draft-07/schema#" {
			return ""
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			return ""
		}
		if args == nil {
			args = map[string]any{}
		}
		if resolved.Validate(args) == nil {
			return ""
		}
		// 校验器错误可能包含请求值；仅返回字段名与声明，避免回显凭据。 // allow-non-english: 用户要求中文代码注释
		fields := make([]string, 0, len(schema.Properties))
		for field, property := range schema.Properties {
			if property != nil {
				fields = append(fields, fmt.Sprintf("%s:%v", field, property.Type))
			}
		}
		sort.Strings(fields)
		missing := []string{}
		for _, field := range schema.Required {
			if _, ok := args[field]; !ok {
				missing = append(missing, field)
			}
		}
		sort.Strings(missing)
		invalid := []string{}
		for field, property := range schema.Properties {
			if value, ok := args[field]; ok && property != nil {
				if rule, err := property.Resolve(nil); err == nil && rule.Validate(value) != nil {
					invalid = append(invalid, field)
				}
			}
		}
		sort.Strings(invalid)
		received := make([]string, 0, len(args))
		for field := range args {
			received = append(received, field)
		}
		sort.Strings(received)
		return fmt.Sprintf("Error: local MCP argument validation rejected %q; no service call was made. Missing required fields: %v. Invalid declared fields: %v. Received field names: %v. Declared properties: %s. Required: %v. Check the tool inputSchema (including additional-property and value constraints), correct the arguments and try again.", name, missing, invalid, received, strings.Join(fields, ", "), schema.Required)
	}
	return fmt.Sprintf("Error: MCP tool %q is not available on this connection; no service call was made. Use the tools declared for this conversation.", name)
}
