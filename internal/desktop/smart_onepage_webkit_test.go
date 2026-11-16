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

const smartOnePageProbeMarker = "tokicv-smart-one-page="
const smartOnePageRealWindowProbeMarker = "tokicv-smart-one-page-real-window="

var acceptanceResumePath = filepath.Join("..", "export", "testdata", "compact-120.md")

type smartOnePageProbeReport struct {
	Error              string  `json:"error,omitempty"`
	DurationMS         int64   `json:"durationMS"`
	SolveDurationMS    int64   `json:"solveDurationMS"`
	RefitDurationMS    int64   `json:"refitDurationMS"`
	RefitSolveMS       int64   `json:"refitSolveMS"`
	Pages              int     `json:"pages"`
	Smart              bool    `json:"smart"`
	SectionGap         float64 `json:"sectionGap"`
	ItemGap            float64 `json:"itemGap"`
	MarginTop          float64 `json:"marginTop"`
	MarginBottom       float64 `json:"marginBottom"`
	LineHeight         float64 `json:"lineHeight"`
	Adjusted           bool    `json:"adjusted"`
	AdjustedSectionGap float64 `json:"adjustedSectionGap"`
	SmartAfterAdjust   bool    `json:"smartAfterAdjust"`
}

type smartOnePageRealWindowReport struct {
	Error           string `json:"error,omitempty"`
	FileError       string `json:"fileError,omitempty"`
	FitError        string `json:"fitError,omitempty"`
	DurationMS      int64  `json:"durationMS"`
	SolveDurationMS int64  `json:"solveDurationMS"`
	Pages           int    `json:"pages"`
	PreviewPages    int    `json:"previewPages"`
	Smart           bool   `json:"smart"`
}

func TestSmartOnePageFitsAcceptanceFixtureAndRemainsEditableUnderHeadlessLinux(t *testing.T) {
	if !cgoAvailable {
		t.Skip("WebKitGTK Smart One-Page probe requires cgo")
	}
	if _, err := os.Stat(acceptanceResumePath); err != nil {
		t.Skipf("acceptance Resume Document fixture unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	runtimeDir, waylandDisplay := startHeadlessMutter(t)

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(environmentWithout("DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND"),
		"TOKICV_SMART_ONEPAGE_PROBE=1",
		"TOKICV_SMART_ONEPAGE_DATA="+t.TempDir(),
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
		t.Fatalf("Smart One-Page probe timed out: %v; stderr: %s", ctx.Err(), stderr.String())
	}
	if err != nil {
		t.Fatalf("Smart One-Page probe failed: %v; stderr: %s", err, stderr.String())
	}

	var raw string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if value, ok := strings.CutPrefix(line, smartOnePageProbeMarker); ok {
			raw = value
		}
	}
	if raw == "" {
		t.Fatalf("no Smart One-Page result; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	}
	var report smartOnePageProbeReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode Smart One-Page result: %v; result: %s", err, raw)
	}
	if report.Error != "" {
		t.Fatalf("Smart One-Page probe: %s", report.Error)
	}
	t.Logf("Smart One-Page: %s", raw)
	if report.Pages != 1 || !report.Smart {
		t.Errorf("Smart One-Page = %d pages, enabled=%t", report.Pages, report.Smart)
	}
	if report.MarginTop != 10 || report.MarginBottom != 10 {
		t.Errorf("acceptance fixture margins = %v / %v mm, want 10 / 10", report.MarginTop, report.MarginBottom)
	}
	if report.SectionGap != 8 || report.ItemGap != 3 || report.LineHeight != 1.30 {
		t.Errorf("Smart One-Page profile = %+v", report)
	}
	if !report.Adjusted || report.AdjustedSectionGap != 10 {
		t.Errorf("layout popover adjustment = %t, section gap %v", report.Adjusted, report.AdjustedSectionGap)
	}
	if report.SmartAfterAdjust {
		t.Error("manual Layout Profile editing left Smart One-Page active")
	}
	if report.SolveDurationMS > 2000 {
		t.Errorf("Smart One-Page DOM prefilter plus print confirmation took %dms, want below 2s", report.SolveDurationMS)
	}
	if report.DurationMS > 2000 {
		t.Errorf("Smart One-Page click-to-result took %dms, want below 2s", report.DurationMS)
	}
	if report.RefitDurationMS <= 0 || report.RefitSolveMS <= 0 {
		t.Errorf("automatic refit did not run: %+v", report)
	}
	if report.RefitDurationMS > 2000 {
		t.Errorf("automatic refit click-to-result took %dms, want below 2s", report.RefitDurationMS)
	}
	if report.RefitSolveMS > 2000 {
		t.Errorf("automatic refit solve took %dms, want below 2s", report.RefitSolveMS)
	}
}

func TestSmartOnePageFitsOnePageResumesInRealWindowUnderHeadlessLinux(t *testing.T) {
	if !cgoAvailable {
		t.Skip("WebKitGTK Smart One-Page real-window probe requires cgo")
	}
	for _, tc := range []struct {
		name   string
		source string
		path   string
	}{
		{
			name:   "short",
			source: "# 示例文档\n\n## 技能\n\n- Go\n",
		},
		{
			name: "acceptance",
			path: acceptanceResumePath,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.path
			if path == "" {
				path = filepath.Join(t.TempDir(), "resume.md")
				if err := os.WriteFile(path, []byte(tc.source), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if _, err := os.Stat(path); err != nil {
				t.Skipf("acceptance Resume Document fixture unavailable: %v", err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			runtimeDir, waylandDisplay := startHeadlessMutter(t)
			dataDir := t.TempDir()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
			cmd.Env = append(environmentWithout(
				"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND",
				"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
			),
				"TOKICV_SMART_ONEPAGE_REAL_PROBE=1",
				"TOKICV_SMART_ONEPAGE_REAL_PATH="+path,
				"TOKICV_SMART_ONEPAGE_REAL_DATA="+dataDir,
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
				t.Fatalf("Smart One-Page real-window probe timed out: %v; stderr: %s", ctx.Err(), stderr.String())
			}
			if err != nil {
				t.Fatalf("Smart One-Page real-window probe failed: %v; stderr: %s", err, stderr.String())
			}

			var raw string
			for _, line := range strings.Split(stdout.String(), "\n") {
				if value, ok := strings.CutPrefix(line, smartOnePageRealWindowProbeMarker); ok {
					raw = value
				}
			}
			if raw == "" {
				t.Fatalf("no Smart One-Page real-window result; stdout: %s; stderr: %s",
					stdout.String(), stderr.String())
			}
			var report smartOnePageRealWindowReport
			if err := json.Unmarshal([]byte(raw), &report); err != nil {
				t.Fatalf("decode Smart One-Page real-window result: %v; result: %s", err, raw)
			}
			t.Logf("Smart One-Page real window: %s", raw)
			if report.Error != "" {
				t.Fatalf("Smart One-Page real-window probe: %s", report.Error)
			}
			if report.FileError != "" {
				t.Errorf("file error = %q, want empty", report.FileError)
			}
			if report.FitError != "" {
				t.Errorf("fit error = %q, want nil", report.FitError)
			}
			if report.Pages != 1 {
				t.Errorf("page count = %d, want 1", report.Pages)
			}
			if report.PreviewPages != 1 {
				t.Errorf("preview page count = %d, want 1", report.PreviewPages)
			}
			if !report.Smart {
				t.Error("Smart One-Page is not enabled")
			}
			if report.SolveDurationMS >= 2000 {
				t.Errorf("Smart One-Page solve took %dms, want below 2s", report.SolveDurationMS)
			}
		})
	}
}

func runSmartOnePageProbe() int {
	if dir := os.Getenv("TOKICV_SMART_ONEPAGE_DATA"); dir != "" {
		userData := filepath.Join(dir, "user-data")
		cache := filepath.Join(dir, "cache")
		_ = os.MkdirAll(userData, 0o755)
		_ = os.MkdirAll(cache, 0o755)
		mygo.App.SetPath(mygo.PathUserData, userData)
		mygo.App.SetPath(mygo.PathCache, cache)
	}
	mygo.App.SetName("tokicv-smart-onepage")

	done := make(chan smartOnePageProbeReport, 1)
	mygo.App.WhenReady(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			done <- inspectSmartOnePage(ctx)
			mygo.App.Quit()
		}()
	})
	runErr := mygo.App.Run()
	var report smartOnePageProbeReport
	select {
	case report = <-done:
	case <-time.After(5 * time.Second):
		report.Error = "the Smart One-Page probe did not finish"
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal Smart One-Page report: %v\n", err)
		return 1
	}
	fmt.Printf("%s%s\n", smartOnePageProbeMarker, data)
	return 0
}

type smartOnePageRecordingPipeline struct {
	ArtifactPipeline
	mu     sync.Mutex
	fitErr error
}

func (p *smartOnePageRecordingPipeline) FitOnePage(
	ctx context.Context,
	html string,
	base render.Layout,
) (export.OnePageFit, error) {
	fit, err := p.ArtifactPipeline.FitOnePage(ctx, html, base)
	p.mu.Lock()
	p.fitErr = err
	p.mu.Unlock()
	return fit, err
}

func (p *smartOnePageRecordingPipeline) fitError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fitErr
}

func runSmartOnePageRealWindowProbe() int {
	if dir := os.Getenv("TOKICV_SMART_ONEPAGE_REAL_DATA"); dir != "" {
		userData := filepath.Join(dir, "user-data")
		cache := filepath.Join(dir, "cache")
		_ = os.MkdirAll(userData, 0o755)
		_ = os.MkdirAll(cache, 0o755)
		mygo.App.SetPath(mygo.PathUserData, userData)
		mygo.App.SetPath(mygo.PathCache, cache)
	}
	mygo.App.SetName("tokicv-smart-onepage-real")
	mygo.App.SetLocale("zh-Hans")

	done := make(chan smartOnePageRealWindowReport, 1)
	mygo.App.WhenReady(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			done <- inspectSmartOnePageRealWindow(
				ctx,
				os.Getenv("TOKICV_SMART_ONEPAGE_REAL_PATH"),
			)
			mygo.App.Quit()
		}()
	})
	runErr := mygo.App.Run()
	var report smartOnePageRealWindowReport
	select {
	case report = <-done:
	case <-time.After(5 * time.Second):
		report.Error = "the Smart One-Page real-window probe did not finish"
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal Smart One-Page real-window report: %v\n", err)
		return 1
	}
	fmt.Printf("%s%s\n", smartOnePageRealWindowProbeMarker, data)
	return 0
}

func inspectSmartOnePageRealWindow(ctx context.Context, path string) smartOnePageRealWindowReport {
	if path == "" {
		return smartOnePageRealWindowReport{Error: "missing Resume Document path"}
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return smartOnePageRealWindowReport{Error: "read Resume Document: " + err.Error()}
	}

	app := New(Options{})
	defer app.CloseAndWait()
	app.loadDocument(path, string(source))
	pipeline := &smartOnePageRecordingPipeline{ArtifactPipeline: newDesktopArtifactPipeline()}
	app.SetArtifactPipeline(pipeline)

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "tokicv",
		Width:           1280,
		Height:          800,
		UseContentSize:  true,
		BackgroundColor: "#EDECE8",
		Content:         ui.View(app.View),
	})
	defer win.Destroy()
	app.SetUpdate(win.Update)
	app.SetInvalidate(win.Invalidate)
	app.SetTitleHandler(win.SetTitle)

	web, err := win.NewWebView(mygo.WebViewOptions{
		Page: mygo.PageOptions{
			BackgroundThrottling: mygo.ThrottlingOff,
			Untrusted:            true,
		},
		BackgroundColor: "#E0DFDB",
	})
	if err != nil {
		return smartOnePageRealWindowReport{Error: "create preview: " + err.Error()}
	}
	web.Page().OnPageTitleUpdated(func(event *mygo.TitleEvent) {
		if count, ok := render.PreviewPageCount(event.Title); ok {
			event.PreventDefault()
			win.Update(func() { app.SetPageCount(count) })
		}
	})
	app.SetPreview(&webPreview{web: web})
	win.Invalidate()
	if _, err := waitForPreviewPages(ctx, web, 0); err != nil {
		return smartOnePageRealWindowReport{Error: "initial preview: " + err.Error()}
	}

	started := time.Now()
	mygo.RunOnMain(func() { app.setSmartOnePage(true) })
	app.work.Wait()
	win.Update(func() {})
	report := smartOnePageRealWindowReport{
		DurationMS: time.Since(started).Milliseconds(),
	}
	mygo.RunOnMain(func() {
		report.FileError = app.fileError
		report.SolveDurationMS = app.smartSolveDuration.Milliseconds()
		report.Pages = app.PageCount()
		report.Smart = app.smartOnePage
		if err := pipeline.fitError(); err != nil {
			report.FitError = err.Error()
		}
	})
	if report.FileError == "" {
		waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
		pages, err := waitForPreviewPages(waitCtx, web, 1)
		waitCancel()
		if err != nil {
			report.Error = "preview did not reach one page: " + err.Error()
		} else {
			report.PreviewPages = pages
		}
	}
	return report
}

func waitForPreviewPages(ctx context.Context, web *mygo.WebView, want int) (int, error) {
	for {
		value, err := web.Page().EvalContext(ctx,
			`document.documentElement.dataset.tokicvPages || ""`)
		if err == nil {
			if text, ok := value.(string); ok && text != "" {
				pages, convErr := strconv.Atoi(text)
				if convErr == nil && pages > 0 && (want == 0 || pages == want) {
					return pages, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func inspectSmartOnePage(ctx context.Context) smartOnePageProbeReport {
	source, err := os.ReadFile(acceptanceResumePath)
	if err != nil {
		return smartOnePageProbeReport{Error: "read acceptance fixture: " + err.Error()}
	}

	warm := mygo.NewWindow(mygo.WindowOptions{
		Hidden:          true,
		Width:           1280,
		Height:          800,
		UseContentSize:  true,
		BackgroundColor: "#E0DFDB",
		Page: mygo.PageOptions{
			BackgroundThrottling: mygo.ThrottlingOff,
			Untrusted:            true,
		},
	})
	defer warm.Destroy()
	warm.Page().LoadHTML(render.PreviewHTML(dialect.Parse("# 预热\n"), render.DefaultLayout()), "")
	for {
		value, evalErr := warm.Page().EvalContext(ctx, `document.documentElement.dataset.tokicvPages || ""`)
		if evalErr == nil {
			if pages, ok := value.(string); ok && pages != "" {
				break
			}
		}
		select {
		case <-ctx.Done():
			return smartOnePageProbeReport{Error: "warm preview did not finish: " + ctx.Err().Error()}
		case <-time.After(50 * time.Millisecond):
		}
	}

	app := New(Options{})
	preview := &fakePreview{nextPages: 1, onPages: app.SetPageCount}
	app.SetPreview(preview)
	app.loadDocument(acceptanceResumePath, string(source))
	app.SetArtifactPipeline(desktopArtifactPipeline{runner: hiddenPageRunner{}})
	tester := ui.NewTester(app.View, 1440, 900)
	tester.Frame()

	started := time.Now()
	if err := tester.Click("智能一页"); err != nil {
		return smartOnePageProbeReport{Error: "click Smart One-Page: " + err.Error()}
	}
	tester.Frame()
	app.work.Wait()
	app.drainUpdates()
	tester.Frame()
	duration := time.Since(started)

	report := smartOnePageProbeReport{
		DurationMS:      duration.Milliseconds(),
		SolveDurationMS: app.smartSolveDuration.Milliseconds(),
		Pages:           app.PageCount(),
		Smart:           app.smartOnePage,
		SectionGap:      app.layout.SectionGapPX,
		ItemGap:         app.layout.ItemGapPX,
		MarginTop:       app.layout.MarginTopMM,
		MarginBottom:    app.layout.MarginBottomMM,
		LineHeight:      app.layout.LineHeight,
	}
	if !report.Smart {
		report.Error = fmt.Sprintf("Smart One-Page failed to fit: notice=%q file=%q", app.layoutNotice, app.fileError)
		return report
	}

	started = time.Now()
	app.buffer.Set(app.buffer.String() + "\n")
	tester.Frame()
	tester.Advance(PreviewDebounce)
	tester.Frame()
	app.work.Wait()
	app.drainUpdates()
	tester.Frame()
	report.RefitDurationMS = time.Since(started).Milliseconds()
	report.RefitSolveMS = app.smartSolveDuration.Milliseconds()
	report.Pages = app.PageCount()
	report.Smart = app.smartOnePage
	if !report.Smart {
		report.Error = fmt.Sprintf("automatic refit failed: notice=%q file=%q", app.layoutNotice, app.fileError)
		return report
	}

	if err := tester.Click("排版"); err != nil {
		report.Error = "open Layout Profile: " + err.Error()
		return report
	}
	tester.Frame()
	field, ok := tester.AccessByRole(ui.RoleTextField, "分节间距 (px)")
	if !ok {
		report.Error = "Layout Profile has no section spacing field"
		return report
	}
	tester.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
	tester.Key(ui.Cmd, ui.KeyA)
	tester.Type("10")
	tester.Frame()
	report.Adjusted = app.baseLayout.SectionGapPX == 10
	report.AdjustedSectionGap = app.baseLayout.SectionGapPX
	report.SmartAfterAdjust = app.smartOnePage
	return report
}

var _ export.PageRunner = hiddenPageRunner{}
