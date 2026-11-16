package export

import (
	"testing"

	"tokicv/internal/render"
)

func TestSolveOnePageKeepsAnAlreadyFittingBaseline(t *testing.T) {
	base := render.DefaultLayout().Normalized()
	var measured []render.Layout
	result, err := SolveOnePage(base, func(layout render.Layout) (int, error) {
		measured = append(measured, layout)
		return 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(measured) != 1 || measured[0] != base {
		t.Fatalf("measured layouts = %#v, want the active baseline only", measured)
	}
	if result.Layout != base || result.NumPages != 1 || result.Overflowed || result.Notice != "" {
		t.Fatalf("result = %#v, want the unchanged one-page baseline", result)
	}
}

func TestSolveOnePageStopsAtTheFirstFittingCandidate(t *testing.T) {
	base := render.DefaultLayout().Normalized()
	candidates := render.SmartOnePageCandidates(base)
	if len(candidates) < 3 {
		t.Fatalf("candidate count = %d, want at least 3", len(candidates))
	}

	var measured []render.Layout
	result, err := SolveOnePage(base, func(layout render.Layout) (int, error) {
		measured = append(measured, layout)
		if len(measured) <= 3 {
			return 2, nil
		}
		return 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := candidates[2]
	if result.Layout != want || result.NumPages != 1 || result.Overflowed || result.Notice != "" {
		t.Fatalf("result = %#v, want candidate %#v", result, want)
	}
	if len(measured) != 4 {
		t.Fatalf("measure count = %d, want baseline plus three candidates", len(measured))
	}
	for index := 1; index < len(measured); index++ {
		if measured[index] != candidates[index-1] {
			t.Errorf("measure[%d] = %#v, want candidate %#v", index, measured[index], candidates[index-1])
		}
	}
}

func TestSolveOnePageFallsBackToCompleteCustomBaselineAtTheFloor(t *testing.T) {
	base := render.DefaultLayout().Normalized()
	base.FontStack = render.FontSerif
	base.SectionGapPX = 9
	base.ItemGapPX = 2
	base.MarginTopMM = 8
	base.MarginBottomMM = 7.5
	base.LineHeight = 1.25
	base = base.Normalized()

	var measured []render.Layout
	result, err := SolveOnePage(base, func(layout render.Layout) (int, error) {
		measured = append(measured, layout)
		return 3, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	candidates := render.SmartOnePageCandidates(base)
	if len(measured) != len(candidates)+1 {
		t.Fatalf("measure count = %d, want baseline plus %d candidates", len(measured), len(candidates))
	}
	if measured[0] != base {
		t.Fatalf("first measured layout = %#v, want base %#v", measured[0], base)
	}
	for index, candidate := range candidates {
		if measured[index+1] != candidate {
			t.Errorf("measure[%d] = %#v, want candidate %#v", index+1, measured[index+1], candidate)
		}
	}
	if result.Layout != base || result.NumPages != 3 || !result.Overflowed ||
		result.Notice != ReadabilityLimitNotice {
		t.Fatalf("result = %#v, want complete baseline fallback with the readability notice", result)
	}
}
