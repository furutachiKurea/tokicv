//go:build linux

package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tokicv/internal/dialect"
	"tokicv/internal/export"
	"tokicv/internal/render"
)

const desktopParityMarker = "tokicv-desktop-parity="

type desktopParityReport struct {
	Error       string  `json:"error,omitempty"`
	DOMPages    int     `json:"domPages"`
	DOMWidth    float64 `json:"domWidth"`
	DOMHeight   float64 `json:"domHeight"`
	PDFPages    int     `json:"pdfPages"`
	PDFWidth    float64 `json:"pdfWidth"`
	PDFHeight   float64 `json:"pdfHeight"`
	PNGPages    int     `json:"pngPages"`
	PNGWidth    int     `json:"pngWidth"`
	PNGHeight   int     `json:"pngHeight"`
	PDFBytes    int     `json:"pdfBytes"`
	PNGBytes    int     `json:"pngBytes"`
	PreviewHTML int     `json:"previewHTMLBytes"`
}

func TestDesktopLocalePreviewPDFAndPNGParityUnderHeadlessLinux(t *testing.T) {
	if !cgoAvailable {
		t.Skip("WebKitGTK parity probe requires cgo")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	runtimeDir, waylandDisplay := startHeadlessMutter(t)

	dataDir := t.TempDir()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(environmentWithout(
		"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
	),
		"TOKICV_DESKTOP_PARITY_PROBE=1",
		"TOKICV_DESKTOP_PARITY_DATA="+dataDir,
		"XDG_CONFIG_HOME="+filepath.Join(dataDir, "config"),
		"XDG_DATA_HOME="+filepath.Join(dataDir, "data"),
		"XDG_CACHE_HOME="+filepath.Join(dataDir, "cache"),
		"XDG_RUNTIME_DIR="+runtimeDir,
		"WAYLAND_DISPLAY="+waylandDisplay,
		"GDK_BACKEND=wayland",
		"DISPLAY=",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("desktop parity probe timed out: %v; stderr: %s", ctx.Err(), stderr.String())
	}
	if err != nil {
		t.Fatalf("desktop parity probe failed: %v; stderr: %s", err, stderr.String())
	}

	var raw string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if value, ok := strings.CutPrefix(line, desktopParityMarker); ok {
			raw = value
		}
	}
	if raw == "" {
		t.Fatalf("no desktop parity report; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	}
	var report desktopParityReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode desktop parity report: %v; %s", err, raw)
	}
	if report.Error != "" {
		t.Fatalf("desktop parity probe: %s", report.Error)
	}
	t.Logf("desktop parity: %s", raw)

	if report.DOMPages < 2 || report.PDFPages != report.DOMPages || report.PNGPages != report.PDFPages {
		t.Fatalf("page parity: DOM %d, PDF %d, PNG %d", report.DOMPages, report.PDFPages, report.PNGPages)
	}
	domAspect := report.DOMWidth / report.DOMHeight
	pdfAspect := report.PDFWidth / report.PDFHeight
	pngAspect := float64(report.PNGWidth) / float64(report.PNGHeight)
	if math.Abs(domAspect-pdfAspect) > 0.002 || math.Abs(pngAspect-pdfAspect) > 0.002 {
		t.Fatalf("A4 aspect parity: DOM %.5f, PDF %.5f, PNG %.5f", domAspect, pdfAspect, pngAspect)
	}
	if math.Abs(report.PDFWidth-595) > 2 || math.Abs(report.PDFHeight-842) > 2 {
		t.Fatalf("PDF Sheet = %.2fx%.2f pt, want A4", report.PDFWidth, report.PDFHeight)
	}
	wantPNGWidth := int(math.Round(report.PDFWidth * 2))
	wantPNGHeight := int(math.Round(report.PDFHeight * 2))
	if absInt(report.PNGWidth-wantPNGWidth) > 2 || absInt(report.PNGHeight-wantPNGHeight) > 2 {
		t.Fatalf("PNG Sheet = %dx%d, want %.0fx%.0f",
			report.PNGWidth, report.PNGHeight, report.PDFWidth*2, report.PDFHeight*2)
	}
	if report.PDFBytes == 0 || report.PNGBytes == 0 || report.PreviewHTML == 0 {
		t.Fatalf("empty artifact report: %+v", report)
	}
}

func runDesktopParityProbe() int {
	if dir := os.Getenv("TOKICV_DESKTOP_PARITY_DATA"); dir != "" {
		userData := filepath.Join(dir, "user-data")
		cache := filepath.Join(dir, "cache")
		_ = os.MkdirAll(userData, 0o755)
		_ = os.MkdirAll(cache, 0o755)
		mygo.App.SetPath(mygo.PathUserData, userData)
		mygo.App.SetPath(mygo.PathCache, cache)
	}
	mygo.App.SetName("tokicv-desktop-parity")

	done := make(chan desktopParityReport, 1)
	mygo.App.WhenReady(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			done <- inspectDesktopParity(ctx)
			mygo.App.Quit()
		}()
	})
	runErr := mygo.App.Run()
	var report desktopParityReport
	select {
	case report = <-done:
	case <-time.After(10 * time.Second):
		report.Error = "desktop parity probe did not finish"
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal desktop parity report: %v\n", err)
		return 1
	}
	fmt.Printf("%s%s\n", desktopParityMarker, data)
	return 0
}

func inspectDesktopParity(ctx context.Context) desktopParityReport {
	source := parityResume()
	doc := dialect.Parse(source)
	layout := render.DefaultLayout()
	previewHTML := render.PreviewHTML(doc, layout)

	anchor := mygo.NewWindow(mygo.WindowOptions{
		Hidden:         true,
		Width:          1,
		Height:         1,
		UseContentSize: true,
		Content:        ui.View(func(*ui.Context) {}),
	})
	defer anchor.Close()

	pages, width, height, err := desktopPreviewGeometry(ctx, previewHTML)
	if err != nil {
		return desktopParityReport{Error: "DOM preview: " + err.Error()}
	}
	artifacts, err := export.RenderPDFAndPNG(
		ctx,
		hiddenPageRunner{},
		render.RenderWithLayout(doc, layout),
	)
	if err != nil {
		return desktopParityReport{Error: "desktop export: " + err.Error()}
	}
	pdfPages, pdfWidth, pdfHeight, err := desktopPDFGeometry(artifacts.PDF)
	if err != nil {
		return desktopParityReport{Error: "PDF: " + err.Error()}
	}
	if len(artifacts.PNGs) == 0 {
		return desktopParityReport{Error: "PNG: no pages"}
	}
	config, err := png.DecodeConfig(bytes.NewReader(artifacts.PNGs[0]))
	if err != nil {
		return desktopParityReport{Error: "PNG: " + err.Error()}
	}
	pngBytes := 0
	for _, page := range artifacts.PNGs {
		pngBytes += len(page)
	}
	return desktopParityReport{
		DOMPages:    pages,
		DOMWidth:    width,
		DOMHeight:   height,
		PDFPages:    pdfPages,
		PDFWidth:    pdfWidth,
		PDFHeight:   pdfHeight,
		PNGPages:    len(artifacts.PNGs),
		PNGWidth:    config.Width,
		PNGHeight:   config.Height,
		PDFBytes:    len(artifacts.PDF),
		PNGBytes:    pngBytes,
		PreviewHTML: len(previewHTML),
	}
}

type desktopPreviewState struct {
	mu  sync.Mutex
	web *mygo.WebView
}

func (s *desktopPreviewState) set(web *mygo.WebView) {
	s.mu.Lock()
	s.web = web
	s.mu.Unlock()
}

func (s *desktopPreviewState) get() *mygo.WebView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.web
}

func desktopPreviewGeometry(ctx context.Context, html string) (int, float64, float64, error) {
	state := &desktopPreviewState{}
	win := mygo.NewWindow(mygo.WindowOptions{
		Hidden:          true,
		Width:           1280,
		Height:          800,
		UseContentSize:  true,
		BackgroundColor: "#E0DFDB",
		Content: ui.View(func(c *ui.Context) {
			if web := state.get(); web != nil {
				ui.WebView(c, web).Fill()
			}
		}),
	})
	defer win.Close()
	web, err := win.NewWebView(mygo.WebViewOptions{
		Page: mygo.PageOptions{
			BackgroundThrottling: mygo.ThrottlingOff,
			Untrusted:            true,
		},
		BackgroundColor: "#E0DFDB",
	})
	if err != nil {
		return 0, 0, 0, err
	}
	state.set(web)
	win.Invalidate()
	web.Page().LoadHTML(html, "")

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		value, err := web.Page().EvalContext(ctx, `(() => {
  const count = Number(document.documentElement.dataset.tokicvPages || 0);
  const slot = document.querySelector(".sheet-slot");
  if (!slot || count < 1) return null;
  const rect = slot.getBoundingClientRect();
  return {pages: count, width: rect.width, height: rect.height};
})()`)
		if err == nil {
			if object, ok := value.(map[string]any); ok {
				pages := int(object["pages"].(float64))
				width := object["width"].(float64)
				height := object["height"].(float64)
				if pages > 0 && width > 0 && height > 0 {
					return pages, width, height, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0, 0, 0, ctx.Err()
		case <-ticker.C:
		}
	}
}

func desktopPDFGeometry(pdf []byte) (int, float64, float64, error) {
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return 0, 0, 0, fmt.Errorf("invalid PDF header")
	}
	pages := len(regexp.MustCompile(`/Type\s*/Page\b`).FindAll(pdf, -1))
	if pages < 1 {
		return 0, 0, 0, fmt.Errorf("no PDF pages")
	}
	match := regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([\d.]+)\s+([\d.]+)\s*\]`).FindSubmatch(pdf)
	if len(match) != 3 {
		return 0, 0, 0, fmt.Errorf("no PDF MediaBox")
	}
	width, err := strconv.ParseFloat(string(match[1]), 64)
	if err != nil {
		return 0, 0, 0, err
	}
	height, err := strconv.ParseFloat(string(match[2]), 64)
	if err != nil {
		return 0, 0, 0, err
	}
	return pages, width, height, nil
}

func parityResume() string {
	var source strings.Builder
	source.WriteString("# 示例文档\n\n")
	for section := 1; section <= 10; section++ {
		fmt.Fprintf(&source, "## 第 %d 部分\n\n", section)
		fmt.Fprintf(&source,
			"这是第 %d 部分的说明文字，包含 mixed English text 与 0123456789，用于填充分页内容。\n\n", section)
		for item := 1; item <= 6; item++ {
			fmt.Fprintf(&source, "- 第 %d 部分的第 %d 条经历内容\n", section, item)
		}
		source.WriteString("\n")
	}
	return source.String()
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
