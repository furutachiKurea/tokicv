//go:build linux

package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tokicv/internal/render"
)

func TestCLIFitOnePageConvergesBorderlineResumeWithoutChangingType(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "borderline.md")
	pdfPath := filepath.Join(temp, "borderline.pdf")
	source := borderlineOnePageResume()
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	baseline := runBinary(t, binary, env, "check", input, "--json", "--timeout", "30s")
	if baseline.code != 0 {
		t.Fatalf("baseline check exit code = %d, stderr = %s", baseline.code, baseline.stderr)
	}
	var baselineResult CheckResult
	if err := json.Unmarshal([]byte(baseline.stdout), &baselineResult); err != nil {
		t.Fatalf("baseline check --json output = %q: %v", baseline.stdout, err)
	}
	if baselineResult.NumPages != 2 {
		t.Fatalf("baseline Page Fit = %d pages, want the calibrated 2-page overflow", baselineResult.NumPages)
	}

	checked := runBinary(t, binary, env, "check", input, "--fit-one-page", "--json", "--timeout", "30s")
	if checked.code != 0 {
		t.Fatalf("fit check exit code = %d, stderr = %s", checked.code, checked.stderr)
	}
	var checkedResult CheckResult
	if err := json.Unmarshal([]byte(checked.stdout), &checkedResult); err != nil {
		t.Fatalf("fit check --json output = %q: %v", checked.stdout, err)
	}
	if checkedResult.NumPages != 1 || checkedResult.Overflowed {
		t.Fatalf("fit Page Fit = %d pages, overflowed=%t, want one page", checkedResult.NumPages, checkedResult.Overflowed)
	}
	if checkedResult.Notice != "" {
		t.Fatalf("fit notice = %q, want none after convergence", checkedResult.Notice)
	}
	if checkedResult.AppliedProfile.SectionGapPX != 8 || checkedResult.AppliedProfile.ItemGapPX != 3 ||
		checkedResult.AppliedProfile.MarginTopMM != 10 || checkedResult.AppliedProfile.MarginBottomMM != 10 ||
		checkedResult.AppliedProfile.LineHeight != 1.40 {
		t.Fatalf("fit appliedProfile = %#v, want only the first spacing step", checkedResult.AppliedProfile)
	}

	exported := runBinary(t, binary, env, "export", input, "--pdf", pdfPath, "--fit-one-page", "--timeout", "30s")
	if exported.code != 0 {
		t.Fatalf("fit export exit code = %d, stderr = %s", exported.code, exported.stderr)
	}
	pdf, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	pages := pdfTextGlyphs(t, pdf)
	if len(pages) != 1 {
		t.Fatalf("fit PDF has %d pages, want one", len(pages))
	}
	lines := pdfTextLines(pages)
	line, ok := pdfTextLineWithToken(lines, "临界内容")
	if !ok {
		t.Fatal("fit PDF is missing the body text")
	}
	for _, glyph := range line.Glyphs {
		if !isLockedBodyFontSize(glyph.fontSize) {
			t.Errorf("body glyph %q font size = %.2fpt, want locked 10.5pt", glyph.text, glyph.fontSize)
		}
	}

	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, []byte(source)) {
		t.Fatalf("fit CLI changed Markdown source: %q", after)
	}
}

func TestCLIFitOnePageRestoresCustomBaselineWhenTheFloorStillOverflows(t *testing.T) {
	requireGraphicalLinux(t)
	binary := testBinary(t)
	temp := t.TempDir()
	env := isolatedEnv(temp)
	input := filepath.Join(temp, "multi-page.md")
	pdfPath := filepath.Join(temp, "multi-page.pdf")
	source := multiPageResume(10)
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	set := runBinary(t, binary, env, "layout", "set", input,
		"--font", "serif", "--line-height", "1.25",
		"--margin-top", "8", "--margin-bottom", "7.5",
		"--section-gap", "9", "--item-gap", "2")
	if set.code != 0 {
		t.Fatalf("layout set exit code = %d, stderr = %s", set.code, set.stderr)
	}

	baseline := runBinary(t, binary, env, "check", input, "--json", "--timeout", "30s")
	if baseline.code != 0 {
		t.Fatalf("baseline check exit code = %d, stderr = %s", baseline.code, baseline.stderr)
	}
	var baselineResult CheckResult
	if err := json.Unmarshal([]byte(baseline.stdout), &baselineResult); err != nil {
		t.Fatalf("baseline check --json output = %q: %v", baseline.stdout, err)
	}
	if baselineResult.NumPages < 2 || !baselineResult.Overflowed {
		t.Fatalf("baseline Page Fit = %d pages, overflowed=%t, want a multi-page Resume Document",
			baselineResult.NumPages, baselineResult.Overflowed)
	}
	wantProfile := render.DefaultLayout().Normalized()
	wantProfile.FontStack = render.FontSerif
	wantProfile.LineHeight = 1.25
	wantProfile.MarginTopMM = 8
	wantProfile.MarginBottomMM = 7.5
	wantProfile.SectionGapPX = 9
	wantProfile.ItemGapPX = 2
	wantProfile = wantProfile.Normalized()
	if baselineResult.AppliedProfile != wantProfile {
		t.Fatalf("baseline appliedProfile = %#v, want the custom baseline %#v", baselineResult.AppliedProfile, wantProfile)
	}

	fit := runBinary(t, binary, env, "check", input,
		"--fit-one-page", "--strict-one-page", "--json", "--timeout", "30s")
	if fit.code != 2 {
		t.Fatalf("fit strict exit code = %d, want 2; stderr = %s", fit.code, fit.stderr)
	}
	var fitResult CheckResult
	if err := json.Unmarshal([]byte(fit.stdout), &fitResult); err != nil {
		t.Fatalf("fit check --json output = %q: %v", fit.stdout, err)
	}
	if fitResult.NumPages != baselineResult.NumPages || !fitResult.Overflowed {
		t.Fatalf("fit Page Fit = %d pages, overflowed=%t, want restored %d-page baseline",
			fitResult.NumPages, fitResult.Overflowed, baselineResult.NumPages)
	}
	if fitResult.AppliedProfile != wantProfile {
		t.Fatalf("fit appliedProfile = %#v, want complete custom baseline %#v", fitResult.AppliedProfile, wantProfile)
	}
	if fitResult.Notice != ReadabilityLimitNotice {
		t.Fatalf("fit notice = %q, want %q", fitResult.Notice, ReadabilityLimitNotice)
	}

	exported := runBinary(t, binary, env, "export", input, "--pdf", pdfPath, "--fit-one-page", "--timeout", "30s")
	if exported.code != 0 {
		t.Fatalf("fit export exit code = %d, stderr = %s", exported.code, exported.stderr)
	}
	if !strings.Contains(exported.stderr, ReadabilityLimitNotice) {
		t.Fatalf("fit export stderr = %q, want the readability-limit notice", exported.stderr)
	}
	pdf, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(pdfTextGlyphs(t, pdf)); got != baselineResult.NumPages {
		t.Fatalf("fit PDF has %d pages, want the restored %d-page baseline", got, baselineResult.NumPages)
	}

	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, []byte(source)) {
		t.Fatalf("fit CLI changed Markdown source: %q", after)
	}
}

func borderlineOnePageResume() string {
	var source strings.Builder
	source.WriteString("# 示例文档\n\n## 技能\n\n")
	for line := 1; line <= 42; line++ {
		fmt.Fprintf(&source, "第 %d 行临界内容，用于验证智能一页收敛。\n\n", line)
	}
	return source.String()
}
