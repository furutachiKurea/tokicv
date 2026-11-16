//go:build linux

package export

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf16"

	pdftext "github.com/ledongthuc/pdf"

	"golang.org/x/text/unicode/norm"
)

var (
	binaryOnce      sync.Once
	binaryDir       string
	binaryPath      string
	binaryErr       error
	graphicsOnce    sync.Once
	graphicsRuntime string
	graphicsDisplay string
	graphicsProcess *exec.Cmd
	graphicsDBus    *exec.Cmd
	graphicsAddress string
	graphicsErr     error
	graphicsSkip    string
)

const (
	lockedBodyFontSizePT = 10.5
	pdfFontSizeTolerance = 0.15
	pdfLineStepTolerance = 0.75
)

func isolatedExportDBusConfig(runtimeDir string) []byte {
	return []byte(fmt.Sprintf(`<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:path=%s</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
  <limit name="max_connections_per_user">64</limit>
</busconfig>
`, filepath.Join(runtimeDir, "dbus.sock")))
}

func TestMain(m *testing.M) {
	code := m.Run()
	if binaryDir != "" {
		_ = os.RemoveAll(binaryDir)
	}
	if graphicsProcess != nil {
		_ = graphicsProcess.Process.Kill()
		_ = graphicsProcess.Wait()
	}
	if graphicsDBus != nil {
		_ = graphicsDBus.Process.Kill()
		_ = graphicsDBus.Wait()
	}
	if graphicsRuntime != "" {
		_ = os.RemoveAll(graphicsRuntime)
	}
	os.Exit(code)
}

func TestInfoCommandsDoNotCreatePathUserData(t *testing.T) {
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	version := mygoVersion(t)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"version", []string{"--version"}, "tokicv " + version},
		{"help", []string{"--help"}, "用法:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runBinary(t, binary, env, tc.args...)
			if result.code != 0 {
				t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
			}
			if !strings.Contains(result.stdout, tc.want) {
				t.Errorf("stdout = %q, want it to contain %q", result.stdout, tc.want)
			}
		})
	}

	for _, root := range []string{filepath.Join(temp, "config"), filepath.Join(temp, "data")} {
		if _, err := os.Stat(filepath.Join(root, "tokicv")); err == nil {
			t.Errorf("PathUserData was created under %s", root)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat PathUserData under %s: %v", root, err)
		}
	}
}

func mygoVersion(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Version == "" {
		t.Fatal("mygo.json has no version")
	}
	return meta.Version
}

func TestExportMixedCJKResumeToA4PDF(t *testing.T) {
	requireRequiredChineseLocale(t)
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.pdf")

	const source = `# 示例文档

## 工作经历

熟悉 **Go** 与 WebKitGTK，参与 [项目文档](https://example.com/docs) 的维护。正文① 全角Ａ

- 构建跨平台 A4 PDF 导出工具
- 保持 WebView 排版一致性
`
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--timeout", "25s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("output does not start with a PDF header: %q", pdf[:min(len(pdf), 16)])
	}

	mediaBoxes := regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([\d.]+)\s+([\d.]+)\s*\]`).FindAllSubmatch(pdf, -1)
	if len(mediaBoxes) == 0 {
		t.Fatal("PDF has no MediaBox")
	}
	for _, box := range mediaBoxes {
		width, _ := strconv.ParseFloat(string(box[1]), 64)
		height, _ := strconv.ParseFloat(string(box[2]), 64)
		if math.Abs(width-595) > 2 || math.Abs(height-842) > 2 {
			t.Errorf("MediaBox = %.2fx%.2f, want A4 595x842 pt", width, height)
		}
	}

	text := pdfText(t, pdf)
	for _, want := range []string{"示例文档", "工作经历", "项目文档", "正文①", "全角Ａ"} {
		for _, r := range want {
			if !strings.ContainsRune(text, r) {
				t.Errorf("PDF text does not contain %q (%U): %q", r, r, text)
			}
		}
	}
	if !pdfLinkRE.Match(pdf) {
		t.Error("PDF has no link annotation")
	}
	if !bytes.Contains(pdf, []byte(`/URI (https://example.com/docs)`)) {
		t.Error("PDF link annotation does not contain the expected URI")
	}
	t.Logf("acceptance export language=zh-CN pages=%d content-match=true", len(mediaBoxes))
}

func TestReferenceCalibrationFixtures(t *testing.T) {
	requireGraphicalLinux(t)
	binary := calibrationBinary(t)

	for _, tc := range []struct {
		name       string
		fixture    string
		h3Token    string
		lineHeight float64
		sectionGap float64
		itemGap    float64
	}{
		{name: "compact-120", fixture: "compact-120.md", h3Token: "repo:", lineHeight: 1.20},
		{name: "standard-130", fixture: "standard-130.md", h3Token: "技术栈", lineHeight: 1.30,
			sectionGap: 8, itemGap: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			temp := t.TempDir()
			output := filepath.Join(temp, tc.name+".pdf")
			fixture, err := filepath.Abs(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			args := []string{
				"--input", fixture,
				"--output", output,
				"--font", "noto",
				"--line-height", strconv.FormatFloat(tc.lineHeight, 'f', -1, 64),
			}
			if tc.sectionGap > 0 {
				args = append(args, "--section-gap", strconv.FormatFloat(tc.sectionGap, 'f', -1, 64))
			}
			if tc.itemGap > 0 {
				args = append(args, "--item-gap", strconv.FormatFloat(tc.itemGap, 'f', -1, 64))
			}
			result := runBinary(t, binary, isolatedEnv(temp), args...)
			if result.code != 0 {
				t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
			}
			pdf, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}

			assertPDFPageSize(t, pdf)
			pages := pdfTextGlyphs(t, pdf)
			if len(pages) != 1 {
				t.Fatalf("PDF has %d pages, want one calibrated Sheet", len(pages))
			}
			lines := pdfTextLines(pages)
			assertLockedPDFFontSizes(t, lines, tc.h3Token)
			assertPDFMargins(t, pages)
			assertPDFLineHeightStep(t, lines, lockedBodyFontSizePT*tc.lineHeight)

			values := make([]string, len(lines))
			for i, line := range lines {
				values[i] = line.Text
			}
			text := norm.NFKC.String(strings.Join(values, "\n"))
			for _, want := range []string{"示例文档", "教育背景", "实习经历", "项目经历", "开源经历", "技能"} {
				if !strings.Contains(text, want) {
					t.Errorf("calibrated PDF does not contain %q", want)
				}
			}
		})
	}
}

func TestExportFontStackFlagChangesPDFFont(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "font-stack.md")
	output := filepath.Join(temp, "font-stack.pdf")

	const source = `# 示例文档

## 技能

熟悉 **Go** 与 WebKitGTK，使用 ` + "`PDF`" + ` 输出验证字体栈
`
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--font", "serif", "--timeout", "25s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	fonts := pdfBaseFonts(t, pdf)
	for _, font := range fonts {
		if strings.Contains(strings.ToLower(font), "serif") {
			return
		}
	}
	t.Fatalf("PDF fonts = %v, want the selected serif font stack", fonts)
}

func TestExportMultiPageLinksFollowPrintedPages(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.pdf")

	var source strings.Builder
	source.WriteString("# 示例文档\n\n")
	for section := 1; section <= 25; section++ {
		fmt.Fprintf(&source, "## 第 %d 部分\n\n", section)
		fmt.Fprintf(&source, "这是第 %d 部分的说明文字，包含 [链接 %d](https://example.com/%d) 以及用于填充分页的内容。\n\n",
			section, section, section)
		for item := 1; item <= 6; item++ {
			fmt.Fprintf(&source, "- 第 %d 部分的第 %d 条经历内容\n", section, item)
		}
		source.WriteString("\n")
	}
	if err := os.WriteFile(input, []byte(source.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--timeout", "40s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	annotations := pdfLinkAnnotations(t, pdf)
	byURI := make(map[string][]pdfTestAnnotation)
	for _, annotation := range annotations {
		byURI[annotation.URI] = append(byURI[annotation.URI], annotation)
	}
	if len(byURI) != 25 {
		t.Fatalf("PDF has %d linked URIs, want 25", len(byURI))
	}
	glyphs := pdfTextGlyphs(t, pdf)
	if len(glyphs) < 4 {
		t.Fatalf("PDF has %d pages, want a multi-page document", len(glyphs))
	}

	for section := 1; section <= 25; section++ {
		uri := fmt.Sprintf("https://example.com/%d", section)
		found := byURI[uri]
		if len(found) == 0 {
			t.Fatalf("link %d has no /Subtype /Link annotation", section)
		}
		for _, annotation := range found {
			assertAnnotationCoversPrintedText(t, section, fmt.Sprintf("链接 %d", section), annotation, glyphs)
		}
	}

	boundary := byURI["https://example.com/6"]
	if len(boundary) != 1 {
		t.Fatalf("link 6 has %d annotations, want one", len(boundary))
	}
	if boundary[0].Page != 1 {
		t.Fatalf("link 6 annotation is on page %d, want page 2", boundary[0].Page+1)
	}
	if boundary[0].Rect[3] < pageHeightPT-20 {
		t.Fatalf("link 6 annotation top = %.2fpt, want it near the top of page 2", boundary[0].Rect[3])
	}
}

func TestExportDuplicateLinkTextFollowsLinkParagraph(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.pdf")

	const source = `# 示例文档

Repeat

[Repeat](https://example.com/repeat)
`
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--timeout", "25s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	annotations := pdfLinkAnnotations(t, pdf)
	if len(annotations) != 1 {
		t.Fatalf("PDF has %d annotations, want one", len(annotations))
	}
	annotation := annotations[0]
	if annotation.URI != "https://example.com/repeat" {
		t.Fatalf("annotation URI = %q", annotation.URI)
	}
	if annotation.Page != 0 {
		t.Fatalf("annotation is on page %d, want page 1", annotation.Page+1)
	}

	pages := pdfTextGlyphs(t, pdf)
	var repeats []pdfTestLine
	for _, line := range pdfTextLines(pages) {
		if compactPDFText(line.Text) == "Repeat" {
			repeats = append(repeats, line)
		}
	}
	if len(repeats) != 2 {
		t.Fatalf("PDF has %d plain or linked Repeat lines, want two", len(repeats))
	}
	sort.Slice(repeats, func(i, j int) bool { return repeats[i].Baseline > repeats[j].Baseline })
	if !annotationCoversBaseline(annotation, repeats[1].Baseline) {
		t.Fatalf("annotation rect %v does not cover the link Repeat baseline %.2f",
			annotation.Rect, repeats[1].Baseline)
	}
	if annotationCoversBaseline(annotation, repeats[0].Baseline) {
		t.Fatalf("annotation rect %v covers the plain Repeat baseline %.2f",
			annotation.Rect, repeats[0].Baseline)
	}
	if text := compactPDFText(annotationText(annotation, pages[annotation.Page])); text != "Repeat" {
		t.Fatalf("annotation text = %q, want the linked Repeat", text)
	}
}

func TestExportManyWrappedLinksFollowPrintedPages(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.pdf")

	const filler = "Continuation text keeps this paragraph flowing across several printed lines so that pagination advances predictably through the exported resume document without relying on headings or list markers for spacing."
	var source strings.Builder
	source.WriteString("# 示例文档\n\n")
	for index := 0; index < 40; index++ {
		fmt.Fprintf(&source, "[LINK%d](https://example.com/%d) %s %s %s\n\n",
			index, index, filler, filler, filler[:105])
	}
	if err := os.WriteFile(input, []byte(source.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--timeout", "45s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	pages := pdfTextGlyphs(t, pdf)
	if len(pages) != 5 {
		t.Fatalf("PDF has %d pages, want five", len(pages))
	}
	lines := pdfTextLines(pages)
	annotations := pdfLinkAnnotations(t, pdf)
	byURI := make(map[string][]pdfTestAnnotation)
	for _, annotation := range annotations {
		byURI[annotation.URI] = append(byURI[annotation.URI], annotation)
	}
	if len(annotations) != 40 || len(byURI) != 40 {
		t.Fatalf("PDF has %d annotations for %d URIs, want 40 each", len(annotations), len(byURI))
	}

	for index := 0; index < 40; index++ {
		token := fmt.Sprintf("LINK%d", index)
		uri := fmt.Sprintf("https://example.com/%d", index)
		found := byURI[uri]
		if len(found) != 1 {
			t.Fatalf("%s has %d annotations, want one", token, len(found))
		}
		printed, ok := pdfTextLineWithToken(lines, token)
		if !ok {
			t.Fatalf("PDF has no printed line containing %s", token)
		}
		annotation := found[0]
		if annotation.Page != printed.Page {
			t.Fatalf("%s annotation is on page %d, want page %d",
				token, annotation.Page+1, printed.Page+1)
		}
		if text := compactPDFText(annotationText(annotation, pages[annotation.Page])); text != token {
			t.Fatalf("%s annotation text = %q", token, text)
		}
	}

	for token, wantPage := range map[string]int{"LINK20": 2, "LINK39": 4} {
		annotation := byURI[fmt.Sprintf("https://example.com/%s", strings.TrimPrefix(token, "LINK"))][0]
		if annotation.Page != wantPage {
			t.Fatalf("%s annotation is on page %d, want page %d", token, annotation.Page+1, wantPage+1)
		}
	}
}

func TestExportLinkVariantsFollowPrintedText(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.pdf")

	wrapped := strings.Repeat("四行链接文本", 28)
	source := fmt.Sprintf(`# 示例文档

相邻 [重复](https://example.com/dup)[重复](https://example.com/dup)

加粗 [**加粗链接**](https://example.com/bold)

中文 [项目文档](https://example.com/cjk)

## 标题 [标题链接](https://example.com/heading)

- 列表 [列表链接一](https://example.com/list1)
- 列表 [列表链接二](https://example.com/list2)

[%s](https://example.com/wrapped)
`, wrapped)
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--timeout", "30s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	pages := pdfTextGlyphs(t, pdf)
	annotations := pdfLinkAnnotations(t, pdf)
	byURI := make(map[string][]pdfTestAnnotation)
	for _, annotation := range annotations {
		byURI[annotation.URI] = append(byURI[annotation.URI], annotation)
	}

	duplicates := byURI["https://example.com/dup"]
	if len(duplicates) != 2 {
		t.Fatalf("consecutive duplicate link has %d annotations, want two", len(duplicates))
	}
	for _, annotation := range duplicates {
		if text := compactPDFText(annotationText(annotation, pages[annotation.Page])); text != "重复" {
			t.Fatalf("duplicate link annotation text = %q", text)
		}
	}

	for uri, want := range map[string]string{
		"https://example.com/bold":    "加粗链接",
		"https://example.com/cjk":     "项目文档",
		"https://example.com/heading": "标题链接",
		"https://example.com/list1":   "列表链接一",
		"https://example.com/list2":   "列表链接二",
	} {
		found := byURI[uri]
		if len(found) != 1 {
			t.Fatalf("%s has %d annotations, want one", uri, len(found))
		}
		if text := compactPDFText(annotationText(found[0], pages[found[0].Page])); text != want {
			t.Fatalf("%s annotation text = %q, want %q", uri, text, want)
		}
	}

	wrappedAnnotations := byURI["https://example.com/wrapped"]
	if len(wrappedAnnotations) != 4 {
		t.Fatalf("wrapped link has %d annotations, want four", len(wrappedAnnotations))
	}
	sort.Slice(wrappedAnnotations, func(i, j int) bool {
		if wrappedAnnotations[i].Page != wrappedAnnotations[j].Page {
			return wrappedAnnotations[i].Page < wrappedAnnotations[j].Page
		}
		return wrappedAnnotations[i].Rect[3] > wrappedAnnotations[j].Rect[3]
	})
	var wrappedText strings.Builder
	for _, annotation := range wrappedAnnotations {
		wrappedText.WriteString(annotationText(annotation, pages[annotation.Page]))
	}
	if got := compactPDFText(wrappedText.String()); got != compactPDFText(wrapped) {
		t.Fatalf("wrapped link annotation text = %q, want %q", got, compactPDFText(wrapped))
	}
}

func TestExportAdjacentNarrowLinksKeepSeparateAnnotations(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.pdf")

	const source = `# 示例文档

[i](https://example.com/first)[i](https://example.com/second)
`
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--timeout", "25s")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", result.code, result.stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	annotations := pdfLinkAnnotations(t, pdf)
	if len(annotations) != 2 {
		t.Fatalf("PDF has %d annotations, want two", len(annotations))
	}
	byURI := make(map[string]pdfTestAnnotation)
	for _, annotation := range annotations {
		byURI[annotation.URI] = annotation
	}
	first, firstOK := byURI["https://example.com/first"]
	second, secondOK := byURI["https://example.com/second"]
	if !firstOK || !secondOK {
		t.Fatalf("PDF annotations = %v, want distinct first and second URIs", byURI)
	}
	if first.Page != second.Page {
		t.Fatalf("adjacent links are on pages %d and %d, want the same page", first.Page+1, second.Page+1)
	}

	pages := pdfTextGlyphs(t, pdf)
	for uri, annotation := range map[string]pdfTestAnnotation{
		"https://example.com/first":  first,
		"https://example.com/second": second,
	} {
		if text := compactPDFText(annotationText(annotation, pages[annotation.Page])); text != "i" {
			t.Fatalf("%s annotation text = %q, want only its own i", uri, text)
		}
	}

	leftAnnotation, rightAnnotation := first, second
	if leftAnnotation.Rect[0] > rightAnnotation.Rect[0] {
		leftAnnotation, rightAnnotation = rightAnnotation, leftAnnotation
	}
	if leftAnnotation.Rect[2] > rightAnnotation.Rect[0]+0.01 {
		t.Fatalf("adjacent link annotations overlap horizontally: %v and %v",
			leftAnnotation.Rect, rightAnnotation.Rect)
	}
}

func TestExportUnreadableInputFailsCleanly(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	output := filepath.Join(temp, "resume.pdf")

	result := runBinary(t, binary, env, "export", filepath.Join(temp, "missing.md"), "--pdf", output)
	if result.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", result.code, result.stderr)
	}
	if !strings.Contains(result.stderr, "读取") {
		t.Errorf("stderr = %q, want a read error", result.stderr)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output file exists after failure: %v", err)
	}
}

func TestExportTimeoutFailsCleanly(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "resume.md")
	output := filepath.Join(temp, "resume.pdf")
	if err := os.WriteFile(input, []byte("# 示例文档\n\n超时测试\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runBinary(t, binary, env, "export", input, "--pdf", output, "--timeout", "1ms")
	if result.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", result.code, result.stderr)
	}
	if !strings.Contains(result.stderr, "超时") {
		t.Errorf("stderr = %q, want a timeout error", result.stderr)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output file exists after timeout: %v", err)
	}
}

type pdfTestAnnotation struct {
	Page int
	Rect [4]float64
	URI  string
}

type pdfTestGlyph struct {
	x        float64
	baseline float64
	width    float64
	fontSize float64
	font     string
	text     string
}

type pdfTestLine struct {
	Page     int
	Baseline float64
	Text     string
	Glyphs   []pdfTestGlyph
}

func pdfLinkAnnotations(t *testing.T, pdf []byte) []pdfTestAnnotation {
	t.Helper()
	pages := findPDFPages(pdf)
	order := make([]int, 0, len(pages))
	bodies := make(map[int][]byte, len(pages))
	for _, page := range pages {
		if _, ok := bodies[page.Number]; !ok {
			order = append(order, page.Number)
		}
		bodies[page.Number] = page.Body
	}

	rectRE := regexp.MustCompile(`/Rect\s*\[\s*([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s*\]`)
	uriRE := regexp.MustCompile(`/URI\s*\(([^)]*)\)`)
	var annotations []pdfTestAnnotation
	for pageIndex, number := range order {
		body := bodies[number]
		match := pdfAnnotsRE.FindSubmatch(body)
		if match == nil || len(match[1]) == 0 || match[1][0] != '[' {
			continue
		}
		for _, ref := range pdfRefRE.FindAll(match[1], -1) {
			object := findPDFObject(pdf, string(ref))
			if !pdfLinkRE.Match(object) {
				continue
			}
			rect := rectRE.FindSubmatch(object)
			uri := uriRE.FindSubmatch(object)
			if rect == nil || uri == nil {
				continue
			}
			var values [4]float64
			for index := range values {
				value, err := strconv.ParseFloat(string(rect[index+1]), 64)
				if err != nil {
					t.Fatalf("parse link rect: %v", err)
				}
				values[index] = value
			}
			annotations = append(annotations, pdfTestAnnotation{
				Page: pageIndex,
				Rect: values,
				URI:  string(uri[1]),
			})
		}
	}
	return annotations
}

func pdfTextGlyphs(t *testing.T, pdf []byte) [][]pdfTestGlyph {
	t.Helper()
	pdf = stripType3FontEncodings(pdf)
	reader, err := pdftext.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		t.Fatal(err)
	}
	pages := make([][]pdfTestGlyph, reader.NumPage())
	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		for _, text := range reader.Page(pageNumber).Content().Text {
			pages[pageNumber-1] = append(pages[pageNumber-1], pdfTestGlyph{
				x:        text.X,
				baseline: text.Y,
				width:    text.W,
				fontSize: text.FontSize,
				font:     text.Font,
				text:     norm.NFKC.String(text.S),
			})
		}
	}
	return pages
}

var type3EncodingRE = regexp.MustCompile(`(?s)/Encoding\s*<</Type\s*/Encoding\s*/Differences\s*\[[^\]]*\]>>`)

func stripType3FontEncodings(pdf []byte) []byte {
	return type3EncodingRE.ReplaceAllFunc(pdf, func(match []byte) []byte {
		return bytes.Repeat([]byte(" "), len(match))
	})
}

func pdfTextLines(pages [][]pdfTestGlyph) []pdfTestLine {
	var lines []pdfTestLine
	for page, glyphs := range pages {
		sorted := append([]pdfTestGlyph(nil), glyphs...)
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i].baseline != sorted[j].baseline {
				return sorted[i].baseline > sorted[j].baseline
			}
			return sorted[i].x < sorted[j].x
		})
		for _, glyph := range sorted {
			if len(lines) == 0 ||
				lines[len(lines)-1].Page != page ||
				math.Abs(glyph.baseline-lines[len(lines)-1].Baseline) > 0.5 {
				lines = append(lines, pdfTestLine{
					Page:     page,
					Baseline: glyph.baseline,
				})
			}
			line := &lines[len(lines)-1]
			line.Text += glyph.text
			line.Glyphs = append(line.Glyphs, glyph)
		}
	}
	return lines
}

func assertPDFPageSize(t *testing.T, pdf []byte) {
	t.Helper()
	mediaBoxes := regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([\d.]+)\s+([\d.]+)\s*\]`).FindAllSubmatch(pdf, -1)
	if len(mediaBoxes) == 0 {
		t.Fatal("PDF has no MediaBox")
	}
	for _, box := range mediaBoxes {
		width, err := strconv.ParseFloat(string(box[1]), 64)
		if err != nil {
			t.Fatalf("parse MediaBox width: %v", err)
		}
		height, err := strconv.ParseFloat(string(box[2]), 64)
		if err != nil {
			t.Fatalf("parse MediaBox height: %v", err)
		}
		if math.Abs(width-pageWidthPT) > 2 || math.Abs(height-pageHeightPT) > 2 {
			t.Errorf("MediaBox = %.2fx%.2f, want A4 %.2fx%.2f pt", width, height, pageWidthPT, pageHeightPT)
		}
	}
}

func assertLockedPDFFontSizes(t *testing.T, lines []pdfTestLine, h3Token string) {
	t.Helper()
	for _, tc := range []struct {
		token string
		size  float64
	}{
		{token: "示例文档", size: 22.5},
		{token: "实习经历", size: 15},
		{token: h3Token, size: 11.5},
		{token: "此处为经历概述段落", size: 10.5},
		{token: "职位名称字段占位", size: 10},
		{token: "2099.01", size: 10},
	} {
		var found bool
		for _, line := range lines {
			for _, glyphs := range glyphRunsForToken(line, tc.token) {
				found = true
				for _, glyph := range glyphs {
					if math.Abs(glyph.fontSize-tc.size) > 0.15 {
						t.Errorf("%q glyph %q font size = %.2fpt, want %.2fpt",
							tc.token, glyph.text, glyph.fontSize, tc.size)
					}
				}
			}
		}
		if !found {
			t.Errorf("PDF has no line containing %q", tc.token)
		}
	}
}

func glyphRunsForToken(line pdfTestLine, token string) [][]pdfTestGlyph {
	var text strings.Builder
	starts := make([]int, len(line.Glyphs))
	ends := make([]int, len(line.Glyphs))
	for index, glyph := range line.Glyphs {
		starts[index] = text.Len()
		text.WriteString(glyph.text)
		ends[index] = text.Len()
	}

	value := text.String()
	var runs [][]pdfTestGlyph
	for offset := 0; offset < len(value); {
		found := strings.Index(value[offset:], token)
		if found < 0 {
			break
		}
		start := offset + found
		end := start + len(token)
		var run []pdfTestGlyph
		for index, glyph := range line.Glyphs {
			if ends[index] <= start {
				continue
			}
			if starts[index] >= end {
				break
			}
			run = append(run, glyph)
		}
		if len(run) > 0 {
			runs = append(runs, run)
		}
		offset = end
	}
	return runs
}

func assertPDFMargins(t *testing.T, pages [][]pdfTestGlyph) {
	t.Helper()
	minX, maxX := math.Inf(1), math.Inf(-1)
	minBaseline, maxBaseline := math.Inf(1), math.Inf(-1)
	for _, glyphs := range pages {
		for _, glyph := range glyphs {
			minX = math.Min(minX, glyph.x)
			maxX = math.Max(maxX, glyph.x+glyph.width)
			minBaseline = math.Min(minBaseline, glyph.baseline)
			maxBaseline = math.Max(maxBaseline, glyph.baseline)
		}
	}
	if math.Abs(minX-37.5) > 2.5 {
		t.Errorf("left content edge = %.2fpt, want calibrated 37.5pt", minX)
	}
	if math.Abs(maxX-(pageWidthPT-37.5)) > 3 {
		t.Errorf("right content edge = %.2fpt, want calibrated %.2fpt", maxX, pageWidthPT-37.5)
	}
	if top := pageHeightPT - maxBaseline; top < 20 || top > 65 {
		t.Errorf("top margin from first baseline = %.2fpt, want an unclipped 10mm margin", top)
	}
	if minBaseline < 20 {
		t.Errorf("bottom baseline = %.2fpt, want an unclipped bottom margin", minBaseline)
	}
	if minBaseline > 85 {
		t.Errorf("bottom baseline = %.2fpt, want a near-full calibrated Sheet", minBaseline)
	}
}

func assertPDFLineHeightStep(t *testing.T, lines []pdfTestLine, want float64) {
	t.Helper()
	for index, line := range lines {
		if !strings.Contains(line.Text, "示例条目一") {
			continue
		}
		if index+1 >= len(lines) || lines[index+1].Page != line.Page {
			t.Fatal("wrapped calibration paragraph has no continuation line")
		}
		step := line.Baseline - lines[index+1].Baseline
		if math.Abs(step-want) > pdfLineStepTolerance {
			t.Fatalf("body line step = %.2fpt, want %.2fpt ±%.2fpt", step, want, pdfLineStepTolerance)
		}
		return
	}
	t.Fatal("calibration paragraph is missing from the PDF")
}

func pdfBaseFonts(t *testing.T, pdf []byte) []string {
	t.Helper()
	fonts := make(map[string]bool)
	for _, expression := range []string{
		`/BaseFont\s*/([^\s/<>\[\]()]+)`,
		`/FontName\s*/([^\s/<>\[\]()]+)`,
	} {
		for _, match := range regexp.MustCompile(expression).FindAllSubmatch(pdf, -1) {
			fonts[string(match[1])] = true
		}
	}
	if len(fonts) == 0 {
		t.Fatal("PDF has no font names")
	}
	out := make([]string, 0, len(fonts))
	for font := range fonts {
		out = append(out, font)
	}
	sort.Strings(out)
	return out
}

func pdfTextLineWithToken(lines []pdfTestLine, token string) (pdfTestLine, bool) {
	for _, line := range lines {
		for offset := 0; offset < len(line.Text); {
			found := strings.Index(line.Text[offset:], token)
			if found < 0 {
				break
			}
			start := offset + found
			end := start + len(token)
			before := start == 0 || !unicode.IsDigit(rune(line.Text[start-1]))
			after := end == len(line.Text) || !unicode.IsDigit(rune(line.Text[end]))
			if before && after {
				return line, true
			}
			offset = start + 1
		}
	}
	return pdfTestLine{}, false
}

func annotationText(annotation pdfTestAnnotation, glyphs []pdfTestGlyph) string {
	var text strings.Builder
	left, bottom, right, top := annotation.Rect[0], annotation.Rect[1], annotation.Rect[2], annotation.Rect[3]
	for _, glyph := range glyphs {
		center := glyph.x + glyph.width/2
		if center < left-0.5 || center > right+0.5 {
			continue
		}
		if glyph.baseline < bottom-0.5 || glyph.baseline > top+0.5 {
			continue
		}
		text.WriteString(glyph.text)
	}
	return text.String()
}

func annotationCoversBaseline(annotation pdfTestAnnotation, baseline float64) bool {
	return baseline >= annotation.Rect[1]-0.5 && baseline <= annotation.Rect[3]+0.5
}

func compactPDFText(value string) string {
	return strings.Join(strings.Fields(value), "")
}

func assertAnnotationCoversPrintedText(t *testing.T, section int, want string, annotation pdfTestAnnotation, pages [][]pdfTestGlyph) {
	t.Helper()
	if annotation.Page < 0 || annotation.Page >= len(pages) {
		t.Fatalf("link %d annotation page %d is outside the PDF", section, annotation.Page+1)
	}
	left, bottom, right, top := annotation.Rect[0], annotation.Rect[1], annotation.Rect[2], annotation.Rect[3]
	if right <= left || top <= bottom {
		t.Fatalf("link %d annotation has an empty rect: %v", section, annotation.Rect)
	}

	var count int
	minX, maxX := math.Inf(1), math.Inf(-1)
	for _, glyph := range pages[annotation.Page] {
		if glyph.x < left-1.5 || glyph.x >= right-0.25 {
			continue
		}
		if glyph.baseline < bottom-1.5 || glyph.baseline > top+1.5 {
			continue
		}
		count++
		minX = math.Min(minX, glyph.x)
		maxX = math.Max(maxX, glyph.x+glyph.width)
	}
	if count < 2 {
		t.Fatalf("link %d annotation on page %d contains no printed link text", section, annotation.Page+1)
	}
	if math.Abs(minX-left) > 1.5 || math.Abs(maxX-right) > 1.5 {
		t.Fatalf("link %d annotation rect [%.2f %.2f %.2f %.2f] does not match printed text [%.2f %.2f]",
			section, left, bottom, right, top, minX, maxX)
	}
	if height := top - bottom; height < 10 || height > 18 {
		t.Fatalf("link %d annotation height = %.2fpt, want the rendered link line height", section, height)
	}
	if text := compactPDFText(annotationText(annotation, pages[annotation.Page])); text != compactPDFText(want) {
		t.Fatalf("link %d annotation text = %q, want %q", section, text, want)
	}
}

func requireGraphicalLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("WebKitGTK export verification runs on Linux")
	}
	if os.Getenv("CGO_ENABLED") == "0" {
		t.Skip("WebKitGTK export verification is excluded from the pure fast suite")
	}
	graphicsOnce.Do(func() {
		mutter, err := exec.LookPath("mutter")
		if err != nil {
			graphicsSkip = "mutter is required for isolated WebKitGTK export verification"
			return
		}
		if _, err := exec.LookPath("dbus-daemon"); err != nil {
			graphicsSkip = "dbus-daemon is required for isolated WebKitGTK settings"
			return
		}
		graphicsRuntime, err = os.MkdirTemp("", "tokicv-export-runtime-")
		if err != nil {
			graphicsErr = fmt.Errorf("create isolated runtime directory: %w", err)
			return
		}
		for _, name := range []string{"home", "config", "cache", "data"} {
			if err := os.MkdirAll(filepath.Join(graphicsRuntime, name), 0o700); err != nil {
				graphicsErr = fmt.Errorf("create isolated %s directory: %w", name, err)
				return
			}
		}
		graphicsAddress, graphicsDBus, err = startIsolatedExportDBus(graphicsRuntime)
		if err != nil {
			graphicsErr = err
			return
		}
		graphicsDisplay = "wayland-0"
		cmd := exec.Command(mutter,
			"--headless",
			"--wayland",
			"--no-x11",
			"--wayland-display", graphicsDisplay,
			"--virtual-monitor", "1440x900",
		)
		cmd.Env = append(environmentWithoutDisplay(os.Environ()),
			"HOME="+filepath.Join(graphicsRuntime, "home"),
			"XDG_CONFIG_HOME="+filepath.Join(graphicsRuntime, "config"),
			"XDG_DATA_HOME="+filepath.Join(graphicsRuntime, "data"),
			"XDG_CACHE_HOME="+filepath.Join(graphicsRuntime, "cache"),
			"XDG_RUNTIME_DIR="+graphicsRuntime,
			"WAYLAND_DISPLAY="+graphicsDisplay,
			"GDK_BACKEND=wayland",
			"GIO_USE_VFS=local",
			"GSETTINGS_BACKEND=memory",
			"DBUS_SESSION_BUS_ADDRESS="+graphicsAddress,
			"DISPLAY=",
		)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Start(); err != nil {
			graphicsErr = fmt.Errorf("start isolated mutter: %w", err)
			return
		}
		graphicsProcess = cmd
		socket := filepath.Join(graphicsRuntime, graphicsDisplay)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(socket); err == nil {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		graphicsErr = fmt.Errorf("isolated mutter did not create %s: %s", socket, output.String())
	})
	if graphicsSkip != "" {
		t.Skip(graphicsSkip)
	}
	if graphicsErr != nil {
		t.Fatal(graphicsErr)
	}
}

func startIsolatedExportDBus(runtimeDir string) (string, *exec.Cmd, error) {
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		return "", nil, err
	}
	config := filepath.Join(runtimeDir, "session.conf")
	if err := os.WriteFile(config, isolatedExportDBusConfig(runtimeDir), 0o600); err != nil {
		return "", nil, fmt.Errorf("write isolated D-Bus config: %w", err)
	}
	logFile, err := os.Create(filepath.Join(runtimeDir, "dbus.log"))
	if err != nil {
		return "", nil, fmt.Errorf("create isolated D-Bus log: %w", err)
	}
	cmd := exec.Command(daemon, "--config-file="+config, "--nofork", "--print-address=1")
	cmd.Stderr = logFile
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = logFile.Close()
		return "", nil, fmt.Errorf("open isolated D-Bus output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return "", nil, fmt.Errorf("start isolated D-Bus: %w", err)
	}
	_ = logFile.Close()
	type result struct {
		address string
		err     error
	}
	address := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		address <- result{address: strings.TrimSpace(line), err: err}
	}()
	select {
	case result := <-address:
		if result.err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return "", nil, fmt.Errorf("read isolated D-Bus address: %w", result.err)
		}
		if result.address == "" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return "", nil, errors.New("isolated D-Bus returned an empty address")
		}
		return result.address, cmd, nil
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", nil, errors.New("isolated D-Bus did not publish an address")
	}
}

func isLockedBodyFontSize(fontSize float64) bool {
	return math.Abs(fontSize-lockedBodyFontSizePT) <= pdfFontSizeTolerance
}

func testBinary(t *testing.T) string {
	t.Helper()
	binaryOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		binaryDir, binaryErr = os.MkdirTemp("", "tokicv-test-bin-")
		if binaryErr != nil {
			return
		}
		binaryPath = filepath.Join(binaryDir, "tokicv")
		cmd := exec.Command("go", "build", "-o", binaryPath, ".")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		output, err := cmd.CombinedOutput()
		if err != nil {
			binaryErr = fmt.Errorf("go build: %w\n%s", err, output)
		}
	})
	if binaryErr != nil {
		t.Fatal(binaryErr)
	}
	return binaryPath
}

func calibrationBinary(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	path := filepath.Join(t.TempDir(), "tokicv-calibration")
	cmd := exec.Command("go", "build", "-o", path, "./internal/export/testdata/calibration")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build calibration helper: %v\n%s", err, output)
	}
	return path
}

type binaryResult struct {
	stdout string
	stderr string
	code   int
}

func runBinary(t *testing.T, binary string, env []string, args ...string) binaryResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("tokicv %v did not exit before the test timeout", args)
	}
	code := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run tokicv %v: %v", args, err)
		}
		code = exit.ExitCode()
	}
	return binaryResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func isolatedEnv(temp string) []string {
	vars := map[string]string{
		"HOME":            filepath.Join(temp, "home"),
		"XDG_CONFIG_HOME": filepath.Join(temp, "config"),
		"XDG_DATA_HOME":   filepath.Join(temp, "data"),
		"XDG_CACHE_HOME":  filepath.Join(temp, "cache"),
	}
	if graphicsRuntime != "" {
		vars["XDG_RUNTIME_DIR"] = graphicsRuntime
		vars["WAYLAND_DISPLAY"] = graphicsDisplay
		vars["GDK_BACKEND"] = "wayland"
		vars["DBUS_SESSION_BUS_ADDRESS"] = graphicsAddress
		vars["GIO_USE_VFS"] = "local"
		vars["GSETTINGS_BACKEND"] = "memory"
		vars["DISPLAY"] = ""
	}
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	for key, value := range vars {
		env[key] = value
	}
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

func environmentWithoutDisplay(env []string) []string {
	skip := map[string]bool{
		"DISPLAY":                  true,
		"WAYLAND_DISPLAY":          true,
		"XDG_RUNTIME_DIR":          true,
		"GDK_BACKEND":              true,
		"HOME":                     true,
		"XDG_CONFIG_HOME":          true,
		"XDG_DATA_HOME":            true,
		"XDG_CACHE_HOME":           true,
		"DBUS_SESSION_BUS_ADDRESS": true,
		"GIO_USE_VFS":              true,
		"GSETTINGS_BACKEND":        true,
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if !skip[name] {
			out = append(out, entry)
		}
	}
	return out
}

func pdfText(t *testing.T, pdf []byte) string {
	t.Helper()
	objects := map[string][]byte{}
	header := regexp.MustCompile(`(?m)^(\d+) \d+ obj\s*`)
	for _, match := range header.FindAllSubmatchIndex(pdf, -1) {
		end := bytes.Index(pdf[match[1]:], []byte("endobj"))
		if end >= 0 {
			objects[string(pdf[match[2]:match[3]])] = pdf[match[1] : match[1]+end]
		}
	}
	refs := map[string]bool{}
	toUnicode := regexp.MustCompile(`/ToUnicode\s+(\d+)\s+\d+\s+R`)
	for _, object := range objects {
		for _, match := range toUnicode.FindAllSubmatch(object, -1) {
			refs[string(match[1])] = true
		}
	}

	var text strings.Builder
	for number := range refs {
		object := objects[number]
		start := bytes.Index(object, []byte("stream"))
		end := bytes.LastIndex(object, []byte("endstream"))
		if start < 0 || end < start {
			t.Fatalf("ToUnicode object %s has no stream", number)
		}
		data := bytes.TrimLeft(object[start+len("stream"):end], "\r\n")
		reader, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("ToUnicode object %s: %v", number, err)
		}
		cmap, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ToUnicode object %s: %v", number, err)
		}
		text.WriteString(cmapText(t, cmap))
	}
	return text.String()
}

func cmapText(t *testing.T, cmap []byte) string {
	t.Helper()
	var out strings.Builder
	sections := regexp.MustCompile(`(?s)begin(bfchar|bfrange)(.*?)end(?:bfchar|bfrange)`)
	tokens := regexp.MustCompile(`<([0-9A-Fa-f]*)>|\[|\]`)
	for _, section := range sections.FindAllSubmatch(cmap, -1) {
		parts := tokens.FindAllSubmatch(section[2], -1)
		for i := 0; i < len(parts); {
			if string(section[1]) == "bfchar" {
				out.WriteString(utf16Hex(t, string(parts[i+1][1])))
				i += 2
				continue
			}
			low, _ := strconv.ParseUint(string(parts[i][1]), 16, 32)
			high, _ := strconv.ParseUint(string(parts[i+1][1]), 16, 32)
			i += 2
			if string(parts[i][0]) == "[" {
				for i++; string(parts[i][0]) != "]"; i++ {
					out.WriteString(utf16Hex(t, string(parts[i][1])))
				}
				i++
				continue
			}
			first := string(parts[i][1])
			last, _ := strconv.ParseUint(first[len(first)-2:], 16, 8)
			for code := low; code <= high; code++ {
				out.WriteString(utf16Hex(t, first[:len(first)-2]+strconv.FormatUint(0x100+last+code-low, 16)[1:]))
			}
			i++
		}
	}
	return out.String()
}

func utf16Hex(t *testing.T, value string) string {
	t.Helper()
	data, err := hex.DecodeString(value)
	if err != nil || len(data)%2 != 0 {
		t.Fatalf("not UTF-16: <%s>", value)
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = uint16(data[2*i])<<8 | uint16(data[2*i+1])
	}
	return string(utf16.Decode(units))
}
