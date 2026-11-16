package desktop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"tokicv/internal/export"
	"tokicv/internal/library"
	"tokicv/internal/render"
)

func TestLayoutPopoverPersistsProfilesWithoutTouchingMarkdown(t *testing.T) {
	store := openDesktopStore(t)
	source := []byte("# 示例文档\r\n\r\n- 原始字节\r\n")
	document, err := store.Create("排版验证", source)
	if err != nil {
		t.Fatal(err)
	}
	app, preview, tt := newLibraryHarness(t, store, 1440, 900)
	if tt.HasText("设为全局默认") {
		t.Fatal("the Layout Profile popover is open by default")
	}

	openLayoutProfile(t, tt)
	for _, label := range []string{"衬线", "分节间距", "条目间距", "上边距", "下边距", "行高", "设为全局默认", "设为当前简历", "清除当前覆盖"} {
		if !tt.HasText(label) {
			t.Errorf("Layout Profile popover does not show %q", label)
		}
	}
	for _, forbidden := range []string{"字号", "字体大小", "Font Size"} {
		if tt.HasText(forbidden) {
			t.Errorf("Layout Profile popover exposes the font-size control %q", forbidden)
		}
	}

	if err := tt.Click("衬线"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.baseLayout.FontStack != render.FontSerif || app.layout.FontStack != render.FontSerif {
		t.Fatalf("font stack = %q / %q, want serif", app.baseLayout.FontStack, app.layout.FontStack)
	}
	if len(preview.html) == 0 || !strings.Contains(preview.html[len(preview.html)-1], `"Noto Serif CJK SC"`) {
		t.Fatal("changing the font stack did not refresh the DOM preview")
	}

	if err := clickAccessible(tt, ui.RoleButton, "降低行高"); err != nil {
		t.Fatal(err)
	}
	if got := app.baseLayout.LineHeight; got < render.MinLineHeight || got >= 1.40 {
		t.Fatalf("line height = %v, want one readable step below 1.40", got)
	}
	activeLineHeight := app.baseLayout.LineHeight
	if got := preview.html[len(preview.html)-1]; !strings.Contains(got, "line-height:") {
		t.Fatal("changing line height did not refresh the DOM preview")
	}

	if err := tt.Click("设为当前简历"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	overrides, err := store.LoadLayoutOverrides()
	if err != nil {
		t.Fatal(err)
	}
	documentProfile, ok := overrides.Documents[document.ID]
	if !ok {
		t.Fatal("saving did not create a per-document Layout Profile")
	}
	if documentProfile.FontStack != render.FontSerif || documentProfile.LineHeight != activeLineHeight {
		t.Fatalf("document profile = %#v", documentProfile)
	}
	if app.layoutOrigin != library.LayoutOriginDocument {
		t.Fatalf("layout origin = %q, want document", app.layoutOrigin)
	}

	if err := tt.Click("设为全局默认"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	overrides, err = store.LoadLayoutOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if overrides.Default == nil || *overrides.Default != documentProfile {
		t.Fatalf("global default = %#v, want %#v", overrides.Default, documentProfile)
	}

	if err := tt.Click("清除当前覆盖"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	overrides, err = store.LoadLayoutOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := overrides.Documents[document.ID]; ok {
		t.Fatal("clearing the override left the per-document profile in layouts.json")
	}
	if app.layoutOrigin != library.LayoutOriginGlobal || app.baseLayout != documentProfile.Normalized() {
		t.Fatalf("after clear: origin = %q, layout = %#v", app.layoutOrigin, app.baseLayout)
	}

	_, current, err := store.Show(document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, source) {
		t.Fatalf("layout persistence changed Markdown bytes: %q", current)
	}
}

func TestLayoutPopoverFieldsStayInsidePanelWithoutOverlap(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("排版浮层", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 1440, 900)
	openLayoutProfile(t, tt)

	panel, ok := tt.AccessByRole(ui.RolePopup, "")
	if !ok {
		t.Fatal("the Layout Profile popover has no accessible panel")
	}
	numberLabels := []string{"分节间距 (px)", "条目间距 (px)", "上边距 (mm)", "下边距 (mm)", "行高"}
	labels := append([]string{"字体"}, numberLabels...)
	type namedRect struct {
		name string
		rect ui.Rect
	}
	var rects []namedRect
	for _, label := range labels {
		info, ok := tt.AccessByRole(ui.RoleText, label)
		if !ok {
			t.Fatalf("Layout Profile popover does not show label %q", label)
		}
		rects = append(rects, namedRect{name: label, rect: info.Bounds})
		_, toMuted := nearestPixelColor(tt, info.Bounds, mutedGraphite)
		_, toInk := nearestPixelColor(tt, info.Bounds, carbonInk)
		if min(toMuted, toInk) > 8 {
			t.Errorf("label %q does not use an accessible text color", label)
		}
	}
	for _, label := range numberLabels {
		info, ok := tt.AccessByRole(ui.RoleTextField, label)
		if !ok {
			t.Fatalf("Layout Profile popover has no number input for %q", label)
		}
		rects = append(rects, namedRect{name: label + " input", rect: info.Bounds})
	}

	steps := 0
	otherControls := map[string]bool{
		"经典": true, "思源": true, "衬线": true, "系统": true,
		"设为全局默认": true, "设为当前简历": true,
		"清除当前覆盖": true, "恢复默认": true,
	}
	seenControls := make(map[string]bool, len(otherControls))
	for _, info := range tt.AccessAll() {
		if info.Role == ui.RoleButton &&
			(strings.HasPrefix(info.Name, "增加") || strings.HasPrefix(info.Name, "减少") ||
				strings.HasPrefix(info.Name, "提高") || strings.HasPrefix(info.Name, "降低")) {
			rects = append(rects, namedRect{name: info.Name, rect: info.Bounds})
			steps++
		}
		if otherControls[info.Name] && (info.Role == ui.RoleButton || info.Role == ui.RoleRadio) {
			rects = append(rects, namedRect{name: info.Name, rect: info.Bounds})
			seenControls[info.Name] = true
		}
	}
	if steps != 2*len(numberLabels) {
		t.Fatalf("Layout Profile popover step buttons = %d, want %d", steps, 2*len(numberLabels))
	}
	for name := range otherControls {
		if !seenControls[name] {
			t.Errorf("Layout Profile popover has no %q control", name)
		}
	}

	const epsilon = 0.5
	for _, item := range rects {
		rect := item.rect
		if rect.W <= epsilon || rect.H <= epsilon {
			t.Errorf("%s has no visible area: %v", item.name, rect)
		}
		if rect.X < panel.Bounds.X-epsilon || rect.Y < panel.Bounds.Y-epsilon ||
			rect.X+rect.W > panel.Bounds.X+panel.Bounds.W+epsilon ||
			rect.Y+rect.H > panel.Bounds.Y+panel.Bounds.H+epsilon {
			t.Errorf("%s at %v is outside the popover %v", item.name, rect, panel.Bounds)
		}
	}
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			a, b := rects[i], rects[j]
			overlapX := min(a.rect.X+a.rect.W, b.rect.X+b.rect.W) - max(a.rect.X, b.rect.X)
			overlapY := min(a.rect.Y+a.rect.H, b.rect.Y+b.rect.H) - max(a.rect.Y, b.rect.Y)
			if overlapX > epsilon && overlapY > epsilon {
				t.Errorf("%s at %v overlaps %s at %v", a.name, a.rect, b.name, b.rect)
			}
		}
	}
}

func TestLayoutPopoverActionsRemainReadableAtMinimumWindowSize(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("窄窗口排版", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 960, 640)
	openLayoutProfile(t, tt)

	panel, ok := tt.AccessByRole(ui.RolePopup, "")
	if !ok {
		t.Fatal("the Layout Profile popover has no accessible panel")
	}
	if panel.Bounds.X < 0 || panel.Bounds.Y < 0 ||
		panel.Bounds.X+panel.Bounds.W > 960 ||
		panel.Bounds.Y+panel.Bounds.H > 640 {
		t.Fatalf("Layout Profile popover is outside the minimum window: %v", panel.Bounds)
	}

	for _, label := range []string{"设为全局默认", "设为当前简历", "清除当前覆盖", "恢复默认"} {
		button, ok := tt.AccessByRole(ui.RoleButton, label)
		if !ok {
			t.Errorf("missing Layout Profile action %q", label)
			continue
		}
		if button.Bounds.W < 140 {
			t.Errorf("Layout Profile action %q is too narrow to show its label: %v", label, button.Bounds)
		}
		if button.Bounds.X < panel.Bounds.X || button.Bounds.Y < panel.Bounds.Y ||
			button.Bounds.X+button.Bounds.W > panel.Bounds.X+panel.Bounds.W ||
			button.Bounds.Y+button.Bounds.H > panel.Bounds.Y+panel.Bounds.H {
			t.Errorf("Layout Profile action %q is outside the popover: %v", label, button.Bounds)
		}
	}
}

func TestExportMenuUsesNativeMenuInteraction(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("导出菜单", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1280, 800)
	for _, kind := range []ArtifactKind{ArtifactPDF, ArtifactPNG, ArtifactMarkdown} {
		app.SetExportPath(kind, func(string) (string, error) { return "", nil })
	}
	tt.Frame()

	if err := tt.Click("导出"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.exportMenuOpen {
		t.Fatal("the export menu did not open")
	}
	if _, ok := tt.AccessByRole(ui.RoleMenuButton, "导出"); !ok {
		t.Fatal("the export trigger does not expose menu-button semantics")
	}
	for _, label := range []string{"导出 PDF", "导出 PNG", "导出 Markdown"} {
		if _, ok := tt.AccessByRole(ui.RoleMenuItem, label); !ok {
			t.Errorf("export menu item %q does not expose menu-item semantics", label)
		}
	}
}

func TestLayoutProfileCoarseButtonsKeepManualFineValues(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("排版步进", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	openLayoutProfile(t, tt)

	steps := []struct {
		button string
		want   func(render.Layout) float64
	}{
		{"增加分节间距 (px)", func(layout render.Layout) float64 { return layout.SectionGapPX }},
		{"减少分节间距 (px)", func(layout render.Layout) float64 { return layout.SectionGapPX }},
		{"增加条目间距 (px)", func(layout render.Layout) float64 { return layout.ItemGapPX }},
		{"减少条目间距 (px)", func(layout render.Layout) float64 { return layout.ItemGapPX }},
		{"增加上边距 (mm)", func(layout render.Layout) float64 { return layout.MarginTopMM }},
		{"减少上边距 (mm)", func(layout render.Layout) float64 { return layout.MarginTopMM }},
		{"增加下边距 (mm)", func(layout render.Layout) float64 { return layout.MarginBottomMM }},
		{"减少下边距 (mm)", func(layout render.Layout) float64 { return layout.MarginBottomMM }},
		{"提高行高", func(layout render.Layout) float64 { return layout.LineHeight }},
		{"降低行高", func(layout render.Layout) float64 { return layout.LineHeight }},
	}
	var before float64
	for index, step := range steps {
		if index%2 == 0 {
			before = step.want(app.baseLayout)
		}
		if err := clickAccessible(tt, ui.RoleButton, step.button); err != nil {
			t.Fatalf("%s: %v", step.button, err)
		}
		tt.Frame()
		got := step.want(app.baseLayout)
		if index%2 == 0 {
			if got <= before {
				t.Fatalf("%s did not apply its coarse step: %v -> %v", step.button, before, got)
			}
			continue
		}
		if got != before {
			t.Fatalf("%s did not restore %v: got %v", step.button, before, got)
		}
	}

	for _, tc := range []struct {
		label string
		value string
		want  float64
		read  func(render.Layout) float64
	}{
		{"分节间距 (px)", "7.5", 7.5, func(layout render.Layout) float64 { return layout.SectionGapPX }},
		{"条目间距 (px)", "1.5", 1.5, func(layout render.Layout) float64 { return layout.ItemGapPX }},
		{"上边距 (mm)", "7.5", 7.5, func(layout render.Layout) float64 { return layout.MarginTopMM }},
		{"下边距 (mm)", "7.5", 7.5, func(layout render.Layout) float64 { return layout.MarginBottomMM }},
		{"行高", "1.25", 1.25, func(layout render.Layout) float64 { return layout.LineHeight }},
	} {
		field, ok := tt.AccessByRole(ui.RoleTextField, tc.label)
		if !ok {
			t.Fatalf("no text field for %s", tc.label)
		}
		tt.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(tc.value)
		tt.Frame()
		if got := tc.read(app.baseLayout); got != tc.want {
			t.Fatalf("%s manual value = %v, want %v", tc.label, got, tc.want)
		}
	}
}

func TestLayoutProfileLineHeightStepsAvoidFloatDrift(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("行高步进", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	openLayoutProfile(t, tt)

	steps := []struct {
		button string
		want   float64
		text   string
	}{
		{button: "降低行高", want: 1.35, text: "1.35"},
		{button: "降低行高", want: 1.30, text: "1.3"},
		{button: "提高行高", want: 1.35, text: "1.35"},
	}
	for _, step := range steps {
		if err := clickAccessible(tt, ui.RoleButton, step.button); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := app.baseLayout.LineHeight; got != step.want {
			t.Fatalf("%s: line height = %v, want %v", step.button, got, step.want)
		}
		field, ok := tt.AccessByRole(ui.RoleTextField, "行高")
		if !ok {
			t.Fatal("no line-height text field")
		}
		if field.Value != step.text {
			t.Fatalf("%s: line-height field = %q, want %q", step.button, field.Value, step.text)
		}
	}
}

func clickAccessible(tt *ui.Tester, role ui.Role, name string) error {
	info, ok := tt.AccessByRole(role, name)
	if !ok {
		var names []string
		for _, candidate := range tt.AccessAll() {
			if candidate.Role == role {
				names = append(names, candidate.Name)
			}
		}
		return fmt.Errorf("ui: no accessible %v named %q; available: %v", role, name, names)
	}
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	return nil
}

type fakeArtifactPipeline struct {
	mu sync.Mutex

	fitCalls     int
	fitHTML      []string
	fitBase      []render.Layout
	fitResult    export.OnePageFit
	fitErr       error
	fitStarted   chan struct{}
	fitRelease   chan struct{}
	pdfCalls     int
	pngCalls     int
	markdownCall int
	pdfHTML      string
	pngHTML      string
	markdown     []byte
	exportErr    error
}

func (p *fakeArtifactPipeline) FitOnePage(ctx context.Context, html string, base render.Layout) (export.OnePageFit, error) {
	p.mu.Lock()
	p.fitCalls++
	p.fitHTML = append(p.fitHTML, html)
	p.fitBase = append(p.fitBase, base)
	started := p.fitStarted
	release := p.fitRelease
	result := p.fitResult
	err := p.fitErr
	p.mu.Unlock()
	if started != nil {
		select {
		case <-started:
		default:
			close(started)
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return export.OnePageFit{}, ctx.Err()
		}
	}
	return result, err
}

func (p *fakeArtifactPipeline) WritePDF(_ context.Context, html, path string) error {
	p.mu.Lock()
	p.pdfCalls++
	p.pdfHTML = html
	err := p.exportErr
	p.mu.Unlock()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte("%PDF-1.7\nstub"), 0o644)
}

func (p *fakeArtifactPipeline) WritePNG(_ context.Context, html, path string) error {
	p.mu.Lock()
	p.pngCalls++
	p.pngHTML = html
	err := p.exportErr
	p.mu.Unlock()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte("\x89PNG\r\n\x1a\nstub"), 0o644)
}

func (p *fakeArtifactPipeline) WriteMarkdown(_ context.Context, source []byte, path string) error {
	p.mu.Lock()
	p.markdownCall++
	p.markdown = append([]byte(nil), source...)
	err := p.exportErr
	p.mu.Unlock()
	if err != nil {
		return err
	}
	return os.WriteFile(path, source, 0o644)
}

func (p *fakeArtifactPipeline) calls() (fit, pdf, png, markdown int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fitCalls, p.pdfCalls, p.pngCalls, p.markdownCall
}

func TestSmartOnePageRecalculatesAfterEditingAndRestoresTheBaseline(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("智能一页", []byte("# 示例文档\n\n## 技能\n\n- Go\n")); err != nil {
		t.Fatal(err)
	}
	app, preview, tt := newLibraryHarness(t, store, 1440, 900)
	base := app.baseLayout
	tight := base
	tight.SectionGapPX = 8
	tight.ItemGapPX = 3
	pipeline := &fakeArtifactPipeline{
		fitResult: export.OnePageFit{Layout: tight.Normalized(), NumPages: 1},
	}
	app.SetArtifactPipeline(pipeline)
	tt.Frame()

	if err := tt.Click("智能一页"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	fit, _, _, _ := pipeline.calls()
	if fit != 1 {
		t.Fatalf("Smart One-Page solves = %d, want 1", fit)
	}
	if !app.smartOnePage || app.layout != tight.Normalized() {
		t.Fatalf("smart = %v, layout = %#v, want tightened %#v", app.smartOnePage, app.layout, tight.Normalized())
	}
	tt.Advance(200 * time.Millisecond)
	tt.Frame()
	toggleBounds, ok := tt.Find("智能一页")
	if !ok {
		t.Fatal("no Smart One-Page toggle")
	}
	if _, distance := nearestPixelColor(tt, toggleBounds, editorialCobalt); distance > 8 {
		t.Errorf("enabled Smart One-Page toggle is not cobalt: distance %d", distance)
	}
	if got := preview.html[len(preview.html)-1]; !strings.Contains(got, "8px") {
		t.Fatal("the tightened profile did not reach the DOM preview")
	}
	openLayoutProfile(t, tt)
	for _, tc := range []struct {
		label string
		want  string
	}{
		{"分节间距 (px)", "8"},
		{"条目间距 (px)", "3"},
	} {
		field, ok := tt.AccessByRole(ui.RoleTextField, tc.label)
		if !ok {
			t.Fatalf("no text field for %s", tc.label)
		}
		if field.Value != tc.want {
			t.Errorf("%s field = %q, want the Smart One-Page result %q", tc.label, field.Value, tc.want)
		}
	}

	tt.Click("Markdown 源文")
	tt.Type("\n继续编辑")
	tt.Advance(PreviewDebounce)
	tt.Frame()
	drainFileIO(t, app, tt)
	fit, _, _, _ = pipeline.calls()
	if fit != 2 {
		t.Fatalf("typing ran %d Smart One-Page solves, want an automatic refit", fit)
	}
	if !app.smartOnePage || app.layout != tight.Normalized() {
		t.Fatalf("automatic refit left enabled=%v layout=%#v", app.smartOnePage, app.layout)
	}

	if err := tt.Click("智能一页"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.smartOnePage || app.layout != base {
		t.Fatalf("turning off smart left enabled=%v layout=%#v", app.smartOnePage, app.layout)
	}
	if got := preview.html[len(preview.html)-1]; !strings.Contains(got, "12px") {
		t.Fatal("turning off Smart One-Page did not restore the base Layout Profile")
	}
}

func TestLayoutFieldsExposeAndEditTheSmartOnePageResult(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("智能一页结果编辑", []byte("# 示例文档\n\n## 技能\n\n- Go\n")); err != nil {
		t.Fatal(err)
	}
	app, preview, tt := newLibraryHarness(t, store, 1440, 900)
	tight := app.baseLayout
	tight.SectionGapPX = 8
	tight.ItemGapPX = 3
	pipeline := &fakeArtifactPipeline{
		fitResult: export.OnePageFit{Layout: tight.Normalized(), NumPages: 1},
	}
	app.SetArtifactPipeline(pipeline)
	tt.Frame()

	if err := tt.Click("智能一页"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	openLayoutProfile(t, tt)

	field, ok := tt.AccessByRole(ui.RoleTextField, "分节间距 (px)")
	if !ok {
		t.Fatal("no section spacing text field")
	}
	if field.Value != "8" {
		t.Fatalf("section spacing field = %q, want the Smart One-Page result", field.Value)
	}
	tt.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("9")
	tt.Frame()
	if app.smartOnePage {
		t.Error("manual layout editing left Smart One-Page active")
	}
	edited := tight
	edited.SectionGapPX = 9
	if app.baseLayout != edited.Normalized() || app.layout != edited.Normalized() {
		t.Fatalf("manual edit layout = %#v / %#v, want the edited Smart result %#v",
			app.baseLayout, app.layout, edited.Normalized())
	}
	if got := preview.html[len(preview.html)-1]; !strings.Contains(got, "9px") {
		t.Fatal("the edited Smart result did not reach the DOM preview")
	}
}

func TestSavingSmartOnePagePersistsTheVisibleLayoutAsTheBaseline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		saveLabel string
		readSaved func(t *testing.T, store *library.Store, documentID string) render.Layout
	}{
		{
			name:      "current document",
			saveLabel: "设为当前简历",
			readSaved: func(t *testing.T, store *library.Store, documentID string) render.Layout {
				t.Helper()
				overrides, err := store.LoadLayoutOverrides()
				if err != nil {
					t.Fatal(err)
				}
				layout, ok := overrides.Documents[documentID]
				if !ok {
					t.Fatal("Smart One-Page save did not persist the current document profile")
				}
				return layout
			},
		},
		{
			name:      "global default",
			saveLabel: "设为全局默认",
			readSaved: func(t *testing.T, store *library.Store, _ string) render.Layout {
				t.Helper()
				overrides, err := store.LoadLayoutOverrides()
				if err != nil {
					t.Fatal(err)
				}
				if overrides.Default == nil {
					t.Fatal("Smart One-Page save did not persist the global default")
				}
				return *overrides.Default
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openDesktopStore(t)
			document, err := store.Create("保存智能一页", []byte("# 示例文档\n\n## 技能\n\n- Go\n"))
			if err != nil {
				t.Fatal(err)
			}
			app, preview, tt := newLibraryHarness(t, store, 1440, 900)
			base := app.baseLayout
			if base.SectionGapPX != 12 {
				t.Fatalf("baseline section gap = %v, want 12", base.SectionGapPX)
			}
			tight := base
			tight.SectionGapPX = 8
			tight.ItemGapPX = 3
			tight = tight.Normalized()
			app.SetArtifactPipeline(&fakeArtifactPipeline{
				fitResult: export.OnePageFit{Layout: tight, NumPages: 1},
			})
			tt.Frame()

			if err := tt.Click("智能一页"); err != nil {
				t.Fatal(err)
			}
			drainFileIO(t, app, tt)
			if !app.smartOnePage || app.layout != tight {
				t.Fatalf("before save smart=%t layout=%#v, want active %#v",
					app.smartOnePage, app.layout, tight)
			}

			openLayoutProfile(t, tt)
			if err := tt.Click(tc.saveLabel); err != nil {
				t.Fatal(err)
			}
			drainFileIO(t, app, tt)

			if got := tc.readSaved(t, store, document.ID); got != tight {
				t.Fatalf("saved layout = %#v, want the visible layout %#v", got, tight)
			}
			if app.smartOnePage || app.smartSolving {
				t.Fatalf("save left smart=%t solving=%t", app.smartOnePage, app.smartSolving)
			}
			if app.baseLayout != tight || app.layout != tight {
				t.Fatalf("after save base/active = %#v / %#v, want %#v",
					app.baseLayout, app.layout, tight)
			}
			if got := preview.html[len(preview.html)-1]; !strings.Contains(got, "8px") {
				t.Fatal("the saved visible layout did not remain in the DOM preview")
			}

			if err := tt.Click("智能一页"); err != nil {
				t.Fatal(err)
			}
			drainFileIO(t, app, tt)
			if !app.smartOnePage || app.layout != tight {
				t.Fatalf("re-enabled smart state = %t / %#v", app.smartOnePage, app.layout)
			}
			if err := tt.Click("智能一页"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if app.smartOnePage || app.baseLayout != tight || app.layout != tight {
				t.Fatalf("disabling smart returned to a stale baseline: smart=%t base=%#v active=%#v",
					app.smartOnePage, app.baseLayout, app.layout)
			}

			if _, ok := tt.AccessByRole(ui.RoleTextField, "分节间距 (px)"); !ok {
				openLayoutProfile(t, tt)
			}
			field, ok := tt.AccessByRole(ui.RoleTextField, "分节间距 (px)")
			if !ok {
				t.Fatal("no section spacing text field")
			}
			tt.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("9")
			tt.Frame()
			if app.smartOnePage || app.layout.SectionGapPX != 9 {
				t.Fatalf("editing after save left smart=%t layout=%#v", app.smartOnePage, app.layout)
			}
			if err := tt.Click("恢复默认"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if app.smartOnePage || app.baseLayout != tight || app.layout != tight {
				t.Fatalf("reset after save = smart %t, base %#v, active %#v; want saved %#v",
					app.smartOnePage, app.baseLayout, app.layout, tight)
			}

			reopened, _, reopenedTT := newLibraryHarness(t, store, 1440, 900)
			reopenedTT.Frame()
			if reopened.smartOnePage || reopened.baseLayout != tight || reopened.layout != tight {
				t.Fatalf("reopened layout = smart %t, base %#v, active %#v; want saved %#v",
					reopened.smartOnePage, reopened.baseLayout, reopened.layout, tight)
			}
		})
	}
}

func TestSmartOnePageRecalculatesAfterEditingDuringAnInFlightSolve(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("智能一页旧结果", []byte("# 示例文档\n\n## 技能\n\n- Go\n")); err != nil {
		t.Fatal(err)
	}
	app, preview, tt := newLibraryHarness(t, store, 1440, 900)
	base := app.baseLayout
	tight := base
	tight.SectionGapPX = 8
	tight.ItemGapPX = 3
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFit := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFit)
	pipeline := &fakeArtifactPipeline{
		fitStarted: started,
		fitRelease: release,
		fitResult:  export.OnePageFit{Layout: tight.Normalized(), NumPages: 1},
	}
	app.SetArtifactPipeline(pipeline)
	tt.Frame()

	if err := tt.Click("智能一页"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Smart One-Page did not start")
	}
	if !app.smartSolving {
		t.Fatal("Smart One-Page is not solving after it started")
	}
	tt.Frame()
	if !tt.HasText("正在压缩为一页") {
		t.Error("the Smart One-Page control does not show its solving state")
	}

	before := app.buffer.Version()
	if err := tt.Click("Markdown 源文"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.SetClipboard("# 长稿\n\n" + strings.Repeat("## 经历\n\n- 新增的长条目\n\n", 30))
	tt.Key(ui.Cmd, ui.KeyV)
	if app.buffer.Version() == before {
		t.Fatal("pasting the long Resume Document did not change its version")
	}

	releaseFit()
	app.work.Wait()
	app.drainUpdates()
	tt.Frame()
	drainFileIO(t, app, tt)

	if app.smartSolving {
		t.Error("the stale Smart One-Page result stayed in progress")
	}
	if !app.smartOnePage {
		t.Error("the stale result disabled Smart One-Page instead of recalculating")
	}
	if app.layout != tight.Normalized() {
		t.Errorf("the automatic refit layout = %#v, want %#v", app.layout, tight.Normalized())
	}
	if app.smartCancel != nil {
		t.Error("the stale Smart One-Page request kept its cancel function")
	}
	fitCalls, _, _, _ := pipeline.calls()
	if fitCalls != 2 {
		t.Fatalf("Smart One-Page solves = %d, want the stale solve and its replacement", fitCalls)
	}
	app.ensureAnalysis()
	if got := preview.html[len(preview.html)-1]; !strings.Contains(got, "8px") {
		t.Error("the replacement fit did not reach the DOM preview")
	}
}

func TestSmartOnePageAutomaticRefitFallsBackWhenTheNewTextOverflows(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("智能一页重算溢出", []byte("# 示例文档\n\n## 技能\n\n- Go\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	base := app.baseLayout
	tight := base
	tight.SectionGapPX = 8
	tight.ItemGapPX = 3
	pipeline := &fakeArtifactPipeline{
		fitResult: export.OnePageFit{Layout: tight.Normalized(), NumPages: 1},
	}
	app.SetArtifactPipeline(pipeline)
	tt.Frame()
	if err := tt.Click("智能一页"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if !app.smartOnePage {
		t.Fatal("Smart One-Page did not enable before the edit")
	}

	pipeline.mu.Lock()
	pipeline.fitResult = export.OnePageFit{
		Layout:     base,
		NumPages:   3,
		Overflowed: true,
		Notice:     export.ReadabilityLimitNotice,
	}
	pipeline.mu.Unlock()

	tt.Click("Markdown 源文")
	tt.Type("\n\n" + strings.Repeat("- 新增内容\n", 20))
	tt.Advance(PreviewDebounce)
	tt.Frame()
	drainFileIO(t, app, tt)

	if app.smartOnePage {
		t.Error("an overflowing automatic refit stayed enabled")
	}
	if app.layout != base {
		t.Errorf("overflow fallback layout = %#v, want baseline %#v", app.layout, base)
	}
	if app.layoutNotice != "无法压缩成一页" {
		t.Fatalf("overflow notice = %q, want %q", app.layoutNotice, "无法压缩成一页")
	}
}

func TestSmartOnePageFailureReportsCauseAndLogsSafeError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		want    string
		wantLog string
	}{
		{
			name:    "timeout",
			err:     fmt.Errorf("等待页面: %w", context.DeadlineExceeded),
			want:    "智能一页失败（超时）",
			wantLog: "context deadline exceeded",
		},
		{
			name:    "print",
			err:     errors.New("打印 PDF: printer offline"),
			want:    "智能一页处理失败",
			wantLog: "<redacted>",
		},
		{
			name:    "other",
			err:     errors.New("render failed"),
			want:    "智能一页失败",
			wantLog: "<redacted>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openDesktopStore(t)
			if _, err := store.Create("失败诊断", []byte("# 示例文档\n")); err != nil {
				t.Fatal(err)
			}
			app, _, tt := newLibraryHarness(t, store, 1440, 900)
			app.SetArtifactPipeline(&fakeArtifactPipeline{fitErr: tc.err})
			tt.Frame()

			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(previous) })

			if err := tt.Click("智能一页"); err != nil {
				t.Fatal(err)
			}
			drainFileIO(t, app, tt)

			if app.fileError != tc.want {
				t.Fatalf("file error = %q, want %q", app.fileError, tc.want)
			}
			if app.layoutNotice != "" {
				t.Errorf("layout notice = %q, want empty", app.layoutNotice)
			}
			if !strings.Contains(logs.String(), tc.wantLog) {
				t.Errorf("log = %q, want safe diagnostic %q", logs.String(), tc.wantLog)
			}
			if !strings.Contains(logs.String(), "fit one page") {
				t.Errorf("log = %q, want the fit stage", logs.String())
			}
			alert, ok := tt.AccessByRole(ui.RoleAlert, tc.want)
			if !ok {
				t.Fatalf("no alert %q", tc.want)
			}
			if _, distance := nearestPixelColor(tt, alert.Bounds, proofVermilion); distance > 8 {
				t.Errorf("failure alert is not proof vermilion: distance %d", distance)
			}
		})
	}
}

func TestSmartOnePageStartFailureReportsBusyAndLogsSafeError(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("启动失败诊断", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	app.cancelWork()
	tt.Frame()

	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })

	if err := tt.Click("智能一页"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)

	if app.fileError != smartOnePageBusyMessage {
		t.Fatalf("file error = %q, want %q", app.fileError, smartOnePageBusyMessage)
	}
	if app.smartOnePage || app.smartSolving {
		t.Errorf("failed start left smart=%t solving=%t", app.smartOnePage, app.smartSolving)
	}
	if !strings.Contains(logs.String(), "<redacted>") {
		t.Errorf("log = %q, want redacted unknown error text", logs.String())
	}
	if !strings.Contains(logs.String(), "begin work") {
		t.Errorf("log = %q, want the begin stage", logs.String())
	}
	if _, ok := tt.AccessByRole(ui.RoleAlert, smartOnePageBusyMessage); !ok {
		t.Fatalf("no alert %q", smartOnePageBusyMessage)
	}
}

func TestDocumentSwitchLoadsDocumentPathAndGlobalLayoutProfiles(t *testing.T) {
	store := openDesktopStore(t)
	first, err := store.Create("第一篇", []byte("# 第一篇\n"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	second, err := store.Create("第二篇", []byte("# 第二篇\n"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	third, err := store.Create("全局回退", []byte("# 全局回退\n"))
	if err != nil {
		t.Fatal(err)
	}
	global := render.DefaultLayout()
	global.LineHeight = 1.5
	if err := store.SetGlobalDefault(global); err != nil {
		t.Fatal(err)
	}
	firstTarget, err := store.ResolveTarget(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstProfile := global
	firstProfile.FontStack = render.FontSerif
	firstProfile.SectionGapPX = 9
	if err := store.SetTargetProfile(firstTarget, firstProfile); err != nil {
		t.Fatal(err)
	}
	secondTarget, err := store.ResolveTarget(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondProfile := global
	secondProfile.FontStack = render.FontNoto
	secondProfile.SectionGapPX = 14
	if err := store.SetTargetProfile(secondTarget, secondProfile); err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), "path-override.md")
	if err := os.WriteFile(externalPath, []byte("# 外部\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pathTarget := library.Target{Kind: library.TargetFile, Path: externalPath}
	pathProfile := global
	pathProfile.FontStack = render.FontSystem
	pathProfile.ItemGapPX = 2
	if err := store.SetTargetProfile(pathTarget, pathProfile); err != nil {
		t.Fatal(err)
	}

	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	openDocumentRail(t, tt)
	if app.currentID != third.ID {
		t.Fatalf("initial document = %q, want %q", app.currentID, third.ID)
	}
	if app.baseLayout != global.Normalized() || app.layoutOrigin != library.LayoutOriginGlobal {
		t.Fatalf("global fallback = %#v from %q", app.baseLayout, app.layoutOrigin)
	}
	if err := tt.Click("第一篇"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if app.baseLayout != firstProfile.Normalized() || app.layoutOrigin != library.LayoutOriginDocument {
		t.Fatalf("document profile = %#v from %q", app.baseLayout, app.layoutOrigin)
	}
	if err := tt.Click("第二篇"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if app.baseLayout != secondProfile.Normalized() {
		t.Fatalf("second document profile = %#v", app.baseLayout)
	}
	app.SetPicker(func() (string, error) { return externalPath, nil })
	if err := tt.Click("打开 Markdown 文件"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if app.path != externalPath || app.baseLayout != pathProfile.Normalized() ||
		app.layoutOrigin != library.LayoutOriginPath {
		t.Fatalf("path profile = %#v from %q at %q", app.baseLayout, app.layoutOrigin, app.path)
	}
}

func TestSmartOnePageOverflowKeepsBaselineAndShowsOneReadableNotice(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("超限", []byte("# 示例文档\n\n## 技能\n\n- Go\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	base := app.baseLayout
	pipeline := &fakeArtifactPipeline{
		fitResult: export.OnePageFit{
			Layout:     base,
			NumPages:   2,
			Overflowed: true,
			Notice:     export.ReadabilityLimitNotice,
		},
	}
	app.SetArtifactPipeline(pipeline)
	tt.Frame()

	if err := tt.Click("智能一页"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if app.smartOnePage {
		t.Fatal("the unfittable Smart One-Page state stayed enabled")
	}
	if app.layout != base {
		t.Fatalf("overflow changed the baseline: %#v", app.layout)
	}
	if app.layoutNotice != "无法压缩成一页" {
		t.Fatalf("layout notice = %q, want %q", app.layoutNotice, "无法压缩成一页")
	}
	notice, ok := tt.AccessByRole(ui.RoleStatus, "无法压缩成一页")
	if !ok {
		t.Fatal("no readable-limit status")
	}
	if _, distance := nearestPixelColor(tt, notice.Bounds, proofVermilion); distance > 8 {
		t.Errorf("readable-limit status is not proof vermilion: distance %d", distance)
	}
	if strings.Count(strings.Join(tt.Texts(), "\n"), "无法压缩成一页") != 1 {
		t.Errorf("readable-limit notice is repeated: %q", tt.Texts())
	}
}

func TestExportActionWritesPDFPNGAndExactMarkdown(t *testing.T) {
	store := openDesktopStore(t)
	source := []byte("# 示例文档\r\n\r\n- 未保存的精确字节")
	document, err := store.Create("导出验证", source)
	if err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	pipeline := &fakeArtifactPipeline{}
	app.SetArtifactPipeline(pipeline)
	app.buffer.Set(app.buffer.String() + "\n\n- 当前缓冲区")
	tt.Frame()
	exportBounds, ok := tt.Find("导出")
	if !ok {
		t.Fatal("no export button")
	}
	if _, distance := nearestPixelColor(tt, exportBounds, editorialCobalt); distance > 8 {
		t.Errorf("primary export button is not cobalt: distance %d", distance)
	}

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "resume.pdf")
	pngPath := filepath.Join(dir, "resume.png")
	markdownPath := filepath.Join(dir, "resume.md")
	defaults := make(map[ArtifactKind]string)
	app.SetExportPath(ArtifactPDF, func(name string) (string, error) {
		defaults[ArtifactPDF] = name
		return pdfPath, nil
	})
	app.SetExportPath(ArtifactPNG, func(name string) (string, error) {
		defaults[ArtifactPNG] = name
		return pngPath, nil
	})
	app.SetExportPath(ArtifactMarkdown, func(name string) (string, error) {
		defaults[ArtifactMarkdown] = name
		return markdownPath, nil
	})

	for _, tc := range []struct {
		option string
		call   func() int
	}{
		{"导出 PDF", func() int { return pipeline.pdfCalls }},
		{"导出 PNG", func() int { return pipeline.pngCalls }},
		{"导出 Markdown", func() int { return pipeline.markdownCall }},
	} {
		if err := tt.Click("导出"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if err := tt.Click(tc.option); err != nil {
			t.Fatal(err)
		}
		drainFileIO(t, app, tt)
		if tc.call() != 1 {
			t.Fatalf("%s did not invoke its export action", tc.option)
		}
	}

	if defaults[ArtifactPDF] != "导出验证.pdf" || defaults[ArtifactPNG] != "导出验证.png" ||
		defaults[ArtifactMarkdown] != "导出验证.md" {
		t.Fatalf("export default names = %#v", defaults)
	}
	if !strings.Contains(pipeline.pdfHTML, "示例文档") || !strings.Contains(pipeline.pngHTML, "示例文档") {
		t.Fatal("PDF/PNG export did not use the current rendered HTML")
	}
	wantMarkdown := []byte(app.buffer.String())
	if !bytes.Equal(pipeline.markdown, wantMarkdown) {
		t.Fatalf("Markdown export = %q, want exact current bytes %q", pipeline.markdown, wantMarkdown)
	}
	for _, path := range []string{pdfPath, pngPath, markdownPath} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was not written: %v", path, err)
		}
	}
	got, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, wantMarkdown) {
		t.Fatalf("Markdown Artifact = %q, want %q", got, wantMarkdown)
	}
	_ = document
}

func TestExportFailureShowsOneProofVermilionSignal(t *testing.T) {
	app, _, tt := newHarness(t, "# 示例文档\n")
	pipeline := &fakeArtifactPipeline{exportErr: context.DeadlineExceeded}
	app.SetArtifactPipeline(pipeline)
	app.path = filepath.Join(t.TempDir(), "resume.md")
	app.document++
	app.SetExportPath(ArtifactPDF, func(string) (string, error) {
		return filepath.Join(t.TempDir(), "resume.pdf"), nil
	})
	tt.Frame()

	if err := tt.Click("导出"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("导出 PDF"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	alert, ok := tt.AccessByRole(ui.RoleAlert, "导出失败")
	if !ok {
		t.Fatal("no export failure alert")
	}
	if _, distance := nearestPixelColor(tt, alert.Bounds, proofVermilion); distance > 8 {
		t.Errorf("export failure is not proof vermilion: distance %d", distance)
	}
	if strings.Count(strings.Join(tt.Texts(), "\n"), "导出失败") != 1 {
		t.Errorf("export failure signal is repeated: %q", tt.Texts())
	}
}

func TestExportRunsOffTheUIThread(t *testing.T) {
	app, _, tt := newHarness(t, "# 示例文档\n")
	started := make(chan struct{})
	release := make(chan struct{})
	pipeline := &blockingPDFPipeline{started: started, release: release}
	app.SetArtifactPipeline(pipeline)
	app.path = filepath.Join(t.TempDir(), "resume.md")
	app.document++
	app.SetExportPath(ArtifactPDF, func(string) (string, error) {
		return filepath.Join(t.TempDir(), "resume.pdf"), nil
	})
	tt.Frame()
	if err := tt.Click("导出"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	clicked := make(chan error, 1)
	go func() { clicked <- tt.Click("导出 PDF") }()
	select {
	case err := <-clicked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("the export click blocked the UI thread")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the export did not start in the background")
	}
	close(release)
	drainFileIO(t, app, tt)
}

type blockingPDFPipeline struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingPDFPipeline) FitOnePage(context.Context, string, render.Layout) (export.OnePageFit, error) {
	return export.OnePageFit{}, nil
}

func (p *blockingPDFPipeline) WritePDF(ctx context.Context, _, path string) error {
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	select {
	case <-p.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return os.WriteFile(path, []byte("%PDF-1.7\nstub"), 0o644)
}

func (p *blockingPDFPipeline) WritePNG(context.Context, string, string) error { return nil }

func (p *blockingPDFPipeline) WriteMarkdown(context.Context, []byte, string) error { return nil }
