package desktop

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func accessRect(t *testing.T, tt *ui.Tester, role ui.Role, name string) ui.Rect {
	t.Helper()
	info, ok := tt.AccessByRole(role, name)
	if !ok {
		t.Fatalf("no %v %q", role, name)
	}
	return info.Bounds
}

func nearWidth(t *testing.T, got, want float32) {
	t.Helper()
	if math.Abs(float64(got-want)) > 1 {
		t.Fatalf("width = %v, want %v", got, want)
	}
}

func TestWorkbenchSplitDragsBetweenReadableBounds(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 1440, 900)
	tt.Frame()

	editor := accessRect(t, tt, ui.RoleMain, "简历编辑")
	preview, ok := tt.Find("A4 预览")
	if !ok {
		t.Fatal("no A4 preview")
	}
	nearWidth(t, editor.W, 576)
	nearWidth(t, preview.W, 863)

	splitter := accessRect(t, tt, ui.RoleSplitter, "调整编辑区与预览区宽度")
	cy := splitter.Y + splitter.H/2
	tt.Press(splitter.X+splitter.W/2, cy)
	tt.Move(0, cy)
	tt.Release(0, cy)
	tt.Frame()
	editor = accessRect(t, tt, ui.RoleMain, "简历编辑")
	preview, _ = tt.Find("A4 预览")
	nearWidth(t, editor.W, minEditorWidth)
	if preview.W < minPreviewWidth {
		t.Fatalf("preview width = %v, want at least %v", preview.W, minPreviewWidth)
	}

	splitter = accessRect(t, tt, ui.RoleSplitter, "调整编辑区与预览区宽度")
	tt.Press(splitter.X+splitter.W/2, cy)
	tt.Move(1439, cy)
	tt.Release(1439, cy)
	tt.Frame()
	editor = accessRect(t, tt, ui.RoleMain, "简历编辑")
	preview, _ = tt.Find("A4 预览")
	nearWidth(t, preview.W, minPreviewWidth)
	if editor.W < minEditorWidth {
		t.Fatalf("editor width = %v, want at least %v", editor.W, minEditorWidth)
	}

	splitter = accessRect(t, tt, ui.RoleSplitter, "调整编辑区与预览区宽度")
	tt.ClickAt(splitter.X+splitter.W/2, cy)
	tt.ClickAt(splitter.X+splitter.W/2, cy)
	tt.Frame()
	editor = accessRect(t, tt, ui.RoleMain, "简历编辑")
	nearWidth(t, editor.W, 576)
}

func TestWorkbenchSplitMovesWithTheKeyboard(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 1440, 900)
	tt.Frame()

	splitter := accessRect(t, tt, ui.RoleSplitter, "调整编辑区与预览区宽度")
	tt.ClickAt(splitter.X+splitter.W/2, splitter.Y+splitter.H/2)
	tt.Frame()
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	editor := accessRect(t, tt, ui.RoleMain, "简历编辑")
	nearWidth(t, editor.W, 592)
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	editor = accessRect(t, tt, ui.RoleMain, "简历编辑")
	nearWidth(t, editor.W, 576)
}

func TestWorkbenchKeepsMinimumsAtTheMinimumWindow(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 960, 640)
	tt.Frame()

	editor := accessRect(t, tt, ui.RoleMain, "简历编辑")
	preview, ok := tt.Find("A4 预览")
	if !ok {
		t.Fatal("no A4 preview")
	}
	if editor.W < minEditorWidth || preview.W < minPreviewWidth {
		t.Fatalf("workbench = %v + %v, want %v + %v", editor.W, preview.W, minEditorWidth, minPreviewWidth)
	}
	if editor.W+splitLineWidth+preview.W > 960+1 {
		t.Fatalf("workbench = %v + %v overflows the window", editor.W, preview.W)
	}
}

func TestDocumentRailOverlaysWhenTheWorkbenchCannotFit(t *testing.T) {
	store := openDesktopStore(t)
	first, err := store.Create("第一篇", []byte("# 第一篇\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("第二篇", []byte("# 第二篇\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1100, 800)
	tt.Frame()

	editorBefore := accessRect(t, tt, ui.RoleMain, "简历编辑")
	previewBefore, _ := tt.Find("A4 预览")
	openDocumentRail(t, tt)
	rail := accessRect(t, tt, ui.RoleNavigation, "文档栏")
	editorAfter := accessRect(t, tt, ui.RoleMain, "简历编辑")
	nearWidth(t, editorAfter.W, editorBefore.W)
	if rail.X != 0 || rail.Y != topBarHeight || rail.W != documentRailWidth {
		t.Fatalf("overlay rail bounds = %v", rail)
	}
	if err := tt.Click("第一篇"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if app.currentID != first.ID {
		t.Fatalf("document switch in the overlay rail = %q, want %q", app.currentID, first.ID)
	}
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); !ok {
		t.Fatal("switching documents closed the overlay rail")
	}

	tt.ClickAt(previewBefore.X+previewBefore.W-12, previewBefore.Y+80)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); ok || app.railOpen {
		t.Fatal("clicking outside did not close the overlay rail")
	}

	openDocumentRail(t, tt)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); ok || app.railOpen {
		t.Fatal("Escape did not close the overlay rail")
	}
}

func TestDocumentRailStartsCollapsedAtWideWindows(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("第一篇", []byte("# 第一篇\n")); err != nil {
		t.Fatal(err)
	}
	_, _, tt := newLibraryHarness(t, store, 1440, 900)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); ok {
		t.Fatal("the document rail started open")
	}
	openDocumentRail(t, tt)
	editor := accessRect(t, tt, ui.RoleMain, "简历编辑")
	nearWidth(t, editor.W, 486)
}
