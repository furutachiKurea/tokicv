package desktop

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func openLayoutProfile(t *testing.T, tt *ui.Tester) {
	t.Helper()
	if _, ok := tt.AccessByRole(ui.RolePopup, ""); !ok {
		if err := tt.Click("排版"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
}

func TestDocumentRailToggleMovesTheWorkbenchContinuously(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 1280, 800)
	tt.Frame()
	closed := accessRect(t, tt, ui.RoleMain, "简历编辑").W
	toggle, ok := tt.AccessByRole(ui.RoleButton, "文档栏")
	if !ok {
		t.Fatal("no document rail toggle")
	}
	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	tt.Frame()

	var widths []float32
	for range 6 {
		tt.Frame()
		widths = append(widths, accessRect(t, tt, ui.RoleMain, "简历编辑").W)
		tt.Advance(60 * time.Millisecond)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	open := accessRect(t, tt, ui.RoleMain, "简历编辑").W
	nearWidth(t, open, minEditorWidth+2)
	if open >= closed {
		t.Fatalf("editor width = %v with the rail open, want less than %v", open, closed)
	}
	for i := 1; i < len(widths); i++ {
		if widths[i] > widths[i-1] {
			t.Fatalf("editor widths are not continuous: %v", widths)
		}
	}
	rail := accessRect(t, tt, ui.RoleNavigation, "文档栏")
	nearWidth(t, rail.W, documentRailWidth)
}

func TestDocumentRailClosingLeavesAccessibilityBeforeTheVisualExit(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1280, 800)
	tt.Frame()
	openDocumentRail(t, tt)
	toggle, ok := tt.AccessByRole(ui.RoleButton, "文档栏")
	if !ok {
		t.Fatal("no document rail toggle")
	}
	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	tt.Frame()
	if app.railOpen {
		t.Fatal("the rail stayed open")
	}
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); ok {
		t.Fatal("the closing rail stays in the accessibility tree")
	}
	if _, distance := nearestPixelColor(tt, ui.Rect{X: 90, Y: 300, W: 20, H: 20}, sidebarTray); distance > 16 {
		t.Fatalf("the closing rail left a blank gap: color distance %d", distance)
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if _, distance := nearestPixelColor(tt, ui.Rect{X: 90, Y: 300, W: 20, H: 20}, sidebarTray); distance <= 16 {
		t.Fatal("the rail did not finish leaving")
	}
}

func TestDocumentRailOverlaySlidesWithoutReflowingTheWorkbench(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 1100, 800)
	tt.Frame()
	editorWidth := accessRect(t, tt, ui.RoleMain, "简历编辑").W
	toggle, ok := tt.AccessByRole(ui.RoleButton, "文档栏")
	if !ok {
		t.Fatal("no document rail toggle")
	}
	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	tt.Advance(40 * time.Millisecond)
	tt.Frame()

	var xs []float32
	for range 6 {
		rail := accessRect(t, tt, ui.RoleNavigation, "文档栏")
		xs = append(xs, rail.X)
		nearWidth(t, accessRect(t, tt, ui.RoleMain, "简历编辑").W, editorWidth)
		tt.Advance(60 * time.Millisecond)
		tt.Frame()
	}
	rail := accessRect(t, tt, ui.RoleNavigation, "文档栏")
	nearWidth(t, rail.X, 0)
	nearWidth(t, rail.W, documentRailWidth)
	for i := 1; i < len(xs); i++ {
		if xs[i] < xs[i-1] {
			t.Fatalf("overlay rail positions are not continuous: %v", xs)
		}
	}
}

func TestDocumentRailMotionHonorsReduceMotion(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 1280, 800)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	tt.Frame()
	toggle, ok := tt.AccessByRole(ui.RoleButton, "文档栏")
	if !ok {
		t.Fatal("no document rail toggle")
	}
	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	tt.Advance(time.Millisecond)
	tt.Frame()
	nearWidth(t, accessRect(t, tt, ui.RoleNavigation, "文档栏").W, documentRailWidth)
	nearWidth(t, accessRect(t, tt, ui.RoleMain, "简历编辑").W, minEditorWidth+2)

	toggle, _ = tt.AccessByRole(ui.RoleButton, "文档栏")
	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	tt.Advance(time.Millisecond)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); ok {
		t.Fatal("the rail stays in the accessibility tree with Reduce Motion")
	}
	nearWidth(t, accessRect(t, tt, ui.RoleMain, "简历编辑").W, 512)
}

func TestRailToggleKeepsThePreviewPageLoaded(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	_, preview, tt := newLibraryHarness(t, store, 1280, 800)
	tt.Frame()
	loads := len(preview.html)
	toggle, _ := tt.AccessByRole(ui.RoleButton, "文档栏")
	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	for range 8 {
		tt.Frame()
		if got := len(preview.html); got != loads {
			t.Fatalf("opening the rail loaded the preview: %d loads", got-loads)
		}
		tt.Advance(50 * time.Millisecond)
	}
	tt.Frame()
	toggle, _ = tt.AccessByRole(ui.RoleButton, "文档栏")
	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	for range 8 {
		tt.Frame()
		if got := len(preview.html); got != loads {
			t.Fatalf("closing the rail loaded the preview: %d loads", got-loads)
		}
		tt.Advance(50 * time.Millisecond)
	}
	tt.Frame()
	if got := len(preview.html); got != loads {
		t.Fatalf("toggling the rail loaded the preview: %d loads", got-loads)
	}
}

func TestLayoutPopoverEntersWithMotion(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("排版动效", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 1440, 900)
	tt.Frame()
	if err := tt.Click("排版"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	start, ok := tt.AccessByRole(ui.RolePopup, "")
	if !ok {
		t.Fatal("the Layout Profile popover did not open")
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	settled, ok := tt.AccessByRole(ui.RolePopup, "")
	if !ok {
		t.Fatal("the Layout Profile popover did not settle")
	}
	dy := settled.Bounds.Y - start.Bounds.Y
	if dy < 4 || dy > 8 {
		t.Fatalf("popover entered with %v DIPs of travel, want about 6", dy)
	}
}

func TestRenameDialogFadesItsBackdrop(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("重命名动效", []byte("# 示例文档\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	tt.Frame()
	openDocumentRail(t, tt)
	if err := tt.Click("重命名"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.renameOpen {
		t.Fatal("the rename dialog did not open")
	}
	before := tt.Image().RGBAAt(300, 300)
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	after := tt.Image().RGBAAt(300, 300)
	if after.R >= before.R {
		t.Fatalf("the dialog backdrop did not darken the editor: %v then %v", before, after)
	}
}
