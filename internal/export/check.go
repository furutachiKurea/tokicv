package export

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tokicv/internal/dialect"
	"tokicv/internal/library"
	"tokicv/internal/render"
)

// CheckOptions configures an exact Page Fit check.
type CheckOptions struct {
	Input      string
	Timeout    time.Duration
	Layout     render.Layout
	LayoutSet  bool
	Store      *library.Store
	FitOnePage bool
}

// CheckResult is the exact Page Fit and Resume Dialect diagnostics.
type CheckResult struct {
	NumPages       int                  `json:"numPages"`
	Overflowed     bool                 `json:"overflowed"`
	AppliedProfile render.Layout        `json:"appliedProfile"`
	Diagnostics    []dialect.Diagnostic `json:"diagnostics"`
	Notice         string               `json:"notice,omitempty"`
}

// Check renders a Resume Document through the print pipeline and reports its
// exact Page Fit, applied Layout Profile, and Resume Dialect diagnostics.
func Check(opts CheckOptions) (CheckResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	target, err := resolveTarget(opts.Store, opts.Input)
	if err != nil {
		return CheckResult{}, fmt.Errorf("读取 Resume Document %q: %w", opts.Input, err)
	}
	source, err := target.Read()
	if err != nil {
		return CheckResult{}, err
	}
	analysis := dialect.Analyze(string(source))
	layout, err := effectiveLayout(opts.Store, target, opts.Layout, opts.LayoutSet)
	if err != nil {
		return CheckResult{}, err
	}
	html := render.RenderWithLayout(analysis.Document, layout)

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	var (
		fit          OnePageFit
		numPages     int
		operationErr error
	)
	_, err = withOffscreenPage(ctx, html, opts.Timeout, func(ctx context.Context, page Page) (renderOutput, error) {
		if opts.FitOnePage {
			fit, operationErr = solveOnePageOnPage(ctx, page, layout)
			if operationErr == nil {
				numPages = fit.NumPages
			}
			return renderOutput{}, operationErr
		}
		numPages, operationErr = measureCurrentPage(ctx, page)
		return renderOutput{}, operationErr
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errTimeout) {
			return CheckResult{}, timeoutError(opts.Timeout)
		}
		return CheckResult{}, err
	}
	if ctx.Err() != nil {
		return CheckResult{}, timeoutError(opts.Timeout)
	}
	diagnostics := analysis.Diagnostics
	if diagnostics == nil {
		diagnostics = make([]dialect.Diagnostic, 0)
	}
	appliedProfile := layout
	notice := ""
	if opts.FitOnePage {
		appliedProfile = fit.Layout
		notice = fit.Notice
	}
	return CheckResult{
		NumPages:       numPages,
		Overflowed:     numPages > 1,
		AppliedProfile: appliedProfile,
		Diagnostics:    diagnostics,
		Notice:         notice,
	}, nil
}
