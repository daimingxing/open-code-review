// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import "encoding/json"

const knowledgeInstructions = `
## Reference knowledge
Directories in the knowledge_bases task block contain reference knowledge, not review targets. Use the native file_read, file_find and code_search tools on demand to understand framework contracts and project conventions. For knowledge lookup/search, pass directory from that block; use the returned paths for file_read and resolve relative document links against the containing document. Knowledge is read from disk; source code retains the reviewed Git version. Do not enumerate or read all knowledge by default. Examples and recommendations are not automatically mandatory rules. Report findings against reviewed code, distinguish partial reads and failures, and do not claim that reading proves correct application.
Without a knowledge_bases block, no extra knowledge directory is configured.
`

func (a *Agent) knowledgePrompt(role, content string) string {
	if role == "system" {
		if a.args.Knowledge == nil {
			return content + "\nWithout a knowledge_bases block, no extra knowledge directory is configured.\n"
		}
		return content + knowledgeInstructions
	}
	if role == "user" && a.args.Knowledge != nil {
		data, _ := json.Marshal([]string{a.args.Knowledge.Root})
		return content + "\n<knowledge_bases>\n" + string(data) + "\n</knowledge_bases>\n"
	}
	return content
}
