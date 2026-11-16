package render

import (
	"encoding/json"
	"fmt"
)

// SmartOnePageCandidates returns the deterministic Layout Profiles to try for
// Smart One-Page, from the active baseline to the readable line-height floor.
// Each step tightens only its own stage and never loosens a value already
// tighter in base.
func SmartOnePageCandidates(base Layout) []Layout {
	current := base.Normalized()
	var candidates []Layout

	appendCandidate := func(next Layout) {
		next = next.Normalized()
		if !layoutTighter(next, current) {
			return
		}
		candidates = append(candidates, next)
		current = next
	}

	for _, step := range []struct {
		section float64
		item    float64
	}{
		{section: 8, item: 3},
	} {
		next := current
		if step.section < next.SectionGapPX {
			next.SectionGapPX = step.section
		}
		if step.item < next.ItemGapPX {
			next.ItemGapPX = step.item
		}
		appendCandidate(next)
	}

	for _, lineHeight := range []float64{1.40, 1.30, MinLineHeight} {
		if lineHeight >= current.LineHeight {
			continue
		}
		next := current
		next.LineHeight = lineHeight
		appendCandidate(next)
	}

	next := current
	if MinSectionGapPX < next.SectionGapPX {
		next.SectionGapPX = MinSectionGapPX
	}
	if MinItemGapPX < next.ItemGapPX {
		next.ItemGapPX = MinItemGapPX
	}
	appendCandidate(next)

	next = current
	if MinMarginMM < next.MarginTopMM {
		next.MarginTopMM = MinMarginMM
	}
	if MinMarginMM < next.MarginBottomMM {
		next.MarginBottomMM = MinMarginMM
	}
	appendCandidate(next)

	return candidates
}

// LayoutStylesheet returns the normalized Resume Sheet CSS for layout.
func LayoutStylesheet(layout Layout) string {
	return stylesheet(layout.Normalized())
}

// ApplyLayoutScript replaces the rendered Resume Sheet stylesheet in place.
// It returns false when the page has no tokicv stylesheet or Sheet root.
func ApplyLayoutScript(layout Layout) string {
	css, err := json.Marshal(LayoutStylesheet(layout))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`(() => {
  const style = document.getElementById("tokicv-layout");
  const root = document.getElementById("sheet-root");
  if (!style || !root) return false;
  style.textContent = %s;
  void root.offsetWidth;
  return true;
})()`, css)
}

func layoutTighter(next, current Layout) bool {
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
