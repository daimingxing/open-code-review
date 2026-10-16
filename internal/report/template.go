// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"embed"
	"fmt"
)

const DefaultHTMLTemplate = "review-summary"
const multiSeverityNotCollectedInstruction = "summary.findings_by_severity.not_collected \u8868\u793a\u4e25\u91cd\u7b49\u7ea7\u672a\u91c7\u96c6\u7684\u95ee\u9898\u8bb0\u5f55\u6570\uff0c\u4e0d\u8ba1\u5165\u56db\u4e2a\u5df2\u77e5\u4e25\u91cd\u7b49\u7ea7\u6216\u5f53\u6210\u96f6\uff0c\u4e0d\u56fa\u5b9a\u663e\u793a\u6807\u7b7e\u3002"

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

func MultiHTMLTemplate(name string) (string, error) {
	content, err := HTMLTemplate(name)
	if err != nil {
		return "", err
	}
	return content + "\n\n" + multiSeverityNotCollectedInstruction + "\n\n" +
		"## \u591a\u5ba1\u67e5\u5355\u5143\u8981\u6c42\n\n" +
		"\u8f93\u5165\u5305\u542b\u591a\u4efd\u6750\u6599\uff0c\u6309\u7528\u6237\u8f93\u5165\u987a\u5e8f\u4fdd\u7559\u5404\u5ba1\u67e5\u5355\u5143\uff0c\u4e3a\u6bcf\u4efd\u6750\u6599\u63d0\u4f9b\u53ef\u7406\u89e3\u7684\u663e\u793a\u6807\u8bc6\u3002\u6807\u9898\u3001\u6807\u7b7e\u3001\u7ae0\u8282\u5185\u90e8\u7ed3\u6784\u548c\u6392\u7248\u53ef\u81ea\u884c\u51b3\u5b9a\u3002\n\n" +
		"\u4e0d\u540c\u5355\u5143\u5373\u4f7f finding ID \u76f8\u540c\u4e5f\u8981\u5206\u522b\u5217\u51fa\u3002\u6bcf\u4e2a\u5355\u5143\u7684\u57fa\u7840\u7edf\u8ba1\u5e94\u4e0e\u8be5\u4efd\u6750\u6599\u4e00\u81f4\u3002\n\n" +
		"\u6458\u8981\u3001\u4eba\u5458\u7edf\u8ba1\u53ca\u4e0d\u540c\u6750\u6599\u5171\u6709\u4fe1\u606f\u53ef\u4ee5\u6982\u62ec\u7ec4\u7ec7\uff0c\u4f46\u4e0d\u6309\u540d\u5b57\u63a8\u65ad\u8eab\u4efd\u76f8\u540c\uff0c\u4e0d\u5f97\u4ece\u4e0d\u540c\u4ed3\u5e93\u95ee\u9898\u63a8\u65ad\u8de8\u4ed3\u5e93\u7f3a\u9677\u3002\u4e0d\u5f97\u53d1\u660e\u6750\u6599\u4e2d\u6ca1\u6709\u7684\u8bc1\u636e\u3001\u7ed3\u8bba\u6216\u7edf\u8ba1\u3002", nil
}
