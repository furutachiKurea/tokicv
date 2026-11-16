package render

import (
	"strings"
	"testing"

	"tokicv/internal/dialect"
)

func TestPreviewHTMLPaginatesDOMWithoutPrinting(t *testing.T) {
	html := PreviewHTML(dialect.Parse("# 示例文档\n\n## 工作经历\n\n- 工程化\n"), DefaultLayout())

	for _, want := range []string{
		`id="stage"`,
		`background: #E0DFDB`,
		`width: 210mm`,
		`height: 297mm`,
		`outline: 1px solid rgba(24, 25, 27, 0.08)`,
		`outline-offset: -1px`,
		`document.createTreeWalker(measure, NodeFilter.SHOW_TEXT)`,
		`const ranges = []`,
		`viewport.style.height = (range.end - range.start) + "px"`,
		`viewport.dataset.pageStart = String(range.start)`,
		`overflow: hidden`,
		`document.title = "` + PreviewPageTitlePrefix + `" + document.documentElement.dataset.tokicvPages`,
		`setTimeout(done, 50)`,
		`<h1>示例文档</h1>`,
		`<h2>工作经历</h2>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("PreviewHTML() does not contain %q", want)
		}
	}
	if strings.Contains(html, "PrintToPDF") || strings.Contains(html, "printToPDF") {
		t.Error("PreviewHTML() reaches for the print pipeline")
	}
}

func TestPreviewHTMLKeepsSheetEdgeBlurFree(t *testing.T) {
	html := PreviewHTML(dialect.Document{}, DefaultLayout())
	rule := previewRule(t, html, ".sheet {")
	for _, want := range []string{
		`outline: 1px solid rgba(24, 25, 27, 0.08)`,
		`outline-offset: -1px`,
	} {
		if !strings.Contains(rule, want) {
			t.Errorf("Sheet rule does not keep %q", want)
		}
	}
	for _, banned := range []string{
		"box-shadow",
		"filter:",
		"blur(",
		"drop-shadow(",
	} {
		if strings.Contains(rule, banned) {
			t.Errorf("Sheet rule uses expensive %q", banned)
		}
	}
	slot := previewRule(t, html, ".sheet-slot {")
	for _, banned := range []string{
		"box-shadow",
		"filter:",
		"blur(",
		"drop-shadow(",
	} {
		if strings.Contains(slot, banned) {
			t.Errorf("Sheet slot rule uses expensive %q", banned)
		}
	}
}

func previewRule(t *testing.T, html, selector string) string {
	t.Helper()
	start := strings.Index(html, selector)
	if start < 0 {
		t.Fatalf("PreviewHTML() has no %q rule", selector)
	}
	end := strings.Index(html[start:], "}")
	if end < 0 {
		t.Fatalf("PreviewHTML() has an unterminated %q rule", selector)
	}
	return html[start : start+end]
}

func TestPreviewHTMLKeepsLockedSheetTypography(t *testing.T) {
	html := PreviewHTML(dialect.Document{}, DefaultLayout())
	for _, want := range []string{
		`font-size: 22.5pt`,
		`font-size: 15pt`,
		`font-size: 11.5pt`,
		`font-size: 10.5pt`,
		`font-size: 10pt`,
		`width: 183.542mm`,
		`--section-gap: 12px`,
		`--item-gap: 5px`,
		`overflow-wrap: break-word`,
		`.page-flow-content`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("PreviewHTML() does not keep %q", want)
		}
	}
}

func TestPreviewHTMLKeepsMinimumReadableScale(t *testing.T) {
	html := PreviewHTML(dialect.Document{}, DefaultLayout())
	for _, want := range []string{
		`align-items: safe center`,
		`Math.max(0.8, Math.min(availableWidth / pageWidth, 1))`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("PreviewHTML() does not keep %q", want)
		}
	}
	if strings.Contains(html, "availableHeight / pageHeight") {
		t.Error("PreviewHTML() still shrinks the Sheet to the window height")
	}
}

func TestPreviewHTMLPaginatesAtAggregatedVisualLineBoundaries(t *testing.T) {
	html := PreviewHTML(dialect.Parse("# 标题\n\n**粗体** 与 mixed text\n"), DefaultLayout())
	for _, want := range []string{
		`function measureLines(measureTop)`,
		`Math.abs(rect.top - line.top) <= tolerance`,
		`lines[index].bottom = lines[index + 1].top`,
		`line.bottom > start + 2 && line.bottom <= limit + 0.5`,
		"measure.replaceChildren(source.content.cloneNode(true));\n    await document.fonts.ready;",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("PreviewHTML() does not paginate with %q", want)
		}
	}
}

func TestPreviewPageTitleRoundTrip(t *testing.T) {
	for _, n := range []int{1, 2, 12} {
		title := PreviewPageTitle(n)
		got, ok := PreviewPageCount(title)
		if !ok || got != n {
			t.Errorf("PreviewPageCount(%q) = %d, %v; want %d, true", title, got, ok, n)
		}
	}
	for _, title := range []string{"tokicv", "tokicv:pages=", "tokicv:pages:2", "tokicv:pages=0"} {
		if _, ok := PreviewPageCount(title); ok {
			t.Errorf("PreviewPageCount(%q) accepted an invalid title", title)
		}
	}
}

func TestPreviewUpdatesInPlaceWithoutReloadingTheShell(t *testing.T) {
	layout := DefaultLayout()
	layout.FontStack = FontSerif
	layout.LineHeight = 1.25
	layout.MarginTopMM = 8
	layout.MarginBottomMM = 7.5
	layout.SectionGapPX = 8
	layout.ItemGapPX = 3
	frame := PreviewFrameFor(dialect.Parse("# 新标题\n\n- 新内容\n"), layout)

	if !strings.Contains(frame.HTML, `window.tokicvPreview = { update };`) {
		t.Fatal("the preview shell does not expose its in-place update entry")
	}
	script := PreviewUpdateScript(frame)
	for _, want := range []string{
		`window.tokicvPreview.update`,
		`新标题`,
		`新内容`,
		`fontFamily`,
		`Noto Serif CJK SC`,
		`lineHeight`,
		`"1.25"`,
		`marginTop`,
		`"8"`,
		`marginBottom`,
		`"7.5"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("PreviewUpdateScript() does not contain %q", want)
		}
	}
	for _, want := range []string{
		`source.innerHTML = frame.content`,
		`probe.dataset.marginTop`,
		`buildPages();`,
		`fit();`,
	} {
		if !strings.Contains(frame.HTML, want) {
			t.Errorf("preview shell does not contain in-place update step %q", want)
		}
	}
	if strings.Contains(script, "LoadHTML") {
		t.Error("PreviewUpdateScript() reloads the page instead of updating it")
	}
}
