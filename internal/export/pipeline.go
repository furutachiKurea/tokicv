package export

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/egoist/mygo"

	"tokicv/internal/render"
)

// Page is the subset of a WebView page used by the shared export pipeline.
type Page interface {
	LoadHTML(html, baseURL string)
	EvalContext(ctx context.Context, code string) (any, error)
	PrintToPDF(options mygo.PDFOptions) ([]byte, error)
}

// PageRunner opens one page session and closes it when operation returns.
// The desktop implementation creates a hidden temporary window in the
// already-running GUI process; the CLI implementation owns the application
// event loop.
type PageRunner interface {
	WithPage(ctx context.Context, html string, operation func(context.Context, Page) error) error
}

// Artifacts is the PDF and per-Sheet PNG output of one page session.
type Artifacts struct {
	PDF      []byte
	PNGs     [][]byte
	NumPages int
}

// RenderPDF prints html through the supplied page runner.
func RenderPDF(ctx context.Context, runner PageRunner, html string) ([]byte, error) {
	output, err := runPageOperation(ctx, runner, html, func(ctx context.Context, page Page) (renderOutput, error) {
		return exportWebView(ctx, page, nil)
	})
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return output.PDF, nil
}

// RenderPDFAndPNG prints html once and rasterizes the resulting PDF through
// the bundled pdfjs-dist assets.
func RenderPDFAndPNG(ctx context.Context, runner PageRunner, html string) (Artifacts, error) {
	output, err := runPageOperation(ctx, runner, html, func(ctx context.Context, page Page) (renderOutput, error) {
		return exportWebView(ctx, page, rasterizePDF)
	})
	if err != nil {
		return Artifacts{}, err
	}
	if ctx.Err() != nil {
		return Artifacts{}, ctx.Err()
	}
	return Artifacts{PDF: output.PDF, PNGs: output.PNGs, NumPages: output.NumPages}, nil
}

// FitOnePage runs Smart One-Page and exact printed Page Fit verification
// through the supplied page runner.
func FitOnePage(ctx context.Context, runner PageRunner, html string, base render.Layout) (OnePageFit, error) {
	if runner == nil {
		return OnePageFit{}, errors.New("缺少页面运行器")
	}
	var fit OnePageFit
	err := runner.WithPage(ctx, html, func(ctx context.Context, page Page) error {
		var err error
		fit, err = solveOnePageOnPage(ctx, page, base)
		return err
	})
	if err != nil {
		return OnePageFit{}, err
	}
	if ctx.Err() != nil {
		return OnePageFit{}, ctx.Err()
	}
	return fit, nil
}

func runPageOperation(
	ctx context.Context,
	runner PageRunner,
	html string,
	operation pageOperation,
) (renderOutput, error) {
	if runner == nil {
		return renderOutput{}, errors.New("缺少页面运行器")
	}
	var output renderOutput
	err := runner.WithPage(ctx, html, func(ctx context.Context, page Page) error {
		var operationErr error
		output, operationErr = operation(ctx, page)
		return operationErr
	})
	return output, err
}

// WritePDFArtifact atomically writes a printed PDF Artifact.
func WritePDFArtifact(path string, pdf []byte) error {
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return errors.New("PDF Artifact 不是有效的 PDF")
	}
	if err := writeAtomic(path, pdf); err != nil {
		return fmt.Errorf("写入 PDF %q: %w", path, err)
	}
	return nil
}

// WritePNGArtifact atomically writes one or more per-Sheet PNG Artifacts.
func WritePNGArtifact(path string, pages [][]byte) error {
	if err := writePNGArtifacts(path, pages); err != nil {
		return fmt.Errorf("写入 PNG %q: %w", path, err)
	}
	return nil
}

// WriteMarkdownArtifact atomically writes the exact Resume Document bytes.
func WriteMarkdownArtifact(path string, source []byte) error {
	if err := writeAtomic(path, source); err != nil {
		return fmt.Errorf("写入 Markdown %q: %w", path, err)
	}
	return nil
}
