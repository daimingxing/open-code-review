// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"

	"github.com/alibaba/open-code-review/internal/llm"
)

func enableKnowledgeTools(defs []llm.ToolDef) error {
	for i := range defs {
		fn := &defs[i].Function
		if fn.Name != "code_search" && fn.Name != "file_find" && fn.Name != "file_read" {
			continue
		}
		properties, ok := fn.Parameters["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("knowledge tool %s requires an object properties schema", fn.Name)
		}
		if fn.Name == "file_read" {
			property, ok := properties["file_path"].(map[string]any)
			if !ok {
				return fmt.Errorf("knowledge tool file_read requires a file_path schema")
			}
			property["description"] = fmt.Sprint(property["description"]) + " An absolute path within knowledge_bases reads authorized disk knowledge, including ignored/untracked files."
		} else {
			properties["directory"] = map[string]any{
				"type":        "string",
				"description": "Optional absolute directory from knowledge_bases or one of its subdirectories. Searches authorized disk knowledge, including ignored/untracked files. Omit for source code; code_search uses file_patterns and file_find uses query_name to scope code paths. Knowledge patterns support glob, directory prefixes and :(exclude).",
			}
		}
		// 计划提示优先使用原始定义；必须与模型调用的参数结构同步，并保留自定义字段。 // allow-non-english: 用户要求中文代码注释
		var raw map[string]any
		if err := json.Unmarshal(fn.RawDefinition, &raw); err != nil {
			return fmt.Errorf("decode knowledge tool %s: %w", fn.Name, err)
		}
		raw["parameters"] = fn.Parameters
		var err error
		fn.RawDefinition, err = json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("encode knowledge tool %s: %w", fn.Name, err)
		}
	}
	return nil
}
