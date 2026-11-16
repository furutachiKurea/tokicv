//go:build linux

package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportFixesCompatibilityRadicalsAcrossPrintedPages(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	input := filepath.Join(temp, "cross-page.md")
	output := filepath.Join(temp, "cross-page.pdf")

	var source strings.Builder
	source.WriteString("# 示例文档\n\n## 增长记录\n\n")
	for line := 1; line <= 90; line++ {
		fmt.Fprintf(&source, "第 %d 行：示例增长第二页 ①Ａ，用于验证跨页复制文本。\n\n", line)
	}
	if err := os.WriteFile(input, []byte(source.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, isolatedEnv(temp), "export", input, "--pdf", output, "--timeout", "30s")
	if result.code != 0 {
		t.Fatalf("export exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if pages := len(pdfTextGlyphs(t, pdf)); pages < 2 {
		t.Fatalf("cross-page PDF has %d pages, want at least two", pages)
	}

	text := pdfText(t, pdf)
	for _, r := range text {
		if r >= 0x2E80 && r <= 0x2EFF || r >= 0x2F00 && r <= 0x2FDF {
			t.Fatalf("cross-page PDF text layer still contains compatibility radical %U", r)
		}
	}
	for _, want := range []rune("示例增长第二页①Ａ") {
		if !strings.ContainsRune(text, want) {
			t.Errorf("cross-page PDF text layer is missing %U", want)
		}
	}
}
