//go:build linux

package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tokicv/internal/dialect"
	"tokicv/internal/render"
)

const (
	previewPerfMarker = "tokicv-preview-perf="
	previewPerfSource = "testdata/preview-scroll.md"
)

type previewPerfSample struct {
	Frames    int     `json:"frames"`
	Seconds   float64 `json:"seconds"`
	P50MS     float64 `json:"p50ms"`
	P95MS     float64 `json:"p95ms"`
	MaxMS     float64 `json:"maxms"`
	Over25MS  int     `json:"over25ms"`
	ScrollY   float64 `json:"scrollY"`
	MaxScroll float64 `json:"maxScroll"`
}

type previewPerfReport struct {
	Error   string              `json:"error,omitempty"`
	Pages   int                 `json:"pages"`
	Samples []previewPerfSample `json:"samples"`
}

// TestPreviewWebKitScrollFrameBudget runs the embedded preview at 2x scale
// and keeps its scrolling inside a 60 Hz frame budget.
func TestPreviewWebKitScrollFrameBudget(t *testing.T) {
	if !cgoAvailable {
		t.Skip("WebKitGTK preview perf probe requires cgo")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	runtimeDir, waylandDisplay := startHeadlessMutterAt(t, "2880x1800")

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(environmentWithout(
		"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND",
		"GDK_SCALE", "GDK_DPI_SCALE",
	),
		"TOKICV_PREVIEW_PERF_PROBE=1",
		"TOKICV_PREVIEW_PERF_DATA="+t.TempDir(),
		"XDG_RUNTIME_DIR="+runtimeDir,
		"WAYLAND_DISPLAY="+waylandDisplay,
		"GDK_BACKEND=wayland",
		"GDK_SCALE=2",
		"GDK_DPI_SCALE=1",
		"DISPLAY=",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("preview perf probe timed out: %v; stderr: %s", ctx.Err(), stderr.String())
	}
	if err != nil {
		t.Fatalf("preview perf probe failed: %v; stderr: %s", err, stderr.String())
	}

	raw := ""
	for _, line := range strings.Split(stdout.String(), "\n") {
		if value, ok := strings.CutPrefix(line, previewPerfMarker); ok {
			raw = value
		}
	}
	if raw == "" {
		t.Fatalf("no preview perf result; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	}
	var report previewPerfReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode preview perf result: %v; result: %s", err, raw)
	}
	if report.Error != "" {
		if strings.Contains(report.Error, "unavailable") {
			t.Skipf("WebKitGTK preview is unavailable: %s", report.Error)
		}
		t.Fatalf("preview perf probe: %s", report.Error)
	}
	if report.Pages < 2 {
		t.Fatalf("preview pages = %d, want a multi-page fixture", report.Pages)
	}
	if len(report.Samples) != 3 {
		t.Fatalf("preview perf samples = %d, want 3", len(report.Samples))
	}
	for index, sample := range report.Samples {
		t.Logf("sample %d: frames=%d p50=%.1fms p95=%.1fms max=%.1fms over25=%d scroll=%.0f/%.0f",
			index+1, sample.Frames, sample.P50MS, sample.P95MS, sample.MaxMS,
			sample.Over25MS, sample.ScrollY, sample.MaxScroll)
	}

	sort.Slice(report.Samples, func(i, j int) bool {
		return report.Samples[i].P95MS < report.Samples[j].P95MS
	})
	median := report.Samples[len(report.Samples)/2]
	if median.Frames < 30 {
		t.Errorf("median sample has only %d frames; the scroll did not run", median.Frames)
	}
	if median.P95MS > 22 {
		t.Errorf("median p95 frame interval = %.1fms, want at most 22ms", median.P95MS)
	}
	if median.Over25MS > 3 {
		t.Errorf("median frames over 25ms = %d, want at most 3", median.Over25MS)
	}
	if median.MaxScroll <= 0 || median.ScrollY < median.MaxScroll*0.9 {
		t.Errorf("scroll reached %.0f of %.0f", median.ScrollY, median.MaxScroll)
	}
}

func runPreviewPerfProbe() int {
	if dir := os.Getenv("TOKICV_PREVIEW_PERF_DATA"); dir != "" {
		userData := filepath.Join(dir, "user-data")
		cache := filepath.Join(dir, "cache")
		_ = os.MkdirAll(userData, 0o755)
		_ = os.MkdirAll(cache, 0o755)
		mygo.App.SetPath(mygo.PathUserData, userData)
		mygo.App.SetPath(mygo.PathCache, cache)
	}
	mygo.App.SetName("tokicv-preview-perf")

	done := make(chan previewPerfReport, 1)
	mygo.App.WhenReady(func() {
		go func() {
			done <- inspectPreviewPerf()
			mygo.App.Quit()
		}()
	})

	runErr := mygo.App.Run()
	var report previewPerfReport
	select {
	case report = <-done:
	case <-time.After(5 * time.Second):
		report.Error = "the preview perf probe did not finish"
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal preview perf report: %v\n", err)
		return 1
	}
	fmt.Printf("%s%s\n", previewPerfMarker, data)
	return 0
}

func inspectPreviewPerf() previewPerfReport {
	if !mygo.WebViewAvailable() {
		return previewPerfReport{Error: "WebKitGTK WebView is unavailable"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	source, err := os.ReadFile(previewPerfSource)
	if err != nil {
		return previewPerfReport{Error: "read fixture: " + err.Error()}
	}
	frame := render.PreviewFrameFor(dialect.Parse(string(source)), render.DefaultLayout())

	var web *mygo.WebView
	window := mygo.NewWindow(mygo.WindowOptions{
		Title:           "tokicv preview perf",
		Frameless:       true,
		X:               0,
		Y:               0,
		Width:           1280,
		Height:          800,
		UseContentSize:  true,
		BackgroundColor: "#EDECE8",
		Content: ui.View(func(c *ui.Context) {
			ui.Row(c.Key("preview-perf-root")).Fill().Children(func() {
				ui.Box(c.Key("preview-perf-editor")).Width(500).FillHeight()
				ui.Box(c.Key("preview-perf-pane")).Grow(1).FillHeight().Children(func() {
					if web == nil {
						ui.Box(c.Key("preview-perf-empty")).Fill()
						return
					}
					ui.WebView(c.Key("preview-perf-sheet"), web).Grow(1)
				})
			})
		}),
	})
	defer window.Destroy()

	webView, err := window.NewWebView(mygo.WebViewOptions{
		Page: mygo.PageOptions{
			BackgroundThrottling: mygo.ThrottlingOff,
			Untrusted:            true,
		},
		BackgroundColor: "#E0DFDB",
	})
	if err != nil {
		return previewPerfReport{Error: "create preview WebView: " + err.Error()}
	}
	web = webView
	web.Page().LoadHTML(frame.HTML, "")
	window.Invalidate()

	var report previewPerfReport
	for {
		value, err := web.Page().EvalContext(ctx, `document.documentElement.dataset.tokicvPages || ""`)
		if err == nil {
			if pages, ok := value.(string); ok && pages != "" {
				report.Pages, _ = strconv.Atoi(pages)
				break
			}
		}
		select {
		case <-ctx.Done():
			return previewPerfReport{Error: "preview pagination: " + ctx.Err().Error()}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if _, err := web.Page().EvalContext(ctx, `(async () => {
  await document.fonts.ready;
  await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  return true;
})()`); err != nil {
		return previewPerfReport{Error: "wait for preview fonts: " + err.Error()}
	}
	time.Sleep(500 * time.Millisecond)

	if _, err := runPreviewPerfScroll(web); err != nil {
		return previewPerfReport{Error: "warm up preview scroll: " + err.Error()}
	}
	for index := 0; index < 3; index++ {
		sample, err := runPreviewPerfScroll(web)
		if err != nil {
			return previewPerfReport{Error: "measure preview scroll: " + err.Error()}
		}
		report.Samples = append(report.Samples, sample)
	}
	return report
}

func runPreviewPerfScroll(web *mygo.WebView) (previewPerfSample, error) {
	return mygo.EvalAs[previewPerfSample](web.Page(), `(async () => {
  window.scrollTo(0, 0);
  await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  const duration = 2500;
  const maxScroll = Math.max(1, document.documentElement.scrollHeight - innerHeight);
  const start = performance.now();
  const intervals = [];
  let last = start;
  let frames = 0;
  await new Promise(resolve => {
    function step(now) {
      const progress = Math.min((now - start) / duration, 1);
      window.scrollTo(0, maxScroll * (0.5 - 0.5 * Math.cos(progress * Math.PI)));
      document.documentElement.getBoundingClientRect();
      if (frames > 0) intervals.push(now - last);
      last = now;
      frames++;
      if (progress < 1) requestAnimationFrame(step); else resolve();
    }
    requestAnimationFrame(step);
  });
  const seconds = (performance.now() - start) / 1000;
  const sorted = intervals.slice().sort((a, b) => a - b);
  const percentile = (values, p) => values.length
    ? values[Math.min(values.length - 1, Math.floor(values.length * p))]
    : 0;
  return {
    frames,
    seconds,
    p50ms: percentile(sorted, 0.5),
    p95ms: percentile(sorted, 0.95),
    maxms: sorted.length ? sorted[sorted.length - 1] : 0,
    over25ms: intervals.filter(interval => interval > 25).length,
    scrollY,
    maxScroll
  };
})()`)
}
