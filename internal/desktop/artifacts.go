package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tokicv/internal/export"
	"tokicv/internal/render"
)

// ArtifactKind identifies one desktop export action.
type ArtifactKind string

// Desktop Artifact kinds.
const (
	ArtifactPDF      ArtifactKind = "pdf"
	ArtifactPNG      ArtifactKind = "png"
	ArtifactMarkdown ArtifactKind = "markdown"
)

const artifactTimeout = 30 * time.Second

// ArtifactPipeline is the shared print, raster, and fit pipeline used by the
// desktop workspace.
type ArtifactPipeline interface {
	FitOnePage(ctx context.Context, html string, base render.Layout) (export.OnePageFit, error)
	WritePDF(ctx context.Context, html, path string) error
	WritePNG(ctx context.Context, html, path string) error
	WriteMarkdown(ctx context.Context, source []byte, path string) error
}

type desktopArtifactPipeline struct {
	runner export.PageRunner
}

func newDesktopArtifactPipeline() ArtifactPipeline {
	return desktopArtifactPipeline{runner: hiddenPageRunner{}}
}

func (p desktopArtifactPipeline) FitOnePage(
	ctx context.Context,
	html string,
	base render.Layout,
) (export.OnePageFit, error) {
	return export.FitOnePage(ctx, p.runner, html, base)
}

func (p desktopArtifactPipeline) WritePDF(ctx context.Context, html, path string) error {
	pdf, err := export.RenderPDF(ctx, p.runner, html)
	if err != nil {
		return err
	}
	return export.WritePDFArtifact(path, pdf)
}

func (p desktopArtifactPipeline) WritePNG(ctx context.Context, html, path string) error {
	artifacts, err := export.RenderPDFAndPNG(ctx, p.runner, html)
	if err != nil {
		return err
	}
	return export.WritePNGArtifact(path, artifacts.PNGs)
}

func (p desktopArtifactPipeline) WriteMarkdown(_ context.Context, source []byte, path string) error {
	return export.WriteMarkdownArtifact(path, source)
}

type hiddenPageRunner struct{}

var _ export.PageRunner = hiddenPageRunner{}

type hiddenWebView struct {
	mu  sync.Mutex
	web *mygo.WebView
}

func (s *hiddenWebView) set(web *mygo.WebView) {
	s.mu.Lock()
	s.web = web
	s.mu.Unlock()
}

func (s *hiddenWebView) get() *mygo.WebView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.web
}

// WithPage creates and closes a hidden print window inside the already-running
// desktop application. It never starts or quits the application event loop.
func (hiddenPageRunner) WithPage(
	ctx context.Context,
	html string,
	operation func(context.Context, export.Page) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	state := &hiddenWebView{}
	win := mygo.NewWindow(mygo.WindowOptions{
		Hidden:          true,
		Width:           794,
		Height:          1122,
		UseContentSize:  true,
		BackgroundColor: "#FFFFFF",
		Content: ui.View(func(c *ui.Context) {
			if web := state.get(); web != nil {
				ui.WebView(c, web).Fill()
			}
		}),
	})
	defer win.Close()

	web, err := win.NewWebView(mygo.WebViewOptions{
		Page:            mygo.PageOptions{BackgroundThrottling: mygo.ThrottlingOff},
		BackgroundColor: "#FFFFFF",
	})
	if err != nil {
		return fmt.Errorf("创建临时打印页面: %w", err)
	}
	state.set(web)
	win.Invalidate()
	// LoadHTML starts a navigation. Evaluating against the old document now
	// loses the completion handler and stalls until WebKit's call timeout.
	domReady := make(chan struct{}, 1)
	offDOMReady := web.Page().OnDOMReady(func() {
		select {
		case domReady <- struct{}{}:
		default:
		}
	})
	defer offDOMReady()
	web.Page().LoadHTML(html, "")
	select {
	case <-domReady:
	case <-ctx.Done():
		return ctx.Err()
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			win.Close()
		case <-done:
		}
	}()
	err = operation(ctx, web.Page())
	close(done)
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	return err
}

func (a *App) canExport() bool {
	return a.artifacts != nil && a.hasDocument()
}

func (a *App) exportArtifact(kind ArtifactKind) {
	if !a.canExport() {
		return
	}
	choose := a.exportPaths[kind]
	if choose == nil {
		return
	}
	a.exportMenuOpen = false
	defaultName, err := a.artifactName(kind)
	if err != nil {
		a.setArtifactError("导出失败")
		return
	}
	path, err := choose(defaultName)
	if err != nil {
		a.setArtifactError("导出失败")
		return
	}
	if path == "" {
		return
	}
	a.setArtifactError("")

	var (
		html   string
		source []byte
		layout render.Layout
	)
	if kind == ArtifactMarkdown {
		source = a.exactSource()
	} else {
		a.ensureAnalysis()
		layout = a.layout
		html = render.RenderWithLayout(a.analysis.Document, layout)
	}
	document := a.document
	a.startArtifactWork(func(ctx context.Context) error {
		switch kind {
		case ArtifactPDF:
			return a.artifacts.WritePDF(ctx, html, path)
		case ArtifactPNG:
			return a.artifacts.WritePNG(ctx, html, path)
		case ArtifactMarkdown:
			return a.artifacts.WriteMarkdown(ctx, source, path)
		default:
			return errors.New("未知导出类型")
		}
	}, func(err error) {
		if a.closed || document != a.document {
			return
		}
		if err != nil {
			logDesktopError("export-"+string(kind), err)
			a.setArtifactError("导出失败")
			return
		}
		a.setArtifactError("")
	})
}

func (a *App) setArtifactError(message string) {
	if a.artifactError == message {
		return
	}
	a.artifactError = message
	a.invalidate()
}

func (a *App) artifactName(kind ArtifactKind) (string, error) {
	name := a.markdownExportName()
	name = stringsTrimSuffixFold(name, ".md")
	switch kind {
	case ArtifactPDF:
		return name + ".pdf", nil
	case ArtifactPNG:
		return name + ".png", nil
	case ArtifactMarkdown:
		return name + ".md", nil
	default:
		return "", fmt.Errorf("未知导出类型 %q", kind)
	}
}

func stringsTrimSuffixFold(value, suffix string) string {
	if len(value) >= len(suffix) && strings.EqualFold(value[len(value)-len(suffix):], suffix) {
		return value[:len(value)-len(suffix)]
	}
	return value
}
