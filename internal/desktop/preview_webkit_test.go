//go:build linux

package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"tokicv/internal/dialect"
	"tokicv/internal/render"
)

const previewProbeMarker = "tokicv-preview-probe="
const previewUpdateMarker = "tokicv-preview-update="
const isolatedMutterStartupTimeout = 30 * time.Second

type previewProbeReport struct {
	Error              string   `json:"error,omitempty"`
	Pages              int      `json:"pages"`
	Paragraphs         int      `json:"paragraphs"`
	VisibleParagraphs  int      `json:"visibleParagraphs"`
	MeasureLines       int      `json:"measureLines"`
	VisibleLines       int      `json:"visibleLines"`
	DuplicateLines     int      `json:"duplicateLines"`
	PartialLines       int      `json:"partialLines"`
	OutsideLines       int      `json:"outsideLines"`
	WrapDrift          int      `json:"wrapDrift"`
	MarginViolations   int      `json:"marginViolations"`
	Issues             []string `json:"issues"`
	InnerWidth         float64  `json:"innerWidth"`
	InnerHeight        float64  `json:"innerHeight"`
	Scale              float64  `json:"scale"`
	ExpectedScale      float64  `json:"expectedScale"`
	SlotWidth          float64  `json:"slotWidth"`
	SlotHeight         float64  `json:"slotHeight"`
	HorizontalOverflow bool     `json:"horizontalOverflow"`
	VerticalOverflow   bool     `json:"verticalOverflow"`
}

type previewUpdateReport struct {
	Error           string `json:"error,omitempty"`
	Updated         bool   `json:"updated"`
	Marker          string `json:"marker"`
	NavigationCount int    `json:"navigationCount"`
	Pages           int    `json:"pages"`
	Heading         string `json:"heading"`
	MarginTop       string `json:"marginTop"`
	MarginBottom    string `json:"marginBottom"`
	LineHeight      string `json:"lineHeight"`
	FontFamily      string `json:"fontFamily"`
	SourceUpdated   bool   `json:"sourceUpdated"`
}

func TestMain(m *testing.M) {
	if os.Getenv("TOKICV_LIST_GEOMETRY_PROBE") == "1" {
		os.Exit(runListGeometryProbe())
	}
	if os.Getenv("TOKICV_PREVIEW_DOM_PROBE") == "1" {
		os.Exit(runPreviewDOMProbe())
	}
	if os.Getenv("TOKICV_DESKTOP_PARITY_PROBE") == "1" {
		os.Exit(runDesktopParityProbe())
	}
	if os.Getenv("TOKICV_PREVIEW_UPDATE_PROBE") == "1" {
		os.Exit(runPreviewUpdateProbe())
	}
	if os.Getenv("TOKICV_SMART_ONEPAGE_PROBE") == "1" {
		os.Exit(runSmartOnePageProbe())
	}
	if os.Getenv("TOKICV_SMART_ONEPAGE_REAL_PROBE") == "1" {
		os.Exit(runSmartOnePageRealWindowProbe())
	}
	if os.Getenv("TOKICV_PREVIEW_PERF_PROBE") == "1" {
		os.Exit(runPreviewPerfProbe())
	}
	os.Exit(m.Run())
}

func TestPreviewWebKitUpdatesInPlaceWithoutReloadingUnderHeadlessLinux(t *testing.T) {
	if !cgoAvailable {
		t.Skip("WebKitGTK preview probe requires cgo")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runtimeDir, waylandDisplay := startHeadlessMutter(t)

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(environmentWithout("DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND"),
		"TOKICV_PREVIEW_UPDATE_PROBE=1",
		"TOKICV_PREVIEW_UPDATE_DATA="+t.TempDir(),
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
		t.Fatalf("preview update probe timed out: %v; stderr: %s", ctx.Err(), stderr.String())
	}
	if err != nil {
		t.Fatalf("preview update probe failed: %v; stderr: %s", err, stderr.String())
	}

	var raw string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if value, ok := strings.CutPrefix(line, previewUpdateMarker); ok {
			raw = value
		}
	}
	if raw == "" {
		t.Fatalf("no preview update result; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	}
	var report previewUpdateReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode preview update result: %v; result: %s", err, raw)
	}
	if report.Error != "" {
		t.Fatalf("preview update probe: %s", report.Error)
	}
	if !report.Updated {
		t.Error("the in-place preview update did not report success")
	}
	if report.Marker != "stable" {
		t.Errorf("preview window marker = %q, want the same JavaScript context", report.Marker)
	}
	if report.NavigationCount > 1 {
		t.Errorf("navigation count = %d, want at most the initial load", report.NavigationCount)
	}
	if report.Heading != "更新标题" || !report.SourceUpdated {
		t.Errorf("updated preview heading = %q, source updated=%t", report.Heading, report.SourceUpdated)
	}
	if report.Pages < 1 {
		t.Errorf("updated preview pages = %d", report.Pages)
	}
	if report.MarginTop != "8" || report.MarginBottom != "7.5" {
		t.Errorf("updated preview margins = %s / %s, want 8 / 7.5", report.MarginTop, report.MarginBottom)
	}
	if report.LineHeight != "1.25" {
		t.Errorf("updated preview line height = %q, want 1.25", report.LineHeight)
	}
	if !strings.Contains(report.FontFamily, "Noto Serif CJK SC") {
		t.Errorf("updated preview font family = %q", report.FontFamily)
	}
}

func TestPreviewWebKitDOMPaginationWithStyledLongParagraphAtLaptopViewport(t *testing.T) {
	if !cgoAvailable {
		t.Skip("WebKitGTK DOM pagination probe requires cgo")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	runtimeDir, waylandDisplay := startHeadlessMutter(t)

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(environmentWithout("DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND"),
		"TOKICV_PREVIEW_DOM_PROBE=1",
		"TOKICV_PREVIEW_PROBE_DATA="+t.TempDir(),
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
		t.Fatalf("WebKitDOM probe timed out: %v; stderr: %s", ctx.Err(), stderr.String())
	}
	if err != nil {
		t.Fatalf("WebKitDOM probe failed: %v; stderr: %s", err, stderr.String())
	}

	var raw string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if value, ok := strings.CutPrefix(line, previewProbeMarker); ok {
			raw = value
		}
	}
	if raw == "" {
		t.Fatalf("no WebKitDOM probe result; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	}
	var report previewProbeReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode WebKitDOM probe: %v; result: %s", err, raw)
	}
	if report.Error != "" {
		t.Fatalf("WebKitDOM probe: %s", report.Error)
	}
	if report.InnerWidth != 1280 || report.InnerHeight != 800 {
		t.Errorf("viewport = %gx%g, want 1280x800", report.InnerWidth, report.InnerHeight)
	}
	if report.Pages < 2 {
		t.Errorf("pages = %d, want a multi-page document", report.Pages)
	}
	if report.Paragraphs != 100 || report.VisibleParagraphs != 100 {
		t.Errorf("paragraphs = %d visible %d, want 100 visible", report.Paragraphs, report.VisibleParagraphs)
	}
	if report.MeasureLines == 0 || report.VisibleLines != report.MeasureLines {
		t.Errorf("visible lines = %d, measured %d", report.VisibleLines, report.MeasureLines)
	}
	if report.DuplicateLines != 0 {
		t.Errorf("duplicate visible lines = %d", report.DuplicateLines)
	}
	if report.PartialLines != 0 {
		t.Errorf("partially clipped lines = %d", report.PartialLines)
	}
	if report.OutsideLines != 0 {
		t.Errorf("horizontally overflowing lines = %d", report.OutsideLines)
	}
	if report.WrapDrift != 0 {
		t.Errorf("lines with unstable wrapping = %d", report.WrapDrift)
	}
	if report.MarginViolations != 0 {
		t.Errorf("page viewports outside the content box = %d", report.MarginViolations)
	}
	if len(report.Issues) != 0 {
		t.Errorf("layout issues: %s", strings.Join(report.Issues, "; "))
	}
	if math.Abs(report.Scale-report.ExpectedScale) > 0.002 {
		t.Errorf("scale = %g, expected %g", report.Scale, report.ExpectedScale)
	}
	if report.SlotWidth <= 0 || report.SlotHeight <= 0 {
		t.Errorf("scaled Sheet size = %gx%g", report.SlotWidth, report.SlotHeight)
	}
	if report.SlotWidth > report.InnerWidth+0.5 {
		t.Errorf("scaled Sheet width %g does not fit the viewport", report.SlotWidth)
	}
	if report.SlotHeight <= report.InnerHeight {
		t.Errorf("scaled Sheet height %g did not exceed the viewport", report.SlotHeight)
	}
	if !report.VerticalOverflow {
		t.Error("the preview cannot scroll through the Sheet height")
	}
	if report.HorizontalOverflow {
		t.Error("the preview has horizontal overflow")
	}
}

func runPreviewUpdateProbe() int {
	if dir := os.Getenv("TOKICV_PREVIEW_UPDATE_DATA"); dir != "" {
		userData := filepath.Join(dir, "user-data")
		cache := filepath.Join(dir, "cache")
		_ = os.MkdirAll(userData, 0o755)
		_ = os.MkdirAll(cache, 0o755)
		mygo.App.SetPath(mygo.PathUserData, userData)
		mygo.App.SetPath(mygo.PathCache, cache)
	}
	mygo.App.SetName("tokicv-preview-update")

	done := make(chan previewUpdateReport, 1)
	mygo.App.WhenReady(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			done <- inspectPreviewUpdate(ctx)
			mygo.App.Quit()
		}()
	})
	runErr := mygo.App.Run()
	var report previewUpdateReport
	select {
	case report = <-done:
	case <-time.After(5 * time.Second):
		report.Error = "the preview update probe did not finish"
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal preview update report: %v\n", err)
		return 1
	}
	fmt.Printf("%s%s\n", previewUpdateMarker, data)
	return 0
}

func inspectPreviewUpdate(ctx context.Context) previewUpdateReport {
	window := mygo.NewWindow(mygo.WindowOptions{
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
	defer window.Destroy()

	initial := render.PreviewFrameFor(dialect.Parse("# 初始标题\n\n初始内容\n"), render.DefaultLayout())
	window.Page().LoadHTML(initial.HTML, "")
	for {
		value, err := window.Page().EvalContext(ctx, `document.documentElement.dataset.tokicvPages || ""`)
		if err == nil {
			if pages, ok := value.(string); ok && pages != "" {
				break
			}
		}
		select {
		case <-ctx.Done():
			return previewUpdateReport{Error: "initial preview did not finish: " + ctx.Err().Error()}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if _, err := window.Page().EvalContext(ctx, `window.__tokicvStableMarker = "stable"; true`); err != nil {
		return previewUpdateReport{Error: "set preview marker: " + err.Error()}
	}

	layout := render.DefaultLayout()
	layout.FontStack = render.FontSerif
	layout.LineHeight = 1.25
	layout.MarginTopMM = 8
	layout.MarginBottomMM = 7.5
	layout.SectionGapPX = 8
	layout.ItemGapPX = 3
	updated := render.PreviewFrameFor(dialect.Parse("# 更新标题\n\n- 新内容\n"), layout)
	value, err := window.Page().EvalContext(ctx, render.PreviewUpdateScript(updated))
	if err != nil {
		return previewUpdateReport{Error: "update preview: " + err.Error()}
	}
	applied, _ := value.(bool)

	var report previewUpdateReport
	report.Updated = applied
	script := `(() => {
  const root = document.documentElement;
  const probe = document.getElementById("a4-probe");
  const source = document.getElementById("resume-source");
  const heading = document.querySelector("#stage h1");
  return {
    marker: window.__tokicvStableMarker || "",
    navigationCount: performance.getEntriesByType("navigation").length,
    pages: Number(root.dataset.tokicvPages || 0),
    heading: heading ? heading.textContent : "",
    marginTop: probe ? probe.dataset.marginTop : "",
    marginBottom: probe ? probe.dataset.marginBottom : "",
    lineHeight: getComputedStyle(root).getPropertyValue("--line-height").trim(),
    fontFamily: getComputedStyle(document.body).fontFamily,
    sourceUpdated: !!(source && source.innerHTML.includes("更新标题"))
  };
})()`
	inspection, err := mygo.EvalAs[previewUpdateReport](window.Page(), script)
	if err != nil {
		return previewUpdateReport{Error: "inspect preview: " + err.Error()}
	}
	inspection.Updated = applied
	return inspection
}

func startHeadlessMutter(t *testing.T) (string, string) {
	return startHeadlessMutterAt(t, "1440x900")
}

func startHeadlessMutterAt(t *testing.T, monitor string) (string, string) {
	t.Helper()
	path, err := exec.LookPath("mutter")
	if err != nil {
		t.Skip("mutter is required for the isolated WebKitGTK viewport")
	}
	root, err := os.MkdirTemp("", "tokicv-dt-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for _, name := range []string{"home", "config", "cache", "data"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	dbusAddress := startIsolatedDBus(t, root)
	display := "wayland-0"
	cmd := exec.Command(path,
		"--headless",
		"--wayland",
		"--no-x11",
		"--wayland-display", display,
		"--virtual-monitor", monitor,
	)
	cmd.Env = append(environmentWithout(
		"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND",
		"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		"DBUS_SESSION_BUS_ADDRESS", "MUTTER_DEBUG_KMS_THREAD_TYPE",
	),
		"HOME="+filepath.Join(root, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(root, "config"),
		"XDG_DATA_HOME="+filepath.Join(root, "data"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"),
		"XDG_RUNTIME_DIR="+root,
		"WAYLAND_DISPLAY="+display,
		"GDK_BACKEND=wayland",
		"MUTTER_DEBUG_KMS_THREAD_TYPE=user",
		"XDG_SESSION_TYPE=wayland",
		"DBUS_SESSION_BUS_ADDRESS="+dbusAddress,
		"GIO_USE_VFS=local",
		"GSETTINGS_BACKEND=memory",
		"DISPLAY=",
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Skipf("start mutter: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	socket := filepath.Join(root, display)
	deadline := time.Now().Add(isolatedMutterStartupTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			return root, display
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("mutter did not create %s: %s", socket, output.String())
	return "", ""
}

func environmentWithout(names ...string) []string {
	skip := make(map[string]bool, len(names))
	for _, name := range names {
		skip[name] = true
	}
	env := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !skip[name] {
			env = append(env, value)
		}
	}
	return env
}

func runPreviewDOMProbe() int {
	if dir := os.Getenv("TOKICV_PREVIEW_PROBE_DATA"); dir != "" {
		userData := filepath.Join(dir, "user-data")
		cache := filepath.Join(dir, "cache")
		_ = os.MkdirAll(userData, 0o755)
		_ = os.MkdirAll(cache, 0o755)
		mygo.App.SetPath(mygo.PathUserData, userData)
		mygo.App.SetPath(mygo.PathCache, cache)
	}
	mygo.App.SetName("tokicv-preview-probe")

	done := make(chan previewProbeReport, 1)
	mygo.App.WhenReady(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			done <- inspectPreviewDOM(ctx)
			mygo.App.Quit()
		}()
	})

	runErr := mygo.App.Run()
	var report previewProbeReport
	select {
	case report = <-done:
	case <-time.After(5 * time.Second):
		report.Error = "the preview probe did not finish"
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal preview probe: %v\n", err)
		return 1
	}
	fmt.Printf("%s%s\n", previewProbeMarker, data)
	return 0
}

func inspectPreviewDOM(ctx context.Context) previewProbeReport {
	var source strings.Builder
	source.WriteString("# 分页验证\n\n")
	for i := 0; i < 40; i++ {
		source.WriteString("这是用于验证分页边界的**加粗中文片段**与 mixed English text、数字 0123456789 混排行盒，确保同一视觉行不会被上下两页裁切或重复显示。")
	}
	source.WriteString("\n\n## 99 个稳定段落\n\n")
	for i := 1; i <= 99; i++ {
		fmt.Fprintf(&source,
			"[%03d] 分页裁剪验证段落 %03d。这里使用稳定的中文文本、Latin words 与 0123456789 来检查 A4 页面边界，确保内容不会进入上下页边距、不会在相邻页重复，同时保持一致的换行宽度。\n\n",
			i, i)
	}

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "tokicv preview probe",
		Frameless:       true,
		Width:           1280,
		Height:          800,
		UseContentSize:  true,
		BackgroundColor: "#E0DFDB",
		Page: mygo.PageOptions{
			BackgroundThrottling: mygo.ThrottlingOff,
			Untrusted:            true,
		},
	})
	defer win.Destroy()
	win.Page().LoadHTML(render.PreviewHTML(dialect.Parse(source.String()), render.DefaultLayout()), "")

	for {
		value, err := win.Page().EvalContext(ctx, `document.documentElement.dataset.tokicvPages || ""`)
		if err == nil {
			if pages, ok := value.(string); ok && pages != "" {
				break
			}
		}
		select {
		case <-ctx.Done():
			return previewProbeReport{Error: "preview pagination did not finish: " + ctx.Err().Error()}
		case <-time.After(50 * time.Millisecond):
		}
	}

	report, err := mygo.EvalAs[previewProbeReport](win.Page(), previewDOMProbeScript)
	if err != nil {
		return previewProbeReport{Error: err.Error()}
	}
	return report
}

const previewDOMProbeScript = `(() => {
  const mm = 96 / 25.4;
  const stage = document.getElementById("stage");
  const probe = document.getElementById("a4-probe");
  const measure = document.getElementById("measure");
  const sheets = Array.from(document.querySelectorAll(".sheet"));
  const flows = Array.from(document.querySelectorAll(".page-flow"));
  const slots = Array.from(document.querySelectorAll(".sheet-slot"));
  const issues = [];
  const add = (name, detail) => issues.push(detail ? name + ": " + detail : name);
  const textRects = (element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    return Array.from(range.getClientRects()).filter((rect) => rect.height > 0 && rect.width > 0);
  };
  const lineBoxes = (element) => {
    const rects = textRects(element).map((rect) => ({
      top: rect.top,
      bottom: rect.bottom,
      left: rect.left,
      right: rect.right
    }));
    rects.sort((a, b) => a.top - b.top || a.bottom - b.bottom);
    const lines = [];
    for (const rect of rects) {
      const line = lines[lines.length - 1];
      const tolerance = Math.max(1, Math.min(rect.bottom - rect.top,
        line ? line.bottom - line.top : rect.bottom - rect.top) * 0.3);
      if (line && Math.abs(rect.top - line.top) <= tolerance) {
        line.top = Math.min(line.top, rect.top);
        line.bottom = Math.max(line.bottom, rect.bottom);
        line.left = Math.min(line.left, rect.left);
        line.right = Math.max(line.right, rect.right);
        continue;
      }
      lines.push({ ...rect });
    }
    for (let index = 0; index + 1 < lines.length; index++) {
      lines[index].bottom = lines[index + 1].top;
    }
    return lines;
  };
  const same = (name, values) => {
    for (const value of values.slice(1)) {
      if (value !== values[0]) add(name, values.join(" / "));
    }
  };

  const pageWidth = probe.getBoundingClientRect().width;
  const pageHeight = probe.getBoundingClientRect().height;
  const sheetScale = sheets[0].getBoundingClientRect().width / (210 * mm);
  const marginTop = (parseFloat(probe.dataset.marginTop) || 10) * mm;
  const marginBottom = (parseFloat(probe.dataset.marginBottom) || 10) * mm;
  const measureStyle = getComputedStyle(measure);
  const sheetStyle = getComputedStyle(sheets[0]);
  const flowStyle = getComputedStyle(flows[0]);
  const meterParagraph = getComputedStyle(measure.querySelector("p"));
  const flowParagraph = getComputedStyle(flows[0].querySelector("p"));

  const sectionGap = measureStyle.getPropertyValue("--section-gap").trim();
  const itemGap = measureStyle.getPropertyValue("--item-gap").trim();
  if (sectionGap !== "12px") add("measure --section-gap", sectionGap);
  if (itemGap !== "5px") add("measure --item-gap", itemGap);
  if (meterParagraph.marginBottom !== "5px") add("measure paragraph gap", meterParagraph.marginBottom);
  if (flowParagraph.marginBottom !== "5px") add("page paragraph gap", flowParagraph.marginBottom);

  same("font family", [measureStyle.fontFamily, sheetStyle.fontFamily, flowStyle.fontFamily]);
  same("font size", [measureStyle.fontSize, sheetStyle.fontSize, flowStyle.fontSize]);
  same("font weight", [measureStyle.fontWeight, sheetStyle.fontWeight, flowStyle.fontWeight]);
  same("line height", [measureStyle.lineHeight, sheetStyle.lineHeight, flowStyle.lineHeight]);
  same("overflow wrap", [measureStyle.overflowWrap, sheetStyle.overflowWrap, flowStyle.overflowWrap]);
  if (flowStyle.overflowWrap !== "break-word") add("page overflow-wrap", flowStyle.overflowWrap);

  const measureLines = new Map();
  let measureLineCount = 0;
  for (const paragraph of measure.querySelectorAll("p")) {
    const id = paragraph.textContent.slice(0, 5);
    const lines = lineBoxes(paragraph).map((line) => ({ width: line.right - line.left }));
    measureLines.set(id, lines);
    measureLineCount += lines.length;
  }

  const seenLines = new Set();
  const visibleParagraphs = new Set();
  let duplicateLines = 0;
  let partialLines = 0;
  let outsideLines = 0;
  let wrapDrift = 0;
  let marginViolations = 0;

  for (const flow of flows) {
    const sheet = flow.closest(".sheet");
    const flowRect = flow.getBoundingClientRect();
    const start = parseFloat(flow.dataset.pageStart);
    const end = parseFloat(flow.dataset.pageEnd);
    if (Math.abs(flow.offsetTop - marginTop) > 1) {
      marginViolations++;
      add("page top margin", flow.offsetTop + " vs " + marginTop);
    }
    if (Math.abs(flow.offsetHeight - (end - start)) > 1) {
      marginViolations++;
      add("page clip height", flow.offsetHeight + " vs " + (end - start));
    }
    if (flow.offsetTop + flow.offsetHeight > sheet.offsetHeight - marginBottom + 1) {
      marginViolations++;
      add("page bottom margin", flow.offsetTop + flow.offsetHeight + " vs " + sheet.offsetHeight);
    }

    const content = flow.querySelector(".page-flow-content");
    for (const paragraph of content.querySelectorAll("p")) {
      const id = paragraph.textContent.slice(0, 5);
      const lines = lineBoxes(paragraph);
      for (let index = 0; index < lines.length; index++) {
        const line = lines[index];
        const key = id + "#" + index;
        if (line.bottom <= flowRect.top + 0.5 || line.top >= flowRect.bottom - 0.5) continue;
        if (line.top < flowRect.top - 0.5 || line.bottom > flowRect.bottom + 0.5) partialLines++;
        if (line.left < flowRect.left - 0.5 || line.right > flowRect.right + 0.5) outsideLines++;
        if (seenLines.has(key)) {
          duplicateLines++;
          add("duplicate line", key);
        }
        seenLines.add(key);
        const expected = measureLines.get(id);
        if (!expected || !expected[index] ||
            Math.abs(expected[index].width - (line.right - line.left) / sheetScale) > 0.75) {
          wrapDrift++;
          add("unstable wrap", key);
        }
        visibleParagraphs.add(id);
      }
    }
  }

  const availableWidth = Math.max(1, stage.clientWidth - 40);
  const expectedScale = Math.max(0.8, Math.min(availableWidth / pageWidth, 1));
  const slotRect = slots[0].getBoundingClientRect();
  const scale = slotRect.width / pageWidth;
  const sameSize = slots.every((slot) => {
    const rect = slot.getBoundingClientRect();
    return Math.abs(rect.width - slotRect.width) <= 0.5 && Math.abs(rect.height - slotRect.height) <= 0.5;
  });
  if (!sameSize) add("Sheet slots", "scaled sizes differ");
  if (Math.abs(scale - expectedScale) > 0.002) add("scale", scale + " vs " + expectedScale);
  if (Math.abs(slotRect.height - pageHeight * expectedScale) > 1) {
    add("scaled Sheet height", slotRect.height + " vs " + pageHeight * expectedScale);
  }
  if (slotRect.width > innerWidth + 0.5) {
    add("scaled Sheet width", slotRect.width);
  }

  return {
    pages: flows.length,
    paragraphs: measureLines.size,
    visibleParagraphs: visibleParagraphs.size,
    measureLines: measureLineCount,
    visibleLines: seenLines.size,
    duplicateLines,
    partialLines,
    outsideLines,
    wrapDrift,
    marginViolations,
    issues: issues.slice(0, 12),
    innerWidth,
    innerHeight,
    scale,
    expectedScale,
    slotWidth: slotRect.width,
    slotHeight: slotRect.height,
    horizontalOverflow: document.documentElement.scrollWidth > innerWidth + 1 || stage.scrollWidth > stage.clientWidth + 1,
    verticalOverflow: document.documentElement.scrollHeight > innerHeight + 1
  };
})()`
