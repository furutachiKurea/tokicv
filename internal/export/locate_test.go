package export

import "testing"

func TestLayoutLineForTargetUsesVerticalPosition(t *testing.T) {
	lines := []layoutLine{
		{Left: 39.68, Top: 100, Right: 120, Bottom: 114, FontSize: 14.23},
		{Left: 39.68, Top: 80, Right: 120, Bottom: 94, FontSize: 14.23},
	}
	target := linkTarget{
		URI:      "https://example.com/repeat",
		X:        39.68,
		Y:        80,
		Width:    30,
		Height:   14,
		FontSize: 14.23,
	}

	if got := layoutLineForTarget(target, lines); got != 1 {
		t.Fatalf("layout line = %d, want the lower line with the same horizontal geometry", got)
	}
}

func TestLayoutLineForTargetPrefersHorizontalOverlap(t *testing.T) {
	lines := []layoutLine{
		{Left: 39.68, Top: 100, Right: 160, Bottom: 114, FontSize: 14.23},
		{Left: 260, Top: 100, Right: 380, Bottom: 114, FontSize: 14.23},
	}
	target := linkTarget{
		URI:      "https://example.com/right",
		X:        260,
		Y:        100,
		Width:    30,
		Height:   14,
		FontSize: 14.23,
	}

	if got := layoutLineForTarget(target, lines); got != 1 {
		t.Fatalf("layout line = %d, want the line overlapping the link rect", got)
	}
}

func TestLayoutLineForTargetRejectsALinkOutsideEveryLayoutLine(t *testing.T) {
	lines := []layoutLine{
		{Left: 39.68, Top: 100, Right: 380, Bottom: 114, FontSize: 10.67},
	}
	target := linkTarget{
		URI:      "https://example.com/detached",
		X:        39.68,
		Y:        40,
		Width:    30,
		Height:   14,
		FontSize: 10.67,
	}

	if got := layoutLineForTarget(target, lines); got != -1 {
		t.Fatalf("layout line = %d, want no match instead of a neighbouring line", got)
	}
}

func TestMapLayoutLinesSkipsExtraPDFLine(t *testing.T) {
	layoutLines := []layoutLine{
		{Left: 39.68, Top: 100, Right: 520, Bottom: 114, FontSize: 14.23},
		{Left: 39.68, Top: 80, Right: 300, Bottom: 94, FontSize: 14.23},
	}
	pdfLines := []pdfLine{
		testPDFLine(0, 800, 39.68, 80),
		testPDFLine(0, 780, 39.68, 350.32),
		testPDFLine(0, 760, 39.68, 185.32),
	}

	got, err := mapLayoutLines(layoutLines, pdfLines)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("line map = %v, want %v", got, want)
		}
	}
}

func TestMappedPeerLineBridgesSplitColumnsOnOnePrintedBaseline(t *testing.T) {
	layoutLines := []layoutLine{
		{Left: 39.68, Top: 100, Right: 380, Bottom: 114, FontSize: 10.67},
		{Left: 480, Top: 100.4, Right: 558, Bottom: 114.4, FontSize: 10},
	}
	lineMap := []int{-1, 4}

	if got := mappedPeerLine(0, layoutLines, lineMap); got != 4 {
		t.Fatalf("mapped peer = %d, want the PDF line shared by both Split Block columns", got)
	}
}

func TestGlyphSpanExcludesAdjacentPlainText(t *testing.T) {
	glyphs := []pdfGlyph{
		{x: 39.68, width: 34, fontSize: 10.67},
		{x: 80, width: 34, fontSize: 10.67},
	}

	left, right, ok := glyphSpan(glyphs, 80, 114)
	if !ok {
		t.Fatal("the link span did not match any glyph")
	}
	if left != 80 || right != 114 {
		t.Fatalf("glyph span = [%.2f %.2f], want [80 114]", left, right)
	}
}

func TestGlyphSpanKeepsNarrowAdjacentLinksSeparate(t *testing.T) {
	glyphs := []pdfGlyph{
		{x: 39.68, width: 2.42, fontSize: 10.67},
		{x: 42.10, width: 2.42, fontSize: 10.67},
	}

	for _, tc := range []struct {
		name        string
		left, right float64
	}{
		{name: "first", left: 39.68, right: 42.10},
		{name: "second", left: 42.10, right: 44.52},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left, right, ok := glyphSpan(glyphs, tc.left, tc.right)
			if !ok {
				t.Fatal("the link span did not match any glyph")
			}
			if left != tc.left || right != tc.right {
				t.Fatalf("glyph span = [%.2f %.2f], want [%.2f %.2f]", left, right, tc.left, tc.right)
			}
		})
	}
}

func TestGlyphSpanClipsJitteredAndZeroWidthGlyphs(t *testing.T) {
	glyphs := []pdfGlyph{
		{x: 79.90, width: 2.40, fontSize: 10.67},
		{x: 83.00, width: 0, fontSize: 10.67},
		{x: 84.20, width: 2.40, fontSize: 10.67},
	}

	left, right, ok := glyphSpan(glyphs, 80, 84.50)
	if !ok {
		t.Fatal("the link span did not match any glyph")
	}
	if left != 80 || right != 83 {
		t.Fatalf("glyph span = [%.2f %.2f], want [80.00 83.00]", left, right)
	}
}

func testPDFLine(page int, baseline, x, width float64) pdfLine {
	return pdfLine{
		page:     page,
		baseline: baseline,
		glyphs: []pdfGlyph{{
			page:     page,
			baseline: baseline,
			x:        x,
			width:    width,
			fontSize: 10.67,
		}},
	}
}
