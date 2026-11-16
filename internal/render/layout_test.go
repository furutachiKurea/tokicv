package render

import (
	"math"
	"strings"
	"testing"

	"tokicv/internal/dialect"
)

func TestLayoutPatchMergesOnlyExplicitFields(t *testing.T) {
	base := DefaultLayout()
	base.FontStack = FontClassic
	base.LineHeight = 1.40
	base.MarginVMM = 10
	base.SectionGapPX = 12
	base.ItemGapPX = 5

	font := FontSerif
	lineHeight := 1.30
	marginTop := 7.5
	marginBottom := 7.5
	sectionGap := 8.0
	itemGap := 3.0
	patch := LayoutPatch{
		FontStack:      &font,
		LineHeight:     &lineHeight,
		MarginTopMM:    &marginTop,
		MarginBottomMM: &marginBottom,
		SectionGapPX:   &sectionGap,
		ItemGapPX:      &itemGap,
	}

	got := patch.Apply(base)
	if got.FontStack != font || got.LineHeight != lineHeight ||
		got.MarginTopMM != marginTop || got.MarginBottomMM != marginBottom ||
		got.SectionGapPX != sectionGap || got.ItemGapPX != itemGap {
		t.Fatalf("Apply() = %#v, want the explicit patch values", got)
	}
	if got.MarginVMM != base.MarginVMM {
		t.Fatalf("MarginVMM = %v, want unchanged %v", got.MarginVMM, base.MarginVMM)
	}

	if got := (LayoutPatch{}).Apply(base); got != base {
		t.Fatalf("empty Apply() = %#v, want %#v", got, base)
	}
}

func TestLayoutValidationRejectsUnreadableAndOutOfRangeValues(t *testing.T) {
	valid := DefaultLayout().Normalized()
	if err := valid.Validate(); err != nil {
		t.Fatalf("default Layout is invalid: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Layout)
	}{
		{"font", func(layout *Layout) { layout.FontStack = "comic" }},
		{"line height below floor", func(layout *Layout) { layout.LineHeight = 1.19 }},
		{"line height above ceiling", func(layout *Layout) { layout.LineHeight = 2.21 }},
		{"line height NaN", func(layout *Layout) { layout.LineHeight = math.NaN() }},
		{"margin top below minimum", func(layout *Layout) { layout.MarginTopMM = 7.49 }},
		{"margin bottom above maximum", func(layout *Layout) { layout.MarginBottomMM = 30.01 }},
		{"margin V below minimum", func(layout *Layout) { layout.MarginVMM = 7.49 }},
		{"margin V above maximum", func(layout *Layout) { layout.MarginVMM = 31 }},
		{"section gap below minimum", func(layout *Layout) { layout.SectionGapPX = 4.99 }},
		{"section gap above maximum", func(layout *Layout) { layout.SectionGapPX = 41 }},
		{"item gap below minimum", func(layout *Layout) { layout.ItemGapPX = 1.49 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			layout := valid
			tc.mutate(&layout)
			if err := layout.Validate(); err == nil {
				t.Fatalf("Validate() accepted invalid Layout %#v", layout)
			}
		})
	}
}

func TestLayoutNormalizedClampsSpatialValues(t *testing.T) {
	layout := Layout{
		LineHeight:     0.8,
		MarginTopMM:    1,
		MarginBottomMM: 40,
		SectionGapPX:   -1,
		ItemGapPX:      50,
	}
	got := layout.Normalized()
	if got.LineHeight != MinLineHeight {
		t.Errorf("LineHeight = %v, want %v", got.LineHeight, MinLineHeight)
	}
	if got.MarginTopMM != MinMarginMM {
		t.Errorf("MarginTopMM = %v, want %v", got.MarginTopMM, MinMarginMM)
	}
	if got.MarginBottomMM != MaxMarginMM {
		t.Errorf("MarginBottomMM = %v, want %v", got.MarginBottomMM, MaxMarginMM)
	}
	if got.SectionGapPX != DefaultLayout().SectionGapPX {
		t.Errorf("SectionGapPX = %v, want the default", got.SectionGapPX)
	}
	if got.ItemGapPX != MaxItemGapPX {
		t.Errorf("ItemGapPX = %v, want %v", got.ItemGapPX, MaxItemGapPX)
	}
	got.SectionGapPX = 1
	got.ItemGapPX = 0.5
	got = got.Normalized()
	if got.SectionGapPX != MinSectionGapPX {
		t.Errorf("SectionGapPX = %v, want %v", got.SectionGapPX, MinSectionGapPX)
	}
	if got.ItemGapPX != MinItemGapPX {
		t.Errorf("ItemGapPX = %v, want %v", got.ItemGapPX, MinItemGapPX)
	}
}

func TestRenderWithLayoutPatchKeepsFontSizesLocked(t *testing.T) {
	lineHeight := 1.25
	marginTop := 8.0
	marginBottom := 7.5
	html := RenderWithLayout(dialect.Document{}, DefaultLayout().WithPatch(LayoutPatch{
		LineHeight:     &lineHeight,
		MarginTopMM:    &marginTop,
		MarginBottomMM: &marginBottom,
	}))
	for _, want := range []string{
		"font-size: 22.5pt",
		"font-size: 15pt",
		"font-size: 11.5pt",
		"font-size: 10.5pt",
		"font-size: 10pt",
		"line-height: 1.25",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("RenderWithLayout() does not contain %q", want)
		}
	}
}

func TestSmartOnePageCandidatesTightenInDocumentedOrder(t *testing.T) {
	base := DefaultLayout().Normalized()
	got := SmartOnePageCandidates(base)

	want := make([]Layout, 0, 5)
	gentle := base
	gentle.SectionGapPX = 8
	gentle.ItemGapPX = 3
	want = append(want, gentle.Normalized())

	lineHeight := gentle
	lineHeight.LineHeight = 1.30
	want = append(want, lineHeight.Normalized())
	lineHeight.LineHeight = 1.20
	want = append(want, lineHeight.Normalized())

	extreme := lineHeight
	extreme.SectionGapPX = 5
	extreme.ItemGapPX = 1.5
	want = append(want, extreme.Normalized())

	margins := extreme
	margins.MarginTopMM = 7.5
	margins.MarginBottomMM = 7.5
	want = append(want, margins.Normalized())

	if len(got) != len(want) {
		t.Fatalf("SmartOnePageCandidates() returned %d layouts, want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("candidate[%d] = %#v, want %#v", index, got[index], want[index])
		}
		if index > 0 && !strictlyTighter(got[index], got[index-1]) {
			t.Errorf("candidate[%d] loosened the active Layout Profile: %#v -> %#v", index, got[index-1], got[index])
		}
	}
}

func TestSmartOnePageCandidatesStepThroughComfortableLineHeights(t *testing.T) {
	base := DefaultLayout().Normalized()
	base.SectionGapPX = 5
	base.ItemGapPX = 1.5
	base.MarginTopMM = 7.5
	base.MarginBottomMM = 7.5
	base.LineHeight = 1.50

	got := SmartOnePageCandidates(base)
	want := []float64{1.40, 1.30, 1.20}
	if len(got) != len(want) {
		t.Fatalf("SmartOnePageCandidates() returned %d layouts, want %d: %#v", len(got), len(want), got)
	}
	for index, lineHeight := range want {
		if got[index].LineHeight != lineHeight {
			t.Errorf("candidate[%d] LineHeight = %v, want %v", index, got[index].LineHeight, lineHeight)
		}
	}
}

func TestSmartOnePageCandidatesPreserveTighterCustomBaseline(t *testing.T) {
	base := DefaultLayout().Normalized()
	base.SectionGapPX = 10
	base.ItemGapPX = 2
	base.MarginTopMM = 8
	base.MarginBottomMM = 8
	base.LineHeight = 1.25
	base = base.Normalized()

	got := SmartOnePageCandidates(base)
	if len(got) != 4 {
		t.Fatalf("SmartOnePageCandidates() returned %d layouts, want 4: %#v", len(got), got)
	}

	want := make([]Layout, 0, 4)
	next := base
	next.SectionGapPX = 8
	want = append(want, next)
	next.LineHeight = 1.20
	want = append(want, next)
	next.SectionGapPX = 5
	next.ItemGapPX = 1.5
	want = append(want, next.Normalized())
	next.MarginTopMM = 7.5
	next.MarginBottomMM = 7.5
	want = append(want, next.Normalized())

	for index := range want {
		if got[index] != want[index] {
			t.Errorf("candidate[%d] = %#v, want %#v", index, got[index], want[index])
		}
	}
	for index, candidate := range got {
		if candidate.MarginTopMM > base.MarginTopMM || candidate.MarginBottomMM > base.MarginBottomMM {
			t.Errorf("candidate[%d] loosened vertical margins: %#v", index, candidate)
		}
		if candidate.ItemGapPX > base.ItemGapPX || candidate.SectionGapPX > base.SectionGapPX {
			t.Errorf("candidate[%d] loosened spacing: %#v", index, candidate)
		}
		if candidate.LineHeight > base.LineHeight {
			t.Errorf("candidate[%d] loosened line height: %#v", index, candidate)
		}
	}
}

func TestSmartOnePageCandidatesAreEmptyAtReadableFloor(t *testing.T) {
	base := DefaultLayout().Normalized()
	base.SectionGapPX = 5
	base.ItemGapPX = 1.5
	base.MarginTopMM = 7.5
	base.MarginBottomMM = 7.5
	base.LineHeight = MinLineHeight
	base = base.Normalized()

	if got := SmartOnePageCandidates(base); len(got) != 0 {
		t.Fatalf("SmartOnePageCandidates() = %#v, want no tighter candidates", got)
	}
}

func TestRenderWithLayoutExposesAnUpdatableStylesheet(t *testing.T) {
	html := RenderWithLayout(dialect.Document{}, DefaultLayout())
	if !strings.Contains(html, `<style id="tokicv-layout">`) {
		t.Fatal("RenderWithLayout() did not expose the updatable stylesheet")
	}

	layout := DefaultLayout().Normalized()
	layout.SectionGapPX = 8
	layout.ItemGapPX = 3
	layout.MarginTopMM = 7.5
	layout.MarginBottomMM = 7.5
	layout.LineHeight = 1.30
	script := ApplyLayoutScript(layout)
	for _, want := range []string{
		`getElementById("tokicv-layout")`,
		`--section-gap: 8px`,
		`--item-gap: 3px`,
		`--margin-top: 7.5mm`,
		`--margin-bottom: 7.5mm`,
		`line-height: 1.3`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("ApplyLayoutScript() does not contain %q", want)
		}
	}
}

func strictlyTighter(next, current Layout) bool {
	return next.SectionGapPX <= current.SectionGapPX &&
		next.ItemGapPX <= current.ItemGapPX &&
		next.MarginTopMM <= current.MarginTopMM &&
		next.MarginBottomMM <= current.MarginBottomMM &&
		next.LineHeight <= current.LineHeight &&
		(next.SectionGapPX < current.SectionGapPX ||
			next.ItemGapPX < current.ItemGapPX ||
			next.MarginTopMM < current.MarginTopMM ||
			next.MarginBottomMM < current.MarginBottomMM ||
			next.LineHeight < current.LineHeight)
}
