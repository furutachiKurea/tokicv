//go:build linux

package export

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tokicv/internal/library"
)

func TestCLILibraryTargetsAcrossMarkdownPDFPNGAndCheck(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	sourcePath := filepath.Join(temp, "master-resume.md")
	markdownPath := filepath.Join(temp, "artifact.md")
	pdfPath := filepath.Join(temp, "artifact.pdf")
	pngPath := filepath.Join(temp, "artifact.png")

	const source = `# 示例文档

## 工作经历

熟悉 **Go** 与 WebKitGTK，参与 [项目文档](https://example.com/docs) 的维护

- 构建跨平台 A4 PDF 导出工具
- 保持 WebView 排版一致性
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	created := runBinary(t, binary, env, "create", "示例文档 - Go", "--from", sourcePath)
	if created.code != 0 {
		t.Fatalf("create exit code = %d, stderr = %s", created.code, created.stderr)
	}

	listed := runBinary(t, binary, env, "list", "--json")
	if listed.code != 0 {
		t.Fatalf("list exit code = %d, stderr = %s", listed.code, listed.stderr)
	}
	var documents []library.Document
	if err := json.Unmarshal([]byte(listed.stdout), &documents); err != nil {
		t.Fatalf("list --json output = %q: %v", listed.stdout, err)
	}
	if len(documents) != 1 || documents[0].Name != "示例文档 - Go" || documents[0].ID == "" {
		t.Fatalf("library documents = %#v, want the created Resume Document", documents)
	}
	id := documents[0].ID

	shown := runBinary(t, binary, env, "show", "示例文档 - Go")
	if shown.code != 0 {
		t.Fatalf("show exit code = %d, stderr = %s", shown.code, shown.stderr)
	}
	if shown.stdout != source {
		t.Fatalf("show stdout = %q, want exact source", shown.stdout)
	}

	exportedMarkdown := runBinary(t, binary, env, "export", "示例文档 - Go", "--md", markdownPath)
	if exportedMarkdown.code != 0 {
		t.Fatalf("export --md exit code = %d, stderr = %s", exportedMarkdown.code, exportedMarkdown.stderr)
	}
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(markdown, []byte(source)) {
		t.Fatalf("Markdown Artifact = %q, want exact source", markdown)
	}

	exportedPDF := runBinary(t, binary, env, "export", id, "--pdf", pdfPath, "--timeout", "30s")
	if exportedPDF.code != 0 {
		t.Fatalf("export --pdf exit code = %d, stderr = %s", exportedPDF.code, exportedPDF.stderr)
	}
	pdf, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("PDF Artifact does not start with a PDF header: %q", pdf[:min(len(pdf), 8)])
	}

	exportedPNG := runBinary(t, binary, env, "export", "示例文档 - Go", "--png", pngPath, "--timeout", "30s")
	if exportedPNG.code != 0 {
		t.Fatalf("export --png exit code = %d, stderr = %s", exportedPNG.code, exportedPNG.stderr)
	}
	file, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	config, decodeErr := png.DecodeConfig(file)
	closeErr := file.Close()
	if decodeErr != nil {
		t.Fatalf("PNG Artifact is invalid: %v", decodeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if config.Width < 1000 || config.Height < 1500 {
		t.Fatalf("PNG dimensions = %dx%d, want a high-DPI A4 Sheet", config.Width, config.Height)
	}

	checked := runBinary(t, binary, env, "check", id, "--json", "--timeout", "30s")
	if checked.code != 0 {
		t.Fatalf("check exit code = %d, stderr = %s", checked.code, checked.stderr)
	}
	var result CheckResult
	if err := json.Unmarshal([]byte(checked.stdout), &result); err != nil {
		t.Fatalf("check --json output = %q: %v", checked.stdout, err)
	}
	if result.NumPages != 1 || result.Overflowed {
		t.Fatalf("Page Fit = %d pages, overflowed=%t, want one Sheet", result.NumPages, result.Overflowed)
	}

	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, []byte(source)) {
		t.Fatalf("external source changed after library operations: %q", after)
	}
}

func TestCLICombinedMarkdownAndPDFFailureLeavesNoPartialOutput(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	sourcePath := filepath.Join(temp, "resume.md")
	markdownPath := filepath.Join(temp, "artifact.md")
	pdfPath := filepath.Join(temp, "missing", "artifact.pdf")
	if err := os.WriteFile(sourcePath, []byte("# 示例文档\n\n## 技能\n\n- Go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markdownPath, []byte("old markdown"), 0o644); err != nil {
		t.Fatal(err)
	}
	created := runBinary(t, binary, env, "create", "示例文档", "--from", sourcePath)
	if created.code != 0 {
		t.Fatalf("create exit code = %d, stderr = %s", created.code, created.stderr)
	}

	result := runBinary(t, binary, env, "export", "示例文档", "--md", markdownPath, "--pdf", pdfPath, "--timeout", "30s")
	if result.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", result.code, result.stderr)
	}
	if got, err := os.ReadFile(markdownPath); err != nil || string(got) != "old markdown" {
		t.Fatalf("Markdown Artifact changed after failed export: %q, %v", got, err)
	}
	if _, err := os.Stat(pdfPath); !os.IsNotExist(err) {
		t.Fatalf("PDF exists after failed export: %v", err)
	}
	if err := filepath.Walk(temp, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(info.Name(), ".tmp-") {
			t.Errorf("temporary file remains after failed export: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCLIExternalMarkdownExportDoesNotInitializeLibrary(t *testing.T) {
	binary := testBinary(t)
	temp := t.TempDir()
	configBlocker := filepath.Join(temp, "config-blocker")
	if err := os.WriteFile(configBlocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := setEnvValue(isolatedEnv(temp), "XDG_CONFIG_HOME", configBlocker)
	sourcePath := filepath.Join(temp, "source.md")
	const source = "# 示例文档\r\n\r\n- 精确字节\r\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(temp)
	for _, tc := range []struct {
		name   string
		target string
	}{
		{name: "relative", target: "source.md"},
		{name: "absolute", target: sourcePath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(temp, tc.name+".md")
			result := runBinary(t, binary, env, "export", tc.target, "--md", output)
			if result.code != 0 {
				t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != source {
				t.Fatalf("Markdown Artifact = %q, want exact source", got)
			}
		})
	}
}

func TestCLIExportRejectsConflictingOutputPaths(t *testing.T) {
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	sourcePath := filepath.Join(temp, "source.md")
	artifactPath := filepath.Join(temp, "artifact.md")
	normalizedArtifactPath := temp + string(filepath.Separator) + "unused" +
		string(filepath.Separator) + ".." + string(filepath.Separator) + "artifact.md"
	if err := os.WriteFile(sourcePath, []byte("# 示例文档\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("existing artifact"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, env,
		"export", sourcePath, "--md", artifactPath, "--pdf", normalizedArtifactPath)
	if result.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", result.code, result.stderr)
	}
	if !strings.Contains(result.stderr, "输出路径冲突") || !strings.Contains(result.stderr, artifactPath) {
		t.Fatalf("stderr = %q, want a conflict naming %q", result.stderr, artifactPath)
	}
	got, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing artifact" {
		t.Fatalf("conflicting artifact changed to %q", got)
	}
}

func TestCLIPureGoLibraryCommandsRunWithoutDisplay(t *testing.T) {
	binary := testBinary(t)
	temp := t.TempDir()
	env := envWithoutGraphicalDisplay(isolatedEnv(temp))
	sourcePath := filepath.Join(temp, "source.md")
	markdownPath := filepath.Join(temp, "artifact.md")
	const source = "# 示例文档\r\n\r\n- 原始字节\r\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"list"},
		{"create", "示例文档", "--from", sourcePath},
		{"show", "示例文档"},
		{"import", sourcePath, "--name", "导入稿"},
		{"export", "示例文档", "--md", markdownPath},
		{"delete", "导入稿"},
	} {
		result := runBinary(t, binary, env, args...)
		if result.code != 0 {
			t.Fatalf("%v exit code = %d, stderr = %s", args, result.code, result.stderr)
		}
		if args[0] == "show" && result.stdout != source {
			t.Fatalf("show stdout = %q, want exact source", result.stdout)
		}
	}
	got, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != source {
		t.Fatalf("Markdown Artifact = %q, want exact source", got)
	}
}

func envWithoutGraphicalDisplay(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "DISPLAY", "WAYLAND_DISPLAY":
			continue
		}
		out = append(out, entry)
	}
	return out
}

func setEnvValue(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name != key {
			out = append(out, entry)
		}
	}
	return append(out, key+"="+value)
}
