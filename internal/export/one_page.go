package export

import (
	"errors"
	"io"
	"time"

	"tokicv/internal/render"
)

// ReadabilityLimitNotice reports why Smart One-Page kept the multi-page
// Layout Profile.
const ReadabilityLimitNotice = "无法压缩成一页"

// OnePageFit is the effective Layout Profile and Page Fit after Smart
// One-Page.
type OnePageFit struct {
	Layout     render.Layout
	NumPages   int
	Overflowed bool
	Notice     string
	// Duration covers the DOM prefilter and print confirmation after the
	// WebView page is ready.
	Duration time.Duration
}

// SolveOnePage applies deterministic Smart One-Page candidates through
// measure, stopping at the first one-page result. When every candidate
// overflows, it returns the complete active baseline.
func SolveOnePage(base render.Layout, measure func(render.Layout) (int, error)) (OnePageFit, error) {
	if measure == nil {
		return OnePageFit{}, errors.New("缺少 Page Fit 测量器")
	}
	base = base.Normalized()
	basePages, err := measure(base)
	if err != nil {
		return OnePageFit{}, err
	}
	if basePages <= 1 {
		return OnePageFit{Layout: base, NumPages: basePages}, nil
	}

	for _, candidate := range render.SmartOnePageCandidates(base) {
		numPages, err := measure(candidate)
		if err != nil {
			return OnePageFit{}, err
		}
		if numPages <= 1 {
			return OnePageFit{Layout: candidate, NumPages: numPages}, nil
		}
	}
	return OnePageFit{
		Layout:     base,
		NumPages:   basePages,
		Overflowed: true,
		Notice:     ReadabilityLimitNotice,
	}, nil
}

func writeOnePageNotice(writer io.Writer, notice string) error {
	if writer == nil || notice == "" {
		return nil
	}
	_, err := io.WriteString(writer, notice+"\n")
	return err
}
