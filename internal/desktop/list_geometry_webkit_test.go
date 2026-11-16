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
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"tokicv/internal/dialect"
	"tokicv/internal/render"
)

const listGeometryProbeMarker = "tokicv-list-geometry="

type listGeometryItem struct {
	TextLeft      float64   `json:"textLeft"`
	TextRight     float64   `json:"textRight"`
	MarkerLeft    float64   `json:"markerLeft"`
	MarkerWidth   float64   `json:"markerWidth"`
	MarkerHeight  float64   `json:"markerHeight"`
	MarkerGap     float64   `json:"markerGap"`
	MarkerCenterY float64   `json:"markerCenterY"`
	LineTop       float64   `json:"lineTop"`
	LineCenterY   float64   `json:"lineCenterY"`
	LineBottom    float64   `json:"lineBottom"`
	WrapLefts     []float64 `json:"wrapLefts"`
}

type listGeometryReport struct {
	Error  string           `json:"error,omitempty"`
	Level1 listGeometryItem `json:"level1"`
	Level2 listGeometryItem `json:"level2"`
	Issues []string         `json:"issues"`
}

func TestListMarkersKeepTextGeometryAndUseCalibratedSlotsUnderHeadlessLinux(t *testing.T) {
	if !cgoAvailable {
		t.Skip("WebKitGTK list geometry probe requires cgo")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runtimeDir, waylandDisplay := startHeadlessMutter(t)

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(environmentWithout("DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "GDK_BACKEND"),
		"TOKICV_LIST_GEOMETRY_PROBE=1",
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
		t.Fatalf("list geometry probe timed out: %v; stderr: %s", ctx.Err(), stderr.String())
	}
	if err != nil {
		t.Fatalf("list geometry probe failed: %v; stderr: %s", err, stderr.String())
	}

	var raw string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if value, ok := strings.CutPrefix(line, listGeometryProbeMarker); ok {
			raw = value
		}
	}
	if raw == "" {
		t.Fatalf("no list geometry result; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	}
	var report listGeometryReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode list geometry result: %v; result: %s", err, raw)
	}
	if report.Error != "" {
		t.Fatalf("list geometry probe: %s", report.Error)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("list geometry issues: %s", strings.Join(report.Issues, "; "))
	}
	for name, item := range map[string]listGeometryItem{"level1": report.Level1, "level2": report.Level2} {
		if math.Abs(item.MarkerWidth-5) > 0.1 || math.Abs(item.MarkerHeight-5) > 0.1 {
			t.Errorf("%s marker = %.2fx%.2f px, want 5x5 border-box", name, item.MarkerWidth, item.MarkerHeight)
		}
		if math.Abs(item.MarkerGap-10.7) > 0.15 {
			t.Errorf("%s marker gap = %.2f px, want 10.7", name, item.MarkerGap)
		}
		if len(item.WrapLefts) < 2 {
			t.Errorf("%s item did not wrap", name)
			continue
		}
		for _, left := range item.WrapLefts {
			if math.Abs(left-item.TextLeft) > 0.2 {
				t.Errorf("%s wrapped line left = %.2f, want hanging text start %.2f",
					name, left, item.TextLeft)
			}
		}
		if item.MarkerCenterY <= item.LineTop || item.MarkerCenterY >= item.LineBottom {
			t.Errorf("%s marker center y = %.2f, want it inside the first line %.2f-%.2f",
				name, item.MarkerCenterY, item.LineTop, item.LineBottom)
		}
	}
}

func runListGeometryProbe() int {
	mygo.App.SetName("tokicv-list-geometry")
	done := make(chan listGeometryReport, 1)
	mygo.App.WhenReady(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			done <- inspectListGeometry(ctx)
			mygo.App.Quit()
		}()
	})

	runErr := mygo.App.Run()
	var report listGeometryReport
	select {
	case report = <-done:
	case <-time.After(5 * time.Second):
		report.Error = "the list geometry probe did not finish"
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal list geometry: %v\n", err)
		return 1
	}
	fmt.Printf("%s%s\n", listGeometryProbeMarker, data)
	return 0
}

func inspectListGeometry(ctx context.Context) listGeometryReport {
	const source = `# 列表校准

- 一级列表正文起点需要保持固定，并在较长内容换行时维持悬挂缩进。This is a deliberately long first list item that must wrap at least twice.
  - 二级列表正文起点保持固定，并在较长内容换行时维持悬挂缩进。This nested item is also deliberately long enough to wrap onto another visual line.
`
	win := mygo.NewWindow(mygo.WindowOptions{
		Hidden:          true,
		Width:           1280,
		Height:          900,
		UseContentSize:  true,
		BackgroundColor: "#ffffff",
		Page: mygo.PageOptions{
			BackgroundThrottling: mygo.ThrottlingOff,
			Untrusted:            true,
		},
	})
	defer win.Destroy()
	win.Page().LoadHTML(render.RenderWithLayout(dialect.Parse(source), render.DefaultLayout()), "")
	if err := waitForProbeReady(ctx, win); err != nil {
		return listGeometryReport{Error: err.Error()}
	}
	report, err := mygo.EvalAs[listGeometryReport](win.Page(), listGeometryScript)
	if err != nil {
		return listGeometryReport{Error: err.Error()}
	}
	if report.Issues == nil {
		report.Issues = []string{}
	}
	return report
}

func waitForProbeReady(ctx context.Context, win *mygo.Window) error {
	for {
		value, err := win.Page().EvalContext(ctx, `document.fonts.status === "loaded" && !!document.querySelector(".resume-list > li")`)
		if err == nil {
			if ready, _ := value.(bool); ready {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

const listGeometryScript = `(() => {
  const issues = [];
  const firstText = (element) => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      if (node.nodeValue.trim()) return node;
    }
    return null;
  };
  const measure = (li) => {
    const pseudo = getComputedStyle(li, "::before");
    const box = li.getBoundingClientRect();
    const left = parseFloat(pseudo.left) || 0;
    const width = parseFloat(pseudo.width) || 0;
    const height = parseFloat(pseudo.height) || 0;
    const top = parseFloat(pseudo.top) || 0;
    const border = parseFloat(pseudo.borderTopWidth) || 0;
    const outerWidth = pseudo.boxSizing === "border-box" ? width : width + border * 2;
    const outerHeight = pseudo.boxSizing === "border-box" ? height : height + border * 2;
    const node = firstText(li);
    if (!node) {
      issues.push("list item has no text");
      return {};
    }
    const range = document.createRange();
    range.selectNodeContents(node);
    const rects = Array.from(range.getClientRects()).filter(rect => rect.width > 0 && rect.height > 0);
    if (!rects.length) {
      issues.push("list item has no text rect");
      return {};
    }
    return {
      textLeft: rects[0].left,
      textRight: rects[0].right,
      markerLeft: box.left + left,
      markerWidth: outerWidth,
      markerHeight: outerHeight,
      markerGap: rects[0].left - (box.left + left + outerWidth),
      markerCenterY: box.top + top + outerHeight / 2,
      lineTop: rects[0].top,
      lineCenterY: (rects[0].top + rects[0].bottom) / 2,
      lineBottom: rects[0].bottom,
      wrapLefts: rects.map(rect => rect.left)
    };
  };
  const level1 = document.querySelector(".resume-list.level-1 > li");
  const level2 = document.querySelector(".resume-list.level-2 > li");
  if (!level1 || !level2) {
    issues.push("missing nested list");
    return {issues};
  }
  return {level1: measure(level1), level2: measure(level2), issues};
})()`
