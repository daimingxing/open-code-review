// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"embed"
	"fmt"
)

const DefaultHTMLTemplate = "review-summary"

//go:embed templates/review-summary.md
var htmlTemplates embed.FS

func HTMLTemplate(name string) (string, error) {
	if name != DefaultHTMLTemplate {
		return "", fmt.Errorf("unknown report template %q", name)
	}
	content, err := htmlTemplates.ReadFile("templates/review-summary.md")
	if err != nil {
		return "", fmt.Errorf("load report template %q: %w", name, err)
	}
	return string(content), nil
}
