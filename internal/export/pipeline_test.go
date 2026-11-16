package export_test

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo"

	"tokicv/internal/export"
	"tokicv/internal/render"
)

type recordingRunner struct {
	html  string
	calls int
}

func (r *recordingRunner) WithPage(ctx context.Context, html string, fn func(context.Context, export.Page) error) error {
	r.calls++
	r.html = html
	return fn(ctx, &stubPage{})
}

type stubPage struct{}

func (p *stubPage) LoadHTML(string, string) {}

func (p *stubPage) EvalContext(_ context.Context, script string) (any, error) {
	switch {
	case strings.Contains(script, "document.fonts.ready"):
		return true, nil
	case strings.Contains(script, `querySelectorAll("a[href]")`):
		return map[string]any{"text": "正文", "lines": []any{}, "links": []any{}}, nil
	case strings.Contains(script, "numPages"):
		return float64(1), nil
	case strings.Contains(script, "tokicv-fit-measure"):
		return map[string]any{"pages": float64(1)}, nil
	case strings.Contains(script, "toDataURL"):
		return base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nstub")), nil
	default:
		return true, nil
	}
}

func (p *stubPage) PrintToPDF(mygo.PDFOptions) ([]byte, error) {
	return []byte("%PDF-1.7\nstub"), nil
}

type languagePage struct {
	stubPage
	language    string
	languageSet bool
	err         error
}

func (p *languagePage) PrintToPDF(mygo.PDFOptions) ([]byte, error) {
	p.language, p.languageSet = os.LookupEnv("LANGUAGE")
	if p.err != nil {
		return nil, p.err
	}
	return []byte("%PDF-1.7\nstub"), nil
}

type singlePageRunner struct {
	page export.Page
}

func (r singlePageRunner) WithPage(
	ctx context.Context,
	_ string,
	fn func(context.Context, export.Page) error,
) error {
	return fn(ctx, r.page)
}

func TestRenderPDFUsesTheInjectedPageRunner(t *testing.T) {
	runner := &recordingRunner{}
	pdf, err := export.RenderPDF(context.Background(), runner, "<html>resume</html>")
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 || runner.html != "<html>resume</html>" {
		t.Fatalf("runner calls = %d, html = %q", runner.calls, runner.html)
	}
	if !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatalf("PDF = %q", pdf)
	}
}

func TestRenderPDFTemporarilyUsesCLanguageAndRestoresIt(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the Linux print backend localizes GTK's virtual printer")
	}
	const original = "zh_CN:en"
	t.Setenv("LANGUAGE", original)
	printErr := errors.New("print failed")

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "failure", err: printErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := &languagePage{err: tc.err}
			_, err := export.RenderPDF(
				context.Background(),
				singlePageRunner{page: page},
				"<html>resume</html>",
			)
			if tc.err != nil {
				if !errors.Is(err, printErr) {
					t.Fatalf("RenderPDF error = %v, want %v", err, printErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !page.languageSet || page.language != "C" {
				t.Fatalf("LANGUAGE during PrintToPDF = %q (set=%t), want C",
					page.language, page.languageSet)
			}
			if got, set := os.LookupEnv("LANGUAGE"); !set || got != original {
				t.Fatalf("LANGUAGE after RenderPDF = %q (set=%t), want %q",
					got, set, original)
			}
		})
	}
}

func TestRenderPDFAndPNGUsesOnePageSessionForPrintingAndRasterizing(t *testing.T) {
	runner := &recordingRunner{}
	artifacts, err := export.RenderPDFAndPNG(context.Background(), runner, "<html>resume</html>")
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want one page session", runner.calls)
	}
	if artifacts.NumPages != 1 || len(artifacts.PNGs) != 1 {
		t.Fatalf("artifacts = %d pages, %d PNGs", artifacts.NumPages, len(artifacts.PNGs))
	}
	if !strings.HasPrefix(string(artifacts.PDF), "%PDF-") {
		t.Fatalf("PDF = %q", artifacts.PDF)
	}
	if !strings.HasPrefix(string(artifacts.PNGs[0]), "\x89PNG\r\n\x1a\n") {
		t.Fatalf("PNG = %q", artifacts.PNGs[0])
	}
}

func TestFitOnePageUsesTheInjectedPageRunner(t *testing.T) {
	runner := &recordingRunner{}
	fit, err := export.FitOnePage(
		context.Background(),
		runner,
		"<html>resume</html>",
		render.DefaultLayout(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want one page session", runner.calls)
	}
	if fit.NumPages != 1 || fit.Overflowed || fit.Layout != render.DefaultLayout().Normalized() {
		t.Fatalf("fit = %#v", fit)
	}
}

type filteringPage struct {
	prints       int
	measurements int
	fitAfter     int
	fallback     bool
	layout       render.Layout
}

func (p *filteringPage) LoadHTML(string, string) {}

func (p *filteringPage) EvalContext(_ context.Context, script string) (any, error) {
	switch {
	case strings.Contains(script, "tokicv-layout"):
		p.layout = layoutFromScript(script)
		return true, nil
	case strings.Contains(script, "tokicv-fit-measure"):
		p.measurements++
		pages := 2.0
		if p.fitAfter > 0 && p.measurements >= p.fitAfter {
			pages = 1
		}
		return map[string]any{"pages": pages}, nil
	case strings.Contains(script, "document.fonts.ready"):
		return true, nil
	case strings.Contains(script, "numPages"):
		if p.fallback {
			return float64(2), nil
		}
		return float64(1), nil
	default:
		return true, nil
	}
}

func (p *filteringPage) PrintToPDF(mygo.PDFOptions) ([]byte, error) {
	p.prints++
	return []byte("%PDF-1.7\nstub"), nil
}

func layoutFromScript(script string) render.Layout {
	layout := render.DefaultLayout()
	if strings.Contains(script, "--line-height: 1.3;") {
		layout.LineHeight = 1.30
	}
	if strings.Contains(script, "--line-height: 1.2;") {
		layout.LineHeight = render.MinLineHeight
	}
	if strings.Contains(script, "--margin-top: 7.5mm") {
		layout.MarginTopMM = 7.5
		layout.MarginBottomMM = 7.5
	}
	if strings.Contains(script, "--section-gap: 8px") {
		layout.SectionGapPX = 8
	}
	if strings.Contains(script, "--item-gap: 3px") {
		layout.ItemGapPX = 3
	}
	if strings.Contains(script, "--section-gap: 5px") {
		layout.SectionGapPX = 5
	}
	if strings.Contains(script, "--item-gap: 1.5px") {
		layout.ItemGapPX = 1.5
	}
	return layout.Normalized()
}

func TestFitOnePageUsesDOMPrefilterAndPrintsOnlyTheSelectedCandidate(t *testing.T) {
	page := &filteringPage{fitAfter: 5}
	runner := &fixedPageRunner{page: page}
	base := render.DefaultLayout()

	fit, err := export.FitOnePage(context.Background(), runner, "<html>resume</html>", base)
	if err != nil {
		t.Fatal(err)
	}
	if page.prints != 1 {
		t.Fatalf("PrintToPDF calls = %d, want the selected candidate confirmed once", page.prints)
	}
	if page.measurements < 4 {
		t.Fatalf("DOM measurements = %d, want candidates prefiltered before printing", page.measurements)
	}
	want := render.SmartOnePageCandidates(base.Normalized())[3]
	if fit.Layout != want || fit.NumPages != 1 || fit.Overflowed {
		t.Fatalf("fit = %#v, want prefiltered candidate %#v", fit, want)
	}
}

func TestFitOnePagePrintsTheBaselineOnceWhenEveryCandidateOverflows(t *testing.T) {
	page := &filteringPage{fallback: true}
	runner := &fixedPageRunner{page: page}
	base := render.DefaultLayout().Normalized()

	fit, err := export.FitOnePage(context.Background(), runner, "<html>resume</html>", base)
	if err != nil {
		t.Fatal(err)
	}
	if page.prints != 1 {
		t.Fatalf("PrintToPDF calls = %d, want only the baseline fallback confirmation", page.prints)
	}
	if fit.Layout != base || fit.NumPages != 2 || !fit.Overflowed || fit.Notice != export.ReadabilityLimitNotice {
		t.Fatalf("fit = %#v, want the confirmed overflowing baseline", fit)
	}
}

func TestFitOnePageUsesPrintedBaselineWhenDOMPrefilterOverestimates(t *testing.T) {
	page := &filteringPage{}
	runner := &fixedPageRunner{page: page}
	base := render.DefaultLayout().Normalized()

	fit, err := export.FitOnePage(context.Background(), runner, "<html>resume</html>", base)
	if err != nil {
		t.Fatal(err)
	}
	if page.prints != 1 {
		t.Fatalf("PrintToPDF calls = %d, want the baseline confirmed once", page.prints)
	}
	if fit.Layout != base || fit.NumPages != 1 || fit.Overflowed || fit.Notice != "" {
		t.Fatalf("fit = %#v, want the printed one-page baseline", fit)
	}
}

type fixedPageRunner struct {
	page *filteringPage
}

func (r *fixedPageRunner) WithPage(ctx context.Context, _ string, fn func(context.Context, export.Page) error) error {
	return fn(ctx, r.page)
}

type failingRunner struct{}

func (failingRunner) WithPage(context.Context, string, func(context.Context, export.Page) error) error {
	return errors.New("runner failed")
}

func TestRenderPDFReturnsTheRunnerError(t *testing.T) {
	if _, err := export.RenderPDF(context.Background(), failingRunner{}, "<html></html>"); err == nil {
		t.Fatal("RenderPDF accepted a failing page runner")
	}
}
