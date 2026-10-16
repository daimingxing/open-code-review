// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliverHTMLPublishesWithoutContentValidation(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.html")
	raw := "<!doctype html><html><body><script>x()</script><p>ok</p></body></html>"
	written, err := DeliverHTML(target, false, raw)
	if err != nil {
		t.Fatalf("DeliverHTML: %v", err)
	}
	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != raw {
		t.Fatalf("delivered content was altered")
	}
	if _, err := DeliverHTML(target, false, raw); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite error = %v", err)
	}
}
