// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"strings"
	"testing"
)

func TestHTMLTemplateLetsAgentOwnCompleteDocument(t *testing.T) {
	text, err := HTMLTemplate(DefaultHTMLTemplate)
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{
		"\u5e94\u7528\u4f1a\u6dfb\u52a0\u6837\u5f0f",
		"\u4e0d\u8981\u8f93\u51fa `<style>`",
		"\u901a\u8fc7\u6807\u51c6\u805a\u7126",
	}
	for _, phrase := range banned {
		if strings.Contains(text, phrase) {
			t.Fatalf("template still describes removed application HTML rewriting: %q", phrase)
		}
	}
	if !strings.Contains(text, "\u53ef\u76f4\u63a5\u6253\u5f00") {
		t.Fatal("template does not tell the model to produce a ready-to-open HTML document")
	}
	if !strings.Contains(text, "\u53c2\u8003") {
		t.Fatal("template does not describe the stylesheet as a reference resource")
	}

	multi, err := MultiHTMLTemplate(DefaultHTMLTemplate)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(multi, "data-report-kind") || strings.Contains(multi, "data-review-unit-count") {
		t.Fatal("multi template still requires a fixed DOM contract")
	}
	if !strings.Contains(multi, "\u8f93\u5165\u987a\u5e8f") {
		t.Fatal("multi template dropped input-order guidance")
	}
}
