package desktop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"tokicv/internal/library"
)

func newLibraryHarness(t *testing.T, store *library.Store, width, height int) (*App, *fakePreview, *ui.Tester) {
	t.Helper()
	app := New(Options{Store: store})
	app.SetArtifactPipeline(&fakeArtifactPipeline{})
	preview := &fakePreview{nextPages: 1, onPages: app.SetPageCount}
	app.SetPreview(preview)
	return app, preview, ui.NewTester(app.View, width, height)
}

func openDocumentRail(t *testing.T, tt *ui.Tester) {
	t.Helper()
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); ok {
		return
	}
	toggle, ok := tt.AccessByRole(ui.RoleButton, "文档栏")
	if !ok {
		t.Fatal("no document rail toggle")
	}
	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); !ok {
		t.Fatal("the document rail did not open")
	}
}

func openDesktopStore(t *testing.T) *library.Store {
	t.Helper()
	store, err := library.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestEmptyLibraryOnboardingCreatesBlankAndStarterDocuments(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action string
		source func() string
	}{
		{name: "blank", action: "新建空白简历", source: func() string { return "# 未命名简历\n\n" }},
		{name: "starter", action: "从示例模板创建", source: library.StarterMarkdown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openDesktopStore(t)
			app, _, tt := newLibraryHarness(t, store, 1440, 900)

			for _, label := range []string{"新建空白简历", "从示例模板创建", "打开本地 .md 文件"} {
				if !tt.HasText(label) {
					t.Fatalf("empty library does not show %q", label)
				}
			}
			if tt.HasText("本地简历库") {
				t.Error("empty library adds an explanatory heading")
			}
			if err := tt.Click(tc.action); err != nil {
				t.Fatal(err)
			}
			drainFileIO(t, app, tt)

			documents, err := store.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(documents) != 1 {
				t.Fatalf("library documents = %d, want 1", len(documents))
			}
			if app.currentID != documents[0].ID {
				t.Fatalf("current document = %q, want %q", app.currentID, documents[0].ID)
			}
			if got := app.buffer.String(); got != tc.source() {
				t.Fatalf("created source = %q, want %q", got, tc.source())
			}
			if !tt.HasText("Markdown 源文") || !tt.HasText("A4 预览") {
				t.Error("creating a document did not enter split editing and preview")
			}
			openDocumentRail(t, tt)
			if !tt.HasText(documents[0].Name) {
				t.Errorf("document rail does not show %q", documents[0].Name)
			}
			if strings.Contains(strings.Join(tt.Texts(), "\n"), "·") {
				t.Error("the rail added an unexplained status dot")
			}
		})
	}
}

func TestLibrarySwitchSavesEditsThenRenameAndDelete(t *testing.T) {
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

	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	if app.currentID != second.ID {
		t.Fatalf("initial document = %q, want the most recently updated %q", app.currentID, second.ID)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("# 第二篇修改\n")
	openDocumentRail(t, tt)

	if err := tt.Click("第一篇"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if app.currentID != first.ID {
		t.Fatalf("switched to %q, want %q", app.currentID, first.ID)
	}
	if got := app.buffer.String(); got != "# 第一篇\n" {
		t.Fatalf("switched source = %q", got)
	}
	if _, source, err := store.Show(second.ID); err != nil || string(source) != "# 第二篇修改\n" {
		t.Fatalf("edited previous document = %q, %v; want it saved before switching", source, err)
	}

	if err := tt.Click("重命名"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("重命名简历") {
		t.Fatal("rename dialog is missing")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("后端项目版")
	if err := tt.Click("确定"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	renamed, err := store.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "后端项目版" || app.currentName != "后端项目版" {
		t.Fatalf("renamed document = %#v, current name = %q", renamed, app.currentName)
	}
	if !tt.HasText("后端项目版") {
		t.Error("the rail does not show the new name")
	}

	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("删除简历？") {
		t.Fatal("delete confirmation is missing")
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	remaining, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != second.ID {
		t.Fatalf("remaining documents = %#v, want only %q", remaining, second.ID)
	}
	if app.currentID != second.ID || app.buffer.String() != "# 第二篇修改\n" {
		t.Fatalf("after delete current = %q, source = %q", app.currentID, app.buffer.String())
	}
}

func TestLibrarySaveShortcutAndButtonPersistEdits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		source  string
		trigger func(*testing.T, *ui.Tester)
	}{
		{
			name:   "shortcut",
			source: "# 库内快捷保存\n\n正文内容",
			trigger: func(_ *testing.T, tt *ui.Tester) {
				tt.Key(ui.Cmd, ui.KeyS)
			},
		},
		{
			name:   "button",
			source: "# 库内按钮保存\n\n正文内容",
			trigger: func(t *testing.T, tt *ui.Tester) {
				t.Helper()
				if err := tt.Click("保存"); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openDesktopStore(t)
			document, err := store.Create("库内简历", []byte("# 旧标题\n"))
			if err != nil {
				t.Fatal(err)
			}
			app, _, tt := newLibraryHarness(t, store, 1440, 900)
			save, ok := tt.AccessByRole(ui.RoleButton, "保存")
			if !ok {
				t.Fatal("the save button is missing for a library Resume Document")
			}
			if save.Disabled {
				t.Fatal("the save button is disabled for a library Resume Document")
			}

			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type(tc.source)
			tc.trigger(t, tt)
			drainFileIO(t, app, tt)

			if _, source, err := store.Show(document.ID); err != nil || string(source) != tc.source {
				t.Fatalf("persisted source = %q, %v; want %q", source, err, tc.source)
			}
			if app.buffer.Modified() {
				t.Fatal("the editor is still modified after saving a library Resume Document")
			}
		})
	}
}

func TestLibrarySaveFailureShowsAlertAndKeepsEdits(t *testing.T) {
	original := "# 旧标题\n"
	store := openDesktopStore(t)
	document, err := store.Create("库内简历", []byte(original))
	if err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	app.updateContent = func(*library.Store, string, []byte) (library.Document, error) {
		return library.Document{}, errors.New("write failed")
	}

	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("# 未能保存\n")
	tt.Key(ui.Cmd, ui.KeyS)
	drainFileIO(t, app, tt)

	alert, ok := tt.AccessByRole(ui.RoleAlert, "保存失败")
	if !ok || alert.Role != ui.RoleAlert {
		t.Fatalf("library save failure alert = %+v, %v", alert, ok)
	}
	if !app.buffer.Modified() {
		t.Fatal("a failed library save marked the buffer saved")
	}
	if _, source, err := store.Show(document.ID); err != nil || string(source) != original {
		t.Fatalf("persisted source after failure = %q, %v; want %q", source, err, original)
	}
}

func TestClosingLibraryDocumentFlushesUnsavedEdits(t *testing.T) {
	store := openDesktopStore(t)
	document, err := store.Create("库内简历", []byte("# 旧标题\n"))
	if err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)

	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("# 关闭前保存\n\n正文内容")
	if !app.buffer.Modified() {
		t.Fatal("the input did not modify the library Resume Document")
	}

	app.close()
	app.CloseAndWait()

	if _, source, err := store.Show(document.ID); err != nil ||
		string(source) != "# 关闭前保存\n\n正文内容" {
		t.Fatalf("persisted source after close = %q, %v; want the latest edit", source, err)
	}
}

func TestClosingLibraryDocumentFlushesTheLatestVersionAfterAQueuedSave(t *testing.T) {
	original := "# 原始标题\n"
	store := openDesktopStore(t)
	document, err := store.Create("库内简历", []byte(original))
	if err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)

	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	app.updateContent = func(store *library.Store, id string, source []byte) (library.Document, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return store.UpdateContent(id, source)
	}

	app.buffer.Set("# 排队中的旧编辑\n")
	tt.Frame()
	if err := app.Save(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the queued library save did not start")
	}

	app.buffer.Set(original)
	if app.buffer.Modified() {
		t.Fatal("returning to the saved text left the buffer modified")
	}
	closed := make(chan struct{})
	go func() {
		app.close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("closing blocked on the queued library save")
	}

	waited := make(chan struct{})
	go func() {
		app.CloseAndWait()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("CloseAndWait returned before the queued saves finished")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("CloseAndWait did not return after the queued saves finished")
	}

	if _, source, err := store.Show(document.ID); err != nil || string(source) != original {
		t.Fatalf("persisted source after close = %q, %v; want %q", source, err, original)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("library writes = %d, want the queued save and the close-time save", got)
	}
}

func TestExternalMarkdownOpensExportsExactBytesAndImportsToLibrary(t *testing.T) {
	dir := t.TempDir()
	externalPath := filepath.Join(dir, "外部简历.md")
	source := []byte("\xef\xbb\xbf# 外部简历\r\n\r\n- 原始字节没有结尾换行")
	if err := os.WriteFile(externalPath, source, 0o640); err != nil {
		t.Fatal(err)
	}

	store := openDesktopStore(t)
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	app.SetPicker(func() (string, error) { return externalPath, nil })
	if err := tt.Click("打开本地 .md 文件"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if app.path != externalPath || !bytes.Equal(app.rawSource, source) {
		t.Fatalf("opened path = %q, exact source = %q", app.path, app.rawSource)
	}
	if !strings.Contains(app.buffer.String(), "原始字节没有结尾换行") {
		t.Fatalf("opened editor source = %q", app.buffer.String())
	}

	exportPath := filepath.Join(dir, "导出副本.md")
	app.SetMarkdownExportPath(func(defaultName string) (string, error) {
		if defaultName != "外部简历.md" {
			t.Errorf("export default name = %q, want 外部简历.md", defaultName)
		}
		return exportPath, nil
	})
	if err := tt.Click("导出"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("导出 Markdown"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	exported, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(exported, source) {
		t.Fatalf("exported bytes = %q, want the exact source %q", exported, source)
	}

	if err := tt.Click("存入简历库"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	documents, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || documents[0].Name != "外部简历" {
		t.Fatalf("imported documents = %#v, want one named 外部简历", documents)
	}
	if _, imported, err := store.Show(documents[0].ID); err != nil || !bytes.Equal(imported, source) {
		t.Fatalf("imported bytes = %q, %v; want exact source", imported, err)
	}
	after, err := os.ReadFile(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, source) {
		t.Fatalf("source file changed on import: %q", after)
	}
	if app.path != "" || app.currentID != documents[0].ID {
		t.Fatalf("after import path = %q, current = %q", app.path, app.currentID)
	}
}

func TestLibraryRailAutoCollapsesAndKeepsTheSheetAndToolbarVisible(t *testing.T) {
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

	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); ok {
		t.Fatal("the document rail starts open")
	}
	openDocumentRail(t, tt)
	rail, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏")
	if !ok {
		t.Fatal("the document rail did not open at a wide window")
	}
	if rail.Bounds.W != documentRailWidth {
		t.Fatalf("rail width = %v, want %v", rail.Bounds.W, documentRailWidth)
	}
	active, ok := tt.AccessByRole(ui.RoleListItem, second.Name)
	if !ok || !active.Selected {
		t.Fatalf("active document access = %+v, %v", active, ok)
	}
	inactive, ok := tt.AccessByRole(ui.RoleListItem, first.Name)
	if !ok || inactive.Selected {
		t.Fatalf("inactive document access = %+v, %v", inactive, ok)
	}
	selected := 0
	for _, item := range tt.AccessAll() {
		if item.Role == ui.RoleListItem && item.Selected {
			selected++
		}
	}
	if selected != 1 {
		t.Errorf("selected document cards = %d, want exactly 1", selected)
	}
	if _, distance := nearestPixelColor(tt, active.Bounds, manuscriptPaper); distance > 12 {
		t.Errorf("active card has no manuscript-paper fill: color distance %d", distance)
	}
	if _, distance := nearestPixelColor(tt, active.Bounds, activeCardEdge); distance > 12 {
		t.Errorf("active card has no #CFCBC2 border: color distance %d", distance)
	}
	if _, distance := nearestPixelColor(tt, inactive.Bounds, sidebarTray); distance > 12 {
		t.Errorf("inactive card does not rest on the sidebar tray: color distance %d", distance)
	}
	leftStrip := ui.Rect{
		X: active.Bounds.X,
		Y: active.Bounds.Y + 4,
		W: 4,
		H: active.Bounds.H - 8,
	}
	if _, distance := nearestPixelColor(tt, leftStrip, editorialCobalt); distance <= 24 {
		t.Error("active card has a colored left stripe")
	}
	if _, distance := nearestPixelColor(tt, leftStrip, proofVermilion); distance <= 24 {
		t.Error("active card has a colored status mark")
	}

	tt.SetSize(1280, 800)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleNavigation, "文档栏"); ok {
		t.Fatal("the rail remains open at 1280px")
	}
	toggle, ok := tt.AccessByRole(ui.RoleButton, "文档栏")
	if !ok {
		t.Fatal("the toolbar lost its rail toggle")
	}
	bar, ok := tt.AccessByRole(ui.RoleBanner, "tokicv 工具栏")
	if !ok {
		t.Fatal("no toolbar landmark")
	}
	for _, name := range []string{"文档栏", "新建", "打开 Markdown 文件", "保存", "智能一页", "排版", "导出"} {
		control, ok := tt.Find(name)
		if !ok {
			t.Errorf("toolbar control %q is missing at 1280×800", name)
			continue
		}
		if control.X < bar.Bounds.X ||
			control.X+control.W > bar.Bounds.X+bar.Bounds.W+0.5 {
			t.Errorf("toolbar control %q bounds %v are clipped", name, control)
		}
	}
	stage, ok := tt.Find("A4 预览")
	if !ok {
		t.Fatal("no A4 preview stage")
	}
	if stage.W < minPreviewWidth {
		t.Fatalf("preview stage width = %v, want at least %v", stage.W, minPreviewWidth)
	}
	if stage.X+stage.W > 1280 {
		t.Fatalf("preview stage %v is clipped", stage)
	}
	indicator, ok := tt.Find("1 / 1")
	if !ok {
		t.Fatal("no page indicator")
	}
	if indicator.Y < stage.Y+stage.H-48 || indicator.X+indicator.W > stage.X+stage.W-12 {
		t.Errorf("page indicator %v is outside the tray margin", indicator)
	}

	tt.ClickAt(toggle.Bounds.X+toggle.Bounds.W/2, toggle.Bounds.Y+toggle.Bounds.H/2)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	rail, ok = tt.AccessByRole(ui.RoleNavigation, "文档栏")
	if !ok {
		t.Fatal("the rail did not open from the toolbar")
	}
	if rail.Bounds.W != documentRailWidth {
		t.Fatalf("manually opened rail width = %v, want %v", rail.Bounds.W, documentRailWidth)
	}
	stage, ok = tt.Find("A4 预览")
	if !ok {
		t.Fatal("no A4 preview stage with the rail open")
	}
	if stage.W < minPreviewWidth || stage.X+stage.W > 1280 {
		t.Fatalf("A4 stage %v is clipped with the rail open", stage)
	}
	editor, ok := tt.AccessByRole(ui.RoleMain, "简历编辑")
	if !ok {
		t.Fatal("no editor pane with the rail open")
	}
	if editor.Bounds.W < minEditorWidth {
		t.Fatalf("editor width = %v, want at least %v", editor.Bounds.W, minEditorWidth)
	}
	if app.railOpen != true {
		t.Error("the rail toggle did not persist its open state")
	}
}

func TestLibrarySwitchPersistsInBackgroundWithoutBlockingTheUI(t *testing.T) {
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
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	openDocumentRail(t, tt)
	app.buffer.Set("# 第二篇修改\n")
	tt.Frame()

	started := make(chan struct{})
	release := make(chan struct{})
	app.updateContent = func(store *library.Store, id string, source []byte) (library.Document, error) {
		close(started)
		<-release
		return store.UpdateContent(id, source)
	}

	clicked := make(chan error, 1)
	go func() { clicked <- tt.Click("第一篇") }()
	select {
	case err := <-clicked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("switching blocked the UI while saving the current document")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the background save did not start")
	}
	if app.currentID != second.ID {
		t.Fatalf("the UI changed document before the background save finished: %q", app.currentID)
	}

	close(release)
	drainFileIO(t, app, tt)
	if app.currentID != first.ID {
		t.Fatalf("current document = %q, want %q", app.currentID, first.ID)
	}
	if _, source, err := store.Show(second.ID); err != nil || string(source) != "# 第二篇修改\n" {
		t.Fatalf("background save = %q, %v", source, err)
	}
}

func TestRapidLibrarySwitchesKeepTheLatestEdits(t *testing.T) {
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
	third, err := store.Create("第三篇", []byte("# 第三篇\n"))
	if err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	openDocumentRail(t, tt)
	if app.currentID != third.ID {
		t.Fatalf("initial document = %q, want %q", app.currentID, third.ID)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	app.updateContent = func(store *library.Store, id string, source []byte) (library.Document, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return store.UpdateContent(id, source)
	}

	app.buffer.Set("# 第三篇较早版本\n")
	tt.Frame()
	clicked := make(chan error, 1)
	go func() { clicked <- tt.Click("第一篇") }()
	select {
	case err := <-clicked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("the first switch blocked")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the first background save did not start")
	}

	app.buffer.Set("# 第三篇最终版本\n")
	if err := tt.Click("第二篇"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	drainFileIO(t, app, tt)

	if _, source, err := store.Show(third.ID); err != nil || string(source) != "# 第三篇最终版本\n" {
		t.Fatalf("final saved source = %q, %v; want the latest edit", source, err)
	}
	if app.currentID != second.ID {
		t.Fatalf("current document = %q, want the latest requested %q", app.currentID, second.ID)
	}
	if _, source, err := store.Show(first.ID); err != nil || string(source) != "# 第一篇\n" {
		t.Fatalf("unrequested document changed: %q, %v", source, err)
	}
}

func TestToolbarAndRailCreateBlankAndStarterDocuments(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("现有简历", []byte("# 现有\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	openDocumentRail(t, tt)

	if err := tt.Click("新建"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("新建空白简历") || !tt.HasText("从示例模板创建") {
		t.Fatal("the toolbar new menu is missing its two document choices")
	}
	if err := tt.Click("从示例模板创建"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if app.newMenuOpen {
		t.Error("the toolbar new menu stayed open after choosing a template")
	}
	documents, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 2 {
		t.Fatalf("documents after toolbar create = %d, want 2", len(documents))
	}
	if app.currentName != "软件工程师简历" || app.buffer.String() != library.StarterMarkdown() {
		t.Fatalf("toolbar starter document = %q, source match = %v",
			app.currentName, app.buffer.String() == library.StarterMarkdown())
	}

	if err := tt.Click("新建空白简历"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	documents, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 3 || app.currentName != "未命名简历" || app.buffer.String() != "# 未命名简历\n\n" {
		t.Fatalf("rail blank document = %q, documents = %d, source = %q",
			app.currentName, len(documents), app.buffer.String())
	}
}

func TestDeletingTheLastDocumentReturnsToOnboarding(t *testing.T) {
	store := openDesktopStore(t)
	if _, err := store.Create("最后一篇", []byte("# 最后\n")); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	openDocumentRail(t, tt)
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)

	if app.currentID != "" || app.buffer.String() != "" {
		t.Fatalf("deleted current document = %q, source = %q", app.currentID, app.buffer.String())
	}
	for _, label := range []string{"新建空白简历", "从示例模板创建", "打开本地 .md 文件"} {
		if !tt.HasText(label) {
			t.Errorf("onboarding after deleting the last document does not show %q", label)
		}
	}
	documents, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 0 {
		t.Fatalf("remaining documents = %#v, want none", documents)
	}
}

func TestOpeningExternalFileSavesTheCurrentLibraryEdit(t *testing.T) {
	store := openDesktopStore(t)
	document, err := store.Create("库内简历", []byte("# 库内\n"))
	if err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), "外部.md")
	if err := os.WriteFile(externalPath, []byte("# 外部\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newLibraryHarness(t, store, 1440, 900)
	app.SetPicker(func() (string, error) { return externalPath, nil })
	app.buffer.Set("# 库内修改\n")
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyO)
	drainFileIO(t, app, tt)
	if app.path != externalPath || app.buffer.String() != "# 外部\n" {
		t.Fatalf("opened path = %q, source = %q", app.path, app.buffer.String())
	}
	if _, source, err := store.Show(document.ID); err != nil || string(source) != "# 库内修改\n" {
		t.Fatalf("library source after open = %q, %v; want the edit saved first", source, err)
	}
}
