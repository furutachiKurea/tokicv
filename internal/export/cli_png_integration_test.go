//go:build linux

package export

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"tokicv/internal/render"
)

func TestCLICheckJSONReportsPageFitAndDiagnostics(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	input := filepath.Join(temp, "resume.md")
	if err := os.WriteFile(input, []byte("# 示例文档\n\nicon:unknown 内容\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, isolatedEnv(temp), "check", input, "--json", "--timeout", "25s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}

	var got CheckResult
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("stdout is not a CheckResult: %v\n%s", err, result.stdout)
	}
	if got.NumPages != 1 || got.Overflowed {
		t.Fatalf("Page Fit = %d pages, overflowed=%t, want one page", got.NumPages, got.Overflowed)
	}
	if got.AppliedProfile != render.DefaultLayout().Normalized() {
		t.Fatalf("appliedProfile = %#v, want the default Layout Profile", got.AppliedProfile)
	}
	if got.Diagnostics == nil || len(got.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one diagnostic", got.Diagnostics)
	}
	diagnostic := got.Diagnostics[0]
	if diagnostic.Line != 3 || diagnostic.Code != "unknown_icon" {
		t.Fatalf("diagnostic = %#v, want an unknown_icon on line 3", diagnostic)
	}
}

func TestCLIExportPNGMatchesPDFPageCount(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)

	for _, tc := range []struct {
		name   string
		source string
		pages  int
	}{
		{
			name: "one page",
			source: `# 示例文档

## 技能

- 熟悉 Go 与 WebKitGTK，参与 [项目文档](https://example.com/docs) 的维护
`,
			pages: 1,
		},
		{
			name:   "multiple pages",
			source: multiPageResume(18),
			pages:  2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			temp := t.TempDir()
			input := filepath.Join(temp, "resume.md")
			pdfPath := filepath.Join(temp, "resume.pdf")
			pngDir := filepath.Join(temp, "png")
			if err := os.WriteFile(input, []byte(tc.source), 0o644); err != nil {
				t.Fatal(err)
			}

			result := runBinary(t, binary, isolatedEnv(temp),
				"export", input, "--pdf", pdfPath, "--png", pngDir,
				"--timeout", "30s")
			if result.code != 0 {
				t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
			}

			pdf, err := os.ReadFile(pdfPath)
			if err != nil {
				t.Fatal(err)
			}
			pages := pdfTextGlyphs(t, pdf)
			if len(pages) < tc.pages {
				t.Fatalf("PDF has %d pages, want at least %d", len(pages), tc.pages)
			}

			entries, err := os.ReadDir(pngDir)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), "page-") && strings.HasSuffix(entry.Name(), ".png") {
					names = append(names, entry.Name())
				}
			}
			sort.Strings(names)
			if len(names) != len(pages) {
				t.Fatalf("PNG count = %d (%v), PDF page count = %d", len(names), names, len(pages))
			}
			for index, name := range names {
				if want := fmt.Sprintf("page-%d.png", index+1); name != want {
					t.Fatalf("PNG[%d] = %q, want %q", index, name, want)
				}
				file, err := os.Open(filepath.Join(pngDir, name))
				if err != nil {
					t.Fatal(err)
				}
				config, err := png.DecodeConfig(file)
				closeErr := file.Close()
				if err != nil {
					t.Fatalf("%s is not a valid PNG: %v", name, err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
				if config.Width < 1000 || config.Height < 1500 {
					t.Errorf("%s dimensions = %dx%d, want high-DPI A4", name, config.Width, config.Height)
				}
				ratio := float64(config.Width) / float64(config.Height)
				if got, want := ratio, 210.0/297.0; got < want-0.01 || got > want+0.01 {
					t.Errorf("%s aspect = %.4f, want %.4f", name, got, want)
				}
			}
		})
	}
}

func TestCLIExportPNGOnlySingleFile(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.png")
	if err := os.WriteFile(input, []byte("# 示例文档\n\n## 技能\n\n- Go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, isolatedEnv(temp), "export", input, "--png", output, "--timeout", "25s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(file)
	_ = file.Close()
	if err != nil {
		t.Fatalf("output is not a valid PNG: %v", err)
	}
	if config.Width < 1000 || config.Height < 1500 {
		t.Fatalf("PNG dimensions = %dx%d, want high-DPI A4", config.Width, config.Height)
	}
	if _, err := os.Stat(filepath.Join(temp, "resume-1.png")); !os.IsNotExist(err) {
		t.Fatalf("single-page .png output also created a suffixed file: %v", err)
	}
}

func TestCLIExportPNGTimeoutLeavesNoArtifacts(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	input := filepath.Join(temp, "resume.md")
	pngDir := filepath.Join(temp, "png")
	if err := os.WriteFile(input, []byte("# 示例文档\n\n超时测试\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, isolatedEnv(temp),
		"export", input, "--png", pngDir, "--timeout", "1ms")
	if result.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", result.code, result.stderr)
	}
	if !strings.Contains(result.stderr, "超时") {
		t.Errorf("stderr = %q, want a timeout error", result.stderr)
	}
	entries, err := os.ReadDir(pngDir)
	if err == nil && len(entries) != 0 {
		t.Fatalf("PNG output directory has %d entries after timeout", len(entries))
	}
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestCLIExportCombinedFailureLeavesArtifactsUntouched(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	input := filepath.Join(temp, "resume.md")
	pngDir := filepath.Join(temp, "png")
	pdfPath := filepath.Join(temp, "missing", "resume.pdf")
	if err := os.WriteFile(input, []byte("# 示例文档\n\n## 技能\n\n- Go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(pngDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existingPNG := filepath.Join(pngDir, "page-1.png")
	if err := os.WriteFile(existingPNG, []byte("existing page"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, isolatedEnv(temp),
		"export", input, "--png", pngDir, "--pdf", pdfPath, "--timeout", "25s")
	if result.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", result.code, result.stderr)
	}
	if _, err := os.Stat(pdfPath); !os.IsNotExist(err) {
		t.Fatalf("PDF exists after a failed export: %v", err)
	}
	if got, err := os.ReadFile(existingPNG); err != nil || string(got) != "existing page" {
		t.Fatalf("existing PNG changed after a failed export: %q, %v", got, err)
	}
	entries, err := os.ReadDir(pngDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "page-1.png" {
		t.Fatalf("PNG directory after a failed export = %v, want only the existing page", entryNames(entries))
	}
	if err := filepath.Walk(temp, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(info.Name(), ".tmp-") {
			t.Errorf("temporary file remains after a failed export: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCLICheckStrictOnePageExitCode(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	input := filepath.Join(temp, "resume.md")
	if err := os.WriteFile(input, []byte(multiPageResume(18)), 0o644); err != nil {
		t.Fatal(err)
	}
	env := isolatedEnv(temp)

	normal := runBinary(t, binary, env, "check", input, "--timeout", "30s")
	if normal.code != 0 {
		t.Fatalf("normal exit code = %d, stderr = %s", normal.code, normal.stderr)
	}
	if !strings.Contains(normal.stdout, "Page Fit: 1 / ") || strings.Contains(normal.stdout, "超出单页") {
		t.Fatalf("normal stdout = %q", normal.stdout)
	}

	strict := runBinary(t, binary, env, "check", input, "--strict-one-page", "--timeout", "30s")
	if strict.code != 2 {
		t.Fatalf("strict exit code = %d, want 2; stderr = %s", strict.code, strict.stderr)
	}
	if !strings.Contains(strict.stdout, "Page Fit: 1 / ") || !strings.Contains(strict.stdout, "超出单页") {
		t.Fatalf("strict stdout = %q, want Page Fit and overflow notice", strict.stdout)
	}
}

func TestCLICheckUnreadableInputFailsCleanly(t *testing.T) {
	binary := testBinary(t)
	temp := t.TempDir()

	result := runBinary(t, binary, isolatedEnv(temp), "check", filepath.Join(temp, "missing.md"))
	if result.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", result.code, result.stderr)
	}
	if !strings.Contains(result.stderr, "读取") {
		t.Errorf("stderr = %q, want a read error", result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout = %q, want empty", result.stdout)
	}
}

func TestCLICheckTimeoutFailsCleanly(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	input := filepath.Join(temp, "resume.md")
	if err := os.WriteFile(input, []byte("# 示例文档\n\n超时测试\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, isolatedEnv(temp), "check", input, "--timeout", "1ms")
	if result.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", result.code, result.stderr)
	}
	if !strings.Contains(result.stderr, "超时") {
		t.Errorf("stderr = %q, want a timeout error", result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout = %q, want empty", result.stdout)
	}
}

func multiPageResume(sections int) string {
	var source strings.Builder
	source.WriteString("# 示例文档\n\n")
	for section := 1; section <= sections; section++ {
		fmt.Fprintf(&source, "## 第 %d 部分\n\n", section)
		fmt.Fprintf(&source, "这是第 %d 部分的说明文字，包含用于填充分页的内容。\n\n", section)
		for item := 1; item <= 6; item++ {
			fmt.Fprintf(&source, "- 第 %d 部分的第 %d 条经历内容\n", section, item)
		}
		source.WriteString("\n")
	}
	return source.String()
}
