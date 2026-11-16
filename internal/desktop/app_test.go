package desktop

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"tokicv/internal/render"
)

type fakePreview struct {
	html      []string
	nextPages int
	onPages   func(int)
}

func nearestPixelColor(tt *ui.Tester, bounds ui.Rect, want ui.Color) (color.RGBA, int) {
	img := tt.Image()
	x0 := max(0, int(bounds.X))
	y0 := max(0, int(bounds.Y))
	x1 := min(img.Rect.Dx(), int(bounds.X+bounds.W+0.5))
	y1 := min(img.Rect.Dy(), int(bounds.Y+bounds.H+0.5))
	best := color.RGBA{}
	distance := 1 << 30
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			got := img.RGBAAt(x, y)
			dr := int(got.R) - int(want.R)
			dg := int(got.G) - int(want.G)
			db := int(got.B) - int(want.B)
			if d := dr*dr + dg*dg + db*db; d < distance {
				best, distance = got, d
			}
		}
	}
	return best, distance
}

func (p *fakePreview) Show(frame render.PreviewFrame) {
	p.html = append(p.html, frame.HTML)
	if p.onPages != nil {
		pages := p.nextPages
		if pages < 1 {
			pages = 1
		}
		p.onPages(pages)
	}
}

func (p *fakePreview) Update(frame render.PreviewFrame) {
	p.html = append(p.html, frame.HTML)
	if p.onPages != nil {
		pages := p.nextPages
		if pages < 1 {
			pages = 1
		}
		p.onPages(pages)
	}
}

func (p *fakePreview) View(c *ui.Context) ui.Element {
	return ui.Box(c.Key("fake-preview")).Fill().Background(proofingStage).Label("预览画布")
}

func newHarness(t *testing.T, source string) (*App, *fakePreview, *ui.Tester) {
	return newHarnessAt(t, source, 1280, 800)
}

func newHarnessAt(t *testing.T, source string, width, height int) (*App, *fakePreview, *ui.Tester) {
	t.Helper()
	app := New(Options{})
	if source != "" {
		app.buffer.Reset(source)
		app.ensureAnalysis()
	}
	preview := &fakePreview{nextPages: 1, onPages: app.SetPageCount}
	app.SetPreview(preview)
	return app, preview, ui.NewTester(app.View, width, height)
}

func drainFileIO(t *testing.T, app *App, tt *ui.Tester) {
	t.Helper()
	app.work.Wait()
	app.drainUpdates()
	tt.Frame()
}

func TestEditorInputDebouncesDOMPreviewAndPageCount(t *testing.T) {
	app, preview, tt := newHarness(t, "")
	if err := tt.Click("Markdown 源文"); err != nil {
		t.Fatal(err)
	}
	loadsBefore := len(preview.html)
	preview.nextPages = 2

	tt.Type("# 新标题")
	if got := len(preview.html); got != loadsBefore {
		t.Fatalf("typing loaded the preview immediately: %d loads", got-loadsBefore)
	}
	tt.Advance(PreviewDebounce / 2)
	if got := len(preview.html); got != loadsBefore {
		t.Fatalf("preview loaded before the debounce elapsed: %d loads", got-loadsBefore)
	}
	tt.Advance(PreviewDebounce)
	if got := len(preview.html); got != loadsBefore+1 {
		t.Fatalf("preview loads after debounce = %d, want %d", got-loadsBefore, 1)
	}
	if got := app.PageCount(); got != 2 {
		t.Fatalf("page count = %d, want 2", got)
	}
	if !strings.Contains(preview.html[len(preview.html)-1], "新标题") {
		t.Error("the debounced preview does not contain the edited text")
	}
	if !tt.HasText("1 / 2") {
		t.Error("the tray does not show the reported page count")
	}
	if strings.Contains(preview.html[len(preview.html)-1], "PrintToPDF") {
		t.Error("the live preview reaches for the print pipeline")
	}
}

func TestSyntaxDiagnosticsMarkGutterAndSingleIndicator(t *testing.T) {
	app, _, tt := newHarness(t, "::: left\nicon:unknown 内容\n")
	tt.Frame()

	if !tt.HasText("2 处语法错误") {
		t.Error("the shell does not show one concise syntax-error signal")
	}
	if got := len(app.sourceEditor.GutterMarks); got != 2 {
		t.Fatalf("gutter marks = %d, want 2", got)
	}
	for _, mark := range app.sourceEditor.GutterMarks {
		if mark.Color != proofVermilion {
			t.Errorf("gutter mark at line %d has color %#v", mark.Line, mark.Color)
		}
	}
	if got := len(app.sourceEditor.Diagnostics()); got < 2 {
		t.Errorf("editor diagnostics = %d, want the syntax issues", got)
	}

	clean, _, cleanTester := newHarness(t, "# 正常\n")
	cleanTester.Frame()
	if cleanTester.HasText("语法错误") {
		t.Error("a clean document still shows a diagnostic signal")
	}
	if got := len(clean.sourceEditor.GutterMarks); got != 0 {
		t.Errorf("a clean document has %d gutter marks", got)
	}
}

func TestOpenEditAndSaveExternalMarkdownInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.md")
	if err := os.WriteFile(path, []byte("# 旧标题\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	app, preview, tt := newHarness(t, "")
	picked := 0
	app.SetPicker(func() (string, error) {
		picked++
		return path, nil
	})
	var title string
	app.SetTitleHandler(func(value string) { title = value })

	tt.Key(ui.Cmd, ui.KeyO)
	drainFileIO(t, app, tt)
	if picked != 1 {
		t.Fatalf("open shortcut chose %d files, want 1", picked)
	}
	if app.Path() != path {
		t.Fatalf("path = %q, want %q", app.Path(), path)
	}
	if title != "tokicv — resume.md" {
		t.Errorf("window title = %q", title)
	}
	if !tt.HasText("resume.md") {
		t.Error("the shell does not show the external path context")
	}
	if !tt.Focused("Markdown 源文") {
		t.Error("opening a file did not return focus to the editor")
	}
	if len(preview.html) == 0 || !strings.Contains(preview.html[len(preview.html)-1], "旧标题") {
		t.Error("opening a file did not refresh the preview")
	}

	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("# 新标题\n\n正文")
	tt.Key(ui.Cmd, ui.KeyS)
	drainFileIO(t, app, tt)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# 新标题\n\n正文" {
		t.Errorf("saved source = %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("saved mode = %v, want 0640", info.Mode().Perm())
	}
	if app.buffer.Modified() {
		t.Error("the editor is still modified after saving in place")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("saving left files beside the document: %v", entries)
	}
	if tt.HasText("已保存") {
		t.Error("the shell adds a redundant saved state")
	}
}

func TestOpenRunsOffUIThreadAndRejectsEditedDocument(t *testing.T) {
	app, preview, tt := newHarness(t, "# 当前\n")
	started := make(chan struct{})
	release := make(chan struct{})
	app.readFile = func(string) ([]byte, error) {
		close(started)
		<-release
		return []byte("# 外部\n"), nil
	}

	done := make(chan error, 1)
	go func() { done <- app.Open("resume.md") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Open blocked the caller on file I/O")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the file read did not start")
	}

	tt.Type("改动")
	close(release)
	drainFileIO(t, app, tt)

	if app.Path() != "" {
		t.Fatalf("an edit during the read still opened %q", app.Path())
	}
	if !strings.Contains(app.buffer.String(), "改动") {
		t.Error("the stale read replaced the edited document")
	}
	if got := preview.html[len(preview.html)-1]; strings.Contains(got, "外部") {
		t.Error("the stale read refreshed the preview")
	}
}

func TestLatestOpenRequestWins(t *testing.T) {
	app, _, tt := newHarness(t, "# 当前\n")
	startedA := make(chan struct{})
	startedB := make(chan struct{})
	release := make(chan struct{})
	app.readFile = func(path string) ([]byte, error) {
		switch filepath.Base(path) {
		case "a.md":
			close(startedA)
		case "b.md":
			close(startedB)
		}
		<-release
		if filepath.Base(path) == "a.md" {
			return []byte("# A\n"), nil
		}
		return []byte("# B\n"), nil
	}

	if err := app.Open("a.md"); err != nil {
		t.Fatal(err)
	}
	<-startedA
	if err := app.Open("b.md"); err != nil {
		t.Fatal(err)
	}
	<-startedB
	close(release)
	drainFileIO(t, app, tt)

	if filepath.Base(app.Path()) != "b.md" {
		t.Fatalf("opened %q, want the latest request b.md", app.Path())
	}
	if got := app.buffer.String(); !strings.Contains(got, "# B") {
		t.Errorf("opened content = %q, want B", got)
	}
}

func TestClosedWorkspaceDropsPendingFileResults(t *testing.T) {
	app, preview, tt := newHarness(t, "# 当前\n")
	started := make(chan struct{})
	release := make(chan struct{})
	app.readFile = func(string) ([]byte, error) {
		close(started)
		<-release
		return []byte("# 外部\n"), nil
	}
	if err := app.Open("resume.md"); err != nil {
		t.Fatal(err)
	}
	<-started
	app.close()
	close(release)
	drainFileIO(t, app, tt)

	if app.Path() != "" || strings.Contains(app.buffer.String(), "外部") {
		t.Error("a file result reached a closed workspace")
	}
	if got := preview.html[len(preview.html)-1]; strings.Contains(got, "外部") {
		t.Error("a file result refreshed the preview of a closed workspace")
	}
}

func TestCloseAndWaitFinishesPendingSaveWithoutBlockingClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.md")
	if err := os.WriteFile(path, []byte("# 旧标题\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, _, _ := newHarness(t, "")
	app.loadDocument(path, "# 旧标题\n")
	app.buffer.Set("# 新标题\n")

	started := make(chan struct{})
	release := make(chan struct{})
	app.writeFile = func(path string, snapshot editor.Snapshot) error {
		close(started)
		<-release
		return writeDocument(path, snapshot)
	}
	if err := app.Save(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the save did not start")
	}

	closeReturned := make(chan struct{})
	go func() {
		app.close()
		close(closeReturned)
	}()
	select {
	case <-closeReturned:
	case <-time.After(time.Second):
		t.Fatal("closing the workspace blocked on the pending save")
	}

	waitReturned := make(chan struct{})
	go func() {
		app.CloseAndWait()
		close(waitReturned)
	}()
	select {
	case <-waitReturned:
		t.Fatal("CloseAndWait returned before the pending save finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-waitReturned:
	case <-time.After(time.Second):
		t.Fatal("CloseAndWait did not return after the save finished")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# 新标题\n" {
		t.Errorf("saved source after close = %q, want the queued snapshot", got)
	}
}

func TestStaleSaveDoesNotClearEdits(t *testing.T) {
	app, _, tt := newHarness(t, "# 当前\n")
	app.path = filepath.Join(t.TempDir(), "resume.md")
	app.document++
	started := make(chan struct{})
	release := make(chan struct{})
	app.writeFile = func(string, editor.Snapshot) error {
		close(started)
		<-release
		return nil
	}

	tt.Key(ui.Cmd, ui.KeyS)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the save did not start")
	}
	tt.Type("改动")
	close(release)
	drainFileIO(t, app, tt)

	if !app.buffer.Modified() {
		t.Error("a stale save marked the edited buffer saved")
	}
}

func TestSavesToSameResumeDocumentWriteInRequestOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.md")
	app, _, tt := newHarness(t, "")
	app.loadDocument(path, "# 旧标题\n")

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	var writesMu sync.Mutex
	var writes []string
	call := 0
	app.writeFile = func(path string, snapshot editor.Snapshot) error {
		writesMu.Lock()
		call++
		current := call
		writes = append(writes, snapshot.String())
		writesMu.Unlock()
		switch current {
		case 1:
			close(firstStarted)
			<-releaseFirst
		case 2:
			close(secondStarted)
		}
		err := writeDocument(path, snapshot)
		if current == 2 {
			close(secondDone)
		}
		return err
	}

	released := false
	defer func() {
		if !released {
			close(releaseFirst)
		}
	}()

	if err := app.Save(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("the first save did not start")
	}

	app.buffer.Set("# 新标题\n")
	if err := app.Save(); err != nil {
		t.Fatal(err)
	}

	secondRanEarly := false
	select {
	case <-secondStarted:
		secondRanEarly = true
		<-secondDone
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	released = true
	drainFileIO(t, app, tt)

	if secondRanEarly {
		t.Error("the second write began while the first save was still in flight")
	}
	if len(writes) != 2 || writes[0] != "# 旧标题\n" || writes[1] != "# 新标题\n" {
		t.Errorf("writes = %q, want the requested snapshots in order", writes)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# 新标题\n" {
		t.Errorf("saved source = %q, want the latest snapshot", got)
	}
	if app.buffer.Modified() {
		t.Error("the latest save did not mark the Resume Document saved")
	}
	if tt.HasText("保存失败") {
		t.Error("the latest successful save left a failure alert")
	}
}

func TestSaveFailureKeepsAlertAfterFurtherEditing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.md")
	app, _, tt := newHarness(t, "")
	app.loadDocument(path, "# 当前标题\n")

	started := make(chan struct{})
	release := make(chan struct{})
	app.writeFile = func(string, editor.Snapshot) error {
		close(started)
		<-release
		return errors.New("write failed")
	}

	tt.Key(ui.Cmd, ui.KeyS)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the save did not start")
	}
	tt.Type("续写")
	close(release)
	drainFileIO(t, app, tt)

	alert, ok := tt.AccessByRole(ui.RoleAlert, "保存失败")
	if !ok {
		t.Fatal("a failed save during further editing has no accessible alert")
	}
	if !tt.HasText("保存失败") {
		t.Error("the failed save text is missing")
	}
	want := ui.Hex("#C43B2B")
	nearest, distance := nearestPixelColor(tt, alert.Bounds, want)
	if distance > 8 {
		t.Errorf("the save failure alert is not drawn in %#x: nearest %v, distance %d", want, nearest, distance)
	}
	if !app.buffer.Modified() {
		t.Error("a failed save marked the further-edited Resume Document saved")
	}
}

func TestSaveFailureShowsOneAlertUntilSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.md")
	if err := os.WriteFile(path, []byte("# 旧标题\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, _, tt := newHarness(t, "")
	app.SetPicker(func() (string, error) { return path, nil })

	tt.Key(ui.Cmd, ui.KeyO)
	drainFileIO(t, app, tt)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("# 新标题\n")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyS)
	drainFileIO(t, app, tt)

	alert, ok := tt.AccessByRole(ui.RoleAlert, "保存失败")
	if !ok {
		t.Fatal("a failed save has no accessible alert")
	}
	if !tt.HasText("保存失败") || alert.Role != ui.RoleAlert {
		t.Errorf("save failure alert = %+v", alert)
	}
	if tt.HasText("permission denied") || tt.HasText(path) {
		t.Error("the save alert carries explanatory clutter")
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if tt.HasText("保存失败") {
		t.Error("a successful save left the alert visible")
	}
	if app.buffer.Modified() {
		t.Error("the successful save did not mark the buffer saved")
	}

	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	drainFileIO(t, app, tt)
	if !tt.HasText("保存失败") {
		t.Error("the save button did not surface a write failure")
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyS)
	drainFileIO(t, app, tt)
	if tt.HasText("保存失败") {
		t.Error("the save shortcut did not clear the recovered alert")
	}
}

func TestWorkspaceFitsA4PreviewAtStandardViewports(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{"laptop", 1280, 800},
		{"desktop", 1920, 1080},
	} {
		t.Run(size.name, func(t *testing.T) {
			_, _, tt := newHarnessAt(t, "# 示例文档\n", size.width, size.height)
			tt.Frame()

			bar, ok := tt.AccessByRole(ui.RoleBanner, "tokicv 工具栏")
			if !ok {
				t.Fatal("no toolbar landmark")
			}
			if bar.Bounds.H != 46 {
				t.Errorf("top bar height = %v, want 46", bar.Bounds.H)
			}
			editorPane, ok := tt.AccessByRole(ui.RoleMain, "简历编辑")
			if !ok {
				t.Fatal("no editor pane")
			}
			stage, ok := tt.Find("A4 预览")
			if !ok {
				t.Fatal("no A4 preview stage")
			}
			indicator, ok := tt.Find("1 / 1")
			if !ok {
				t.Fatal("no page indicator")
			}
			if editorPane.Bounds.X+editorPane.Bounds.W > stage.X+1 {
				t.Errorf("editor pane %v overlaps preview %v", editorPane.Bounds, stage)
			}
			bottom := float32(size.height)
			if editorPane.Bounds.Y < bar.Bounds.Y+bar.Bounds.H || editorPane.Bounds.Y+editorPane.Bounds.H > bottom {
				t.Errorf("editor pane bounds %v do not fit below the 46px bar", editorPane.Bounds)
			}
			if stage.Y != editorPane.Bounds.Y || stage.H != editorPane.Bounds.H {
				t.Errorf("preview %v and editor pane %v do not share the body height", stage, editorPane.Bounds)
			}
			if stage.X+stage.W > float32(size.width)+0.5 || stage.Y+stage.H > bottom+0.5 {
				t.Errorf("preview stage %v is clipped by the window", stage)
			}
			const pageWidth = 210 * 96 / 25.4
			minWidth := pageWidth*0.8 + 40
			if float64(stage.W) < minWidth {
				t.Errorf("preview width %v cannot show a readable A4 width, want at least %v", stage.W, minWidth)
			}
			if indicator.Y < stage.Y+stage.H-48 || indicator.X+indicator.W > stage.X+stage.W-12 {
				t.Errorf("page indicator %v is not parked in the tray margin", indicator)
			}
		})
	}
	if pageCountColor(1) != carbonInk || pageCountColor(2) != proofVermilion {
		t.Error("the page indicator colors do not follow One-Signal-Per-State")
	}
}

func TestWorkspaceKeyboardAndAccessibilitySemantics(t *testing.T) {
	app, _, tt := newHarness(t, "")
	tt.Frame()

	if _, ok := tt.AccessByRole(ui.RoleMain, "简历编辑"); !ok {
		t.Error("no editor main landmark")
	}
	if _, ok := tt.AccessByRole(ui.RoleComplementary, "A4 预览"); !ok {
		t.Error("no preview complementary landmark")
	}
	editor, ok := tt.AccessByRole(ui.RoleTextField, "Markdown 源文")
	if !ok {
		t.Fatal("no accessible source editor")
	}
	if !editor.Focusable || !editor.Focused {
		t.Errorf("source editor access = %+v, want focusable and focused", editor)
	}
	open, ok := tt.AccessByRole(ui.RoleButton, "打开 Markdown 文件")
	if !ok || !open.Focusable || open.KeyShortcuts == "" {
		t.Errorf("open button access = %+v, %v", open, ok)
	}
	save, ok := tt.AccessByRole(ui.RoleButton, "保存")
	if !ok || !save.Disabled {
		t.Errorf("save button access = %+v, %v; want disabled without a path", save, ok)
	}
	if !tt.Focused("Markdown 源文") {
		t.Error("the editor did not receive initial keyboard focus")
	}

	app.buffer.Reset("# 标题\n")
	tt.Frame()
	if !tt.HasText("1 / 1") {
		t.Error("the page counter is not visible")
	}

	toggled := 0
	app.SetSidebarToggle(func() { toggled++ })
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleButton, "文档栏"); !ok {
		t.Fatal("the sidebar extension hook did not add its toolbar control")
	}
	if err := tt.Click("文档栏"); err != nil {
		t.Fatal(err)
	}
	if toggled != 1 {
		t.Errorf("sidebar toggle ran %d times, want 1", toggled)
	}
}
