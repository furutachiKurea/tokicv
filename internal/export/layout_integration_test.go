//go:build linux

package export

import (
	"bytes"
	"encoding/json"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"tokicv/internal/render"
)

type cliLayoutView struct {
	Target string        `json:"target,omitempty"`
	Origin string        `json:"origin"`
	Layout render.Layout `json:"layout"`
}

func TestCLILayoutCommandsArePureGoAndPreserveMarkdown(t *testing.T) {
	binary := testBinary(t)
	temp := t.TempDir()
	env := withoutGUIEnv(isolatedEnv(temp))
	sourcePath := filepath.Join(temp, "resume.md")
	source := []byte("# 示例文档\r\n\r\n- 原始字节\r\n")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatal(err)
	}

	before := runBinary(t, binary, env, "layout", "show", "--json")
	if before.code != 0 {
		t.Fatalf("layout show exit code = %d, stderr = %s", before.code, before.stderr)
	}
	var initial cliLayoutView
	if err := json.Unmarshal([]byte(before.stdout), &initial); err != nil {
		t.Fatalf("layout show --json output = %q: %v", before.stdout, err)
	}
	if initial.Origin != "default" || initial.Layout != render.DefaultLayout().Normalized() {
		t.Fatalf("initial layout view = %#v", initial)
	}
	targetBefore := runBinary(t, binary, env, "layout", "show", sourcePath, "--json")
	if targetBefore.code != 0 {
		t.Fatalf("layout show target exit code = %d, stderr = %s", targetBefore.code, targetBefore.stderr)
	}
	var initialTarget cliLayoutView
	if err := json.Unmarshal([]byte(targetBefore.stdout), &initialTarget); err != nil {
		t.Fatalf("layout show target --json output = %q: %v", targetBefore.stdout, err)
	}
	if initialTarget.Origin != "default" || initialTarget.Layout != render.DefaultLayout().Normalized() {
		t.Fatalf("initial target layout view = %#v", initialTarget)
	}
	if _, err := os.Stat(filepath.Join(temp, "config")); !os.IsNotExist(err) {
		t.Fatalf("layout show created user data: %v", err)
	}
	resetNoop := runBinary(t, binary, env, "layout", "reset")
	if resetNoop.code != 0 {
		t.Fatalf("layout reset without profiles exit code = %d, stderr = %s", resetNoop.code, resetNoop.stderr)
	}
	if _, err := os.Stat(filepath.Join(temp, "config")); !os.IsNotExist(err) {
		t.Fatalf("layout reset without profiles created user data: %v", err)
	}

	setGlobal := runBinary(t, binary, env, "layout", "set", "--font", "serif",
		"--line-height", "1.45", "--margin-v", "9", "--section-gap", "14", "--item-gap", "6")
	if setGlobal.code != 0 {
		t.Fatalf("layout set global exit code = %d, stderr = %s", setGlobal.code, setGlobal.stderr)
	}
	setPath := runBinary(t, binary, env, "layout", "set", sourcePath,
		"--font", "noto", "--line-height", "1.25")
	if setPath.code != 0 {
		t.Fatalf("layout set path exit code = %d, stderr = %s", setPath.code, setPath.stderr)
	}

	pathView := runBinary(t, binary, env, "layout", "show", sourcePath, "--json")
	if pathView.code != 0 {
		t.Fatalf("layout show path exit code = %d, stderr = %s", pathView.code, pathView.stderr)
	}
	var saved cliLayoutView
	if err := json.Unmarshal([]byte(pathView.stdout), &saved); err != nil {
		t.Fatalf("layout show path --json output = %q: %v", pathView.stdout, err)
	}
	if saved.Origin != "path" || saved.Layout.FontStack != render.FontNoto || saved.Layout.LineHeight != 1.25 ||
		saved.Layout.MarginTopMM != 9 {
		t.Fatalf("path layout view = %#v", saved)
	}

	layoutsPath := filepath.Join(temp, "config", "tokicv", "library", "layouts.json")
	if _, err := os.Stat(layoutsPath); err != nil {
		t.Fatalf("layout set did not write layouts.json at %s: %v", layoutsPath, err)
	}

	for _, args := range [][]string{
		{"layout", "reset", sourcePath},
		{"layout", "reset"},
	} {
		result := runBinary(t, binary, env, args...)
		if result.code != 0 {
			t.Fatalf("%v exit code = %d, stderr = %s", args, result.code, result.stderr)
		}
	}
	after := runBinary(t, binary, env, "layout", "show", "--json")
	if after.code != 0 {
		t.Fatalf("layout show after reset exit code = %d, stderr = %s", after.code, after.stderr)
	}
	var reset cliLayoutView
	if err := json.Unmarshal([]byte(after.stdout), &reset); err != nil {
		t.Fatalf("layout show after reset --json output = %q: %v", after.stdout, err)
	}
	if reset.Origin != "default" || reset.Layout != render.DefaultLayout().Normalized() {
		t.Fatalf("reset layout view = %#v", reset)
	}

	got, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, source) {
		t.Fatalf("layout commands changed Markdown: %q", got)
	}
}

func TestCLILayoutPrecedenceInExportAndCheck(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	sourcePath := filepath.Join(temp, "resume.md")
	source := []byte(`# 示例文档

## 技能

熟悉 Go、WebKitGTK 与跨平台桌面应用开发，长期维护高精度 A4 排版引擎，关注字体度量、分页稳定性、链接注释定位以及 PDF 与 PNG 的一致性，能够定位复杂布局回归并给出可验证的修复方案，同时持续优化文档渲染性能和自动化验证流程。
`)
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatal(err)
	}

	global := runBinary(t, binary, env, "layout", "set", "--font", "serif",
		"--line-height", "1.50", "--margin-v", "11", "--section-gap", "15", "--item-gap", "7")
	if global.code != 0 {
		t.Fatalf("layout set global exit code = %d, stderr = %s", global.code, global.stderr)
	}
	pathSet := runBinary(t, binary, env, "layout", "set", sourcePath,
		"--font", "noto", "--line-height", "1.30",
		"--margin-top", "8", "--margin-bottom", "7.5", "--section-gap", "9", "--item-gap", "3")
	if pathSet.code != 0 {
		t.Fatalf("layout set path exit code = %d, stderr = %s", pathSet.code, pathSet.stderr)
	}

	pathCheck := checkProfile(t, binary, env, sourcePath)
	if pathCheck.FontStack != render.FontNoto || pathCheck.LineHeight != 1.30 ||
		pathCheck.MarginTopMM != 8 || pathCheck.MarginBottomMM != 7.5 ||
		pathCheck.SectionGapPX != 9 || pathCheck.ItemGapPX != 3 {
		t.Fatalf("path check appliedProfile = %#v", pathCheck)
	}

	cliCheck := checkProfile(t, binary, env, sourcePath,
		"--font", "system", "--line-height", "1.80", "--margin-v", "8")
	if cliCheck.FontStack != render.FontSystem || cliCheck.LineHeight != 1.80 ||
		cliCheck.MarginTopMM != 8 || cliCheck.MarginBottomMM != 8 {
		t.Fatalf("CLI check appliedProfile = %#v", cliCheck)
	}

	pdfPath := filepath.Join(temp, "path.pdf")
	pngPath := filepath.Join(temp, "path.png")
	pathExport := runBinary(t, binary, env, "export", sourcePath,
		"--pdf", pdfPath, "--png", pngPath, "--timeout", "30s")
	if pathExport.code != 0 {
		t.Fatalf("path export exit code = %d, stderr = %s", pathExport.code, pathExport.stderr)
	}
	pathPDF, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	pngConfig, decodeErr := png.DecodeConfig(file)
	closeErr := file.Close()
	if decodeErr != nil || closeErr != nil {
		t.Fatalf("path PNG decode = %v, close = %v", decodeErr, closeErr)
	}
	if pngConfig.Width < 1000 || pngConfig.Height < 1500 {
		t.Fatalf("path PNG dimensions = %dx%d, want an A4 Sheet", pngConfig.Width, pngConfig.Height)
	}

	cliPDFPath := filepath.Join(temp, "cli.pdf")
	cliExport := runBinary(t, binary, env, "export", sourcePath,
		"--pdf", cliPDFPath, "--font", "noto", "--line-height", "1.80", "--timeout", "30s")
	if cliExport.code != 0 {
		t.Fatalf("CLI export exit code = %d, stderr = %s", cliExport.code, cliExport.stderr)
	}
	cliPDF, err := os.ReadFile(cliPDFPath)
	if err != nil {
		t.Fatal(err)
	}
	pathGap := averageBodyLineGap(t, pathPDF)
	cliGap := averageBodyLineGap(t, cliPDF)
	if cliGap <= pathGap+3 {
		t.Fatalf("CLI line-height gap = %.2fpt, path gap = %.2fpt; CLI flag did not win", cliGap, pathGap)
	}

	resetPath := runBinary(t, binary, env, "layout", "reset", sourcePath)
	if resetPath.code != 0 {
		t.Fatalf("layout reset path exit code = %d, stderr = %s", resetPath.code, resetPath.stderr)
	}
	globalCheck := checkProfile(t, binary, env, sourcePath)
	if globalCheck.FontStack != render.FontSerif || globalCheck.LineHeight != 1.50 ||
		globalCheck.MarginTopMM != 11 || globalCheck.SectionGapPX != 15 || globalCheck.ItemGapPX != 7 {
		t.Fatalf("global check appliedProfile = %#v", globalCheck)
	}

	resetGlobal := runBinary(t, binary, env, "layout", "reset")
	if resetGlobal.code != 0 {
		t.Fatalf("layout reset global exit code = %d, stderr = %s", resetGlobal.code, resetGlobal.stderr)
	}
	defaultCheck := checkProfile(t, binary, env, sourcePath)
	if defaultCheck != render.DefaultLayout().Normalized() {
		t.Fatalf("default check appliedProfile = %#v", defaultCheck)
	}

	got, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, source) {
		t.Fatalf("profile updates and exports changed Markdown: %q", got)
	}
}

func checkProfile(t *testing.T, binary string, env []string, source string, layoutArgs ...string) render.Layout {
	t.Helper()
	args := []string{"check", source, "--json", "--timeout", "30s"}
	args = append(args, layoutArgs...)
	result := runBinary(t, binary, env, args...)
	if result.code != 0 {
		t.Fatalf("check exit code = %d, stderr = %s", result.code, result.stderr)
	}
	var checked CheckResult
	if err := json.Unmarshal([]byte(result.stdout), &checked); err != nil {
		t.Fatalf("check --json output = %q: %v", result.stdout, err)
	}
	return checked.AppliedProfile
}

func averageBodyLineGap(t *testing.T, pdf []byte) float64 {
	t.Helper()
	lines := pdfTextLines(pdfTextGlyphs(t, pdf))
	var body []pdfTestLine
	for _, line := range lines {
		if len(line.Glyphs) == 0 {
			continue
		}
		fontSize := line.Glyphs[0].fontSize
		if isLockedBodyFontSize(fontSize) {
			body = append(body, line)
		}
	}
	if len(body) < 2 {
		t.Fatalf("PDF has %d body lines, want at least 2", len(body))
	}
	sort.SliceStable(body, func(i, j int) bool {
		if body[i].Page != body[j].Page {
			return body[i].Page < body[j].Page
		}
		return body[i].Baseline > body[j].Baseline
	})
	var total float64
	var count int
	for index := 1; index < len(body); index++ {
		if body[index].Page != body[index-1].Page {
			continue
		}
		gap := body[index-1].Baseline - body[index].Baseline
		if gap <= 0 || math.IsNaN(gap) {
			continue
		}
		total += gap
		count++
	}
	if count == 0 {
		t.Fatal("PDF has no adjacent body line gaps")
	}
	return total / float64(count)
}

func withoutGUIEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, "DISPLAY=") || strings.HasPrefix(entry, "WAYLAND_DISPLAY=") {
			continue
		}
		out = append(out, entry)
	}
	return out
}
