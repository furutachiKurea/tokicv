package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"tokicv/internal/dialect"
	"tokicv/internal/library"
	"tokicv/internal/render"
)

// PreviewDebounce is how long editing rests before the DOM preview changes.
const PreviewDebounce = 180 * time.Millisecond

const topBarHeight float32 = 46

// Preview shows generated preview HTML and embeds its rendering surface.
type Preview interface {
	Show(frame render.PreviewFrame)
	Update(frame render.PreviewFrame)
	View(c *ui.Context) ui.Element
}

// Options configure the desktop authoring workspace.
type Options struct {
	Path        string
	Layout      render.Layout
	Store       *library.Store
	LayoutStore *library.Store
}

// App is the Editorial Proofing Desk authoring state. Its view runs on the
// main thread and stores no UI elements between builds.
type App struct {
	buffer             *editor.Buffer
	sourceEditor       editor.State
	editorFocus        ui.Handle
	layout             render.Layout
	baseLayout         render.Layout
	fallbackLayout     render.Layout
	layoutOrigin       library.LayoutOrigin
	layoutStore        *library.Store
	layoutMenuOpen     bool
	layoutNotice       string
	layoutFields       map[string]*layoutFieldState
	theme              *ui.Theme
	preview            Preview
	previewHTML        string
	previewPending     bool
	previewDue         time.Time
	splitShare         float32
	path               string
	store              *library.Store
	documents          []library.Document
	currentID          string
	currentName        string
	railOpen           bool
	railReady          bool
	railNarrow         bool
	railScroll         ui.ScrollState
	libraryRequest     uint64
	renameOpen         bool
	renameValue        string
	renameID           string
	deleteOpen         bool
	deleteID           string
	newMenuOpen        bool
	analysis           dialect.Analysis
	analysisVersion    uint64
	analyzed           bool
	diagnosticErrors   int
	pageCount          int
	picker             func() (string, error)
	onTitle            func(string)
	onInvalidate       func()
	sidebarToggle      func()
	update             func(func())
	updateMu           sync.Mutex
	pendingUpdates     []func()
	fileIO             sync.WaitGroup
	writeMu            sync.Mutex
	writeTails         map[string]chan struct{}
	readFile           func(string) ([]byte, error)
	writeFile          func(string, editor.Snapshot) error
	openRequest        uint64
	saveRequest        uint64
	document           uint64
	closed             bool
	fileError          string
	artifactError      string
	exportMenuOpen     bool
	exportPaths        map[ArtifactKind]func(string) (string, error)
	artifacts          ArtifactPipeline
	smartOnePage       bool
	smartSolving       bool
	smartRequest       uint64
	smartCancel        context.CancelFunc
	smartFitVersion    uint64
	smartRefitPending  bool
	smartRefitDue      time.Time
	smartSolveDuration time.Duration
	layoutRevision     uint64
	rawSource          []byte
	rawVersion         uint64
	updateContent      func(*library.Store, string, []byte) (library.Document, error)
	work               sync.WaitGroup
	workMu             sync.Mutex
	workNext           uint64
	workRunning        int
	workCancels        map[uint64]context.CancelFunc
	workSealed         bool
}

// New creates an authoring workspace.
func New(opts Options) *App {
	if opts.Layout == (render.Layout{}) {
		opts.Layout = render.DefaultLayout()
	}
	layout := opts.Layout.Normalized()
	layoutStore := opts.LayoutStore
	if layoutStore == nil {
		layoutStore = opts.Store
	}
	buffer := editor.NewBuffer("")
	buffer.SetLanguage(resumeLanguage())
	a := &App{
		buffer:         buffer,
		layout:         layout,
		baseLayout:     layout,
		fallbackLayout: layout,
		layoutStore:    layoutStore,
		layoutOrigin:   library.LayoutOriginDefault,
		store:          opts.Store,
		theme:          shellTheme(),
		pageCount:      1,
		splitShare:     defaultEditorShare,
		readFile:       os.ReadFile,
		writeFile:      writeDocument,
		workCancels:    make(map[uint64]context.CancelFunc),
		layoutFields:   make(map[string]*layoutFieldState),
		sourceEditor: editor.State{
			Buffer:         buffer,
			Font:           ui.Font{Family: editorFontFamily, Size: 13, LineHeight: 1.65},
			SoftWrap:       true,
			NoFolding:      true,
			NoIndentGuides: true,
			NoActiveLine:   true,
			NoAutoClose:    true,
			KeepScrollbars: true,
			Label:          "Markdown 源文",
			Theme:          resumeEditorTheme(),
		},
	}
	if a.store != nil {
		a.loadStoreDocuments()
		if opts.Path == "" && len(a.documents) > 0 {
			first := a.documents[0]
			if _, source, err := a.store.Show(first.ID); err == nil {
				a.loadLibraryDocument(first, string(source))
			} else {
				a.fileError = "打开失败"
			}
		}
	}
	a.refreshAnalysis(buffer.Version())
	return a
}

// Path returns the external Markdown path being edited, if any.
func (a *App) Path() string { return a.path }

// PageCount returns the latest Sheet count reported by the DOM preview.
func (a *App) PageCount() int { return a.pageCount }

// SetPreview attaches the preview surface.
func (a *App) SetPreview(preview Preview) {
	a.preview = preview
	a.ensureAnalysis()
	frame := render.PreviewFrameFor(a.analysis.Document, a.layout)
	a.previewHTML = frame.HTML
	a.previewPending = false
	if preview != nil {
		preview.Show(frame)
	}
}

// SetPicker sets the external Markdown file chooser.
func (a *App) SetPicker(picker func() (string, error)) {
	a.picker = picker
}

// SetArtifactPipeline connects the shared hidden-page export pipeline.
func (a *App) SetArtifactPipeline(pipeline ArtifactPipeline) {
	a.artifacts = pipeline
	a.invalidate()
}

// SetExportPath sets the destination chooser for one Artifact kind.
func (a *App) SetExportPath(kind ArtifactKind, choose func(defaultName string) (string, error)) {
	if a.exportPaths == nil {
		a.exportPaths = make(map[ArtifactKind]func(string) (string, error))
	}
	a.exportPaths[kind] = choose
}

// SetMarkdownExportPath sets the destination chooser for raw Markdown
// Artifact export.
func (a *App) SetMarkdownExportPath(choose func(defaultName string) (string, error)) {
	a.SetExportPath(ArtifactMarkdown, choose)
}

// SetTitleHandler receives the native window title as the open path changes.
func (a *App) SetTitleHandler(fn func(string)) {
	a.onTitle = fn
	a.notifyTitle()
}

// SetInvalidate receives a redraw request.
func (a *App) SetInvalidate(fn func()) {
	a.onInvalidate = fn
}

// SetSidebarToggle connects Ticket 08's document rail toggle.
func (a *App) SetSidebarToggle(fn func()) {
	a.sidebarToggle = fn
}

// SetUpdate connects background file operations to the window's main-thread
// update queue.
func (a *App) SetUpdate(update func(func())) {
	a.updateMu.Lock()
	a.update = update
	a.updateMu.Unlock()
}

// SetPageCount is called when the DOM preview reports its Sheet count.
func (a *App) SetPageCount(n int) {
	if n < 1 {
		n = 1
	}
	if n == a.pageCount {
		return
	}
	a.pageCount = n
	a.invalidate()
}

// Open reads an external Markdown file for in-place editing in the
// background.
func (a *App) Open(path string) error {
	if a.closed {
		return errors.New("工作区已关闭")
	}
	absolute, err := markdownPath(path)
	if err != nil {
		return err
	}
	a.openRequest++
	request := a.openRequest
	version := a.buffer.Version()
	readFile := a.readFile
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		data, err := readFile(absolute)
		a.post(func() {
			a.finishOpen(request, version, absolute, data, err)
		})
	}()
	return nil
}

// Save writes the editor text back to the open Resume Document in the
// background.
func (a *App) Save() error {
	if a.closed {
		return errors.New("工作区已关闭")
	}
	if !a.canSave() {
		return nil
	}
	a.saveRequest++
	request := a.saveRequest
	path := a.path
	id := ""
	if path == "" {
		id = a.currentID
	}
	store := a.store
	document := a.document
	version := a.buffer.Version()
	snapshot := a.buffer.Snapshot()
	source := []byte(snapshot.String())
	writeFile := a.writeFile
	updateContent := a.updateContent
	key := documentWriteKey(path, id)
	previous, done := a.enqueueWrite(key)
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		defer a.finishWrite(key, done)
		if previous != nil {
			<-previous
		}
		err := persistDocument(store, path, id, snapshot, true, writeFile, updateContent)
		a.post(func() {
			a.finishSave(request, document, path, id, version, source, err)
		})
	}()
	return nil
}

func (a *App) canSave() bool {
	return a.path != "" || a.currentID != "" && a.store != nil
}

func documentWriteKey(path, id string) string {
	if path != "" {
		return "path:" + path
	}
	if id != "" {
		return "library:" + id
	}
	return ""
}

func (a *App) enqueueWrite(key string) (<-chan struct{}, chan struct{}) {
	if key == "" {
		return nil, nil
	}
	done := make(chan struct{})
	a.writeMu.Lock()
	if a.writeTails == nil {
		a.writeTails = make(map[string]chan struct{})
	}
	previous := a.writeTails[key]
	a.writeTails[key] = done
	a.writeMu.Unlock()
	return previous, done
}

func (a *App) finishWrite(key string, done chan struct{}) {
	if done == nil {
		return
	}
	close(done)
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	if a.writeTails[key] == done {
		delete(a.writeTails, key)
	}
}

func (a *App) finishOpen(request, version uint64, path string, data []byte, err error) {
	if a.closed || request != a.openRequest || a.buffer.Version() != version {
		return
	}
	if err != nil {
		logDesktopError("open", err)
		a.setFileError("打开失败")
		return
	}
	a.loadDocument(path, string(data))
	a.setFileError("")
}

func (a *App) finishSave(request, document uint64, path, id string, version uint64,
	source []byte, err error) {
	if a.closed || request != a.saveRequest || document != a.document ||
		path != a.path || id != a.currentID {
		return
	}
	if err != nil {
		logDesktopError("save", err)
		a.setFileError("保存失败")
		return
	}
	if a.buffer.Version() == version {
		a.buffer.MarkSaved()
		a.rawSource = source
		a.rawVersion = version
	}
	a.setFileError("")
}

func (a *App) loadDocument(path, source string) {
	a.buffer.Reset(source)
	a.rawSource = []byte(source)
	a.rawVersion = a.buffer.Version()
	a.path = path
	a.currentID = ""
	a.currentName = ""
	a.document++
	a.loadEffectiveLayout()
	a.ensureAnalysis()
	a.previewPending = true
	a.previewDue = time.Time{}
	a.notifyTitle()
	a.editorFocus.Focus()
	a.invalidate()
}

func (a *App) setFileError(message string) {
	if a.fileError == message {
		return
	}
	a.fileError = message
	a.invalidate()
}

func (a *App) saveFromUI() {
	if err := a.Save(); err != nil {
		logDesktopError("save", err)
		a.setFileError("保存失败")
	}
}

func (a *App) close() {
	if a.closed {
		return
	}
	if a.path == "" && a.currentID != "" && a.store != nil &&
		(a.buffer.Modified() || a.buffer.Version() != a.rawVersion) {
		_ = a.Save()
	}
	a.closed = true
	a.openRequest++
	a.saveRequest++
	a.smartRequest++
	if a.smartCancel != nil {
		a.smartCancel()
		a.smartCancel = nil
	}
	a.cancelWork()
}

// CloseAndWait closes the workspace and waits for queued file operations.
func (a *App) CloseAndWait() {
	a.close()
	a.fileIO.Wait()
	a.waitForWork(workCloseGrace)
}

func (a *App) post(fn func()) {
	a.updateMu.Lock()
	update := a.update
	if update == nil {
		a.pendingUpdates = append(a.pendingUpdates, fn)
		a.updateMu.Unlock()
		return
	}
	a.updateMu.Unlock()
	update(fn)
}

func (a *App) drainUpdates() {
	a.fileIO.Wait()
	for {
		a.updateMu.Lock()
		pending := a.pendingUpdates
		a.pendingUpdates = nil
		a.updateMu.Unlock()
		if len(pending) == 0 {
			return
		}
		for _, fn := range pending {
			fn()
		}
	}
}

// View builds the Editorial Proofing Desk.
func (a *App) View(c *ui.Context) {
	c.SetTheme(a.theme)
	a.syncPreview(c)
	c.OnWindowShortcut(ui.Cmd, ui.KeyO, a.pickAndOpen)
	c.OnWindowShortcut(ui.Cmd, ui.KeyS, a.saveFromUI)

	ui.Column(c.Key("shell")).Fill().Background(ashShell).Children(func() {
		a.topBar(c)
		a.workspace(c)
	})
	a.dialogs(c)
}

func (a *App) topBar(c *ui.Context) {
	ui.Row(c.Key("topbar")).Height(topBarHeight).FillWidth().
		Padding(0, 12).Gap(8).AlignItems(ui.Center).
		Background(toolbarLinen).
		BorderWidth(0, 0, 1, 0).BorderColor(hairlineStone).
		Role(ui.RoleBanner).Label("tokicv 工具栏").
		Children(func() {
			if a.store != nil || a.path != "" || a.sidebarToggle != nil {
				iconButton(c, "sidebar-toggle", "文档栏", "文档栏", sidebarIcon, 0, 0, false, a.toggleRail)
			}
			ui.Text(c, "tokicv").Font(uiFontFamily).FontSize(13).FontWeight(600).TextColor(carbonInk)
			if a.store != nil || a.path != "" {
				newButton := iconButton(c, "new-document", "新建", "新建", newIcon,
					0, 0, false, func() { a.newMenuOpen = !a.newMenuOpen })
				newMenu := ui.Popover(c.Key("new-menu"), newButton, &a.newMenuOpen, func() {
					ui.Column(c.Key("new-menu-content")).Width(196).Gap(2).Padding(4).
						Children(func() {
							ui.Button(c.Key("new-blank"), "新建空白简历").FillWidth().
								Justify(ui.Start).OnClick(func() {
								a.newMenuOpen = false
								a.createBlankDocument()
							})
							ui.Button(c.Key("new-starter"), "从示例模板创建").FillWidth().
								Justify(ui.Start).OnClick(func() {
								a.newMenuOpen = false
								a.createStarterDocument()
							})
						})
				})
				if newMenu.Valid() {
					newMenu.Transition(popoverTransition())
				}
			}
			if a.path != "" {
				ui.Text(c, filepath.Base(a.path)).FontSize(12).TextColor(mutedGraphite).
					SingleLine().MaxWidth(280).Tooltip(a.path)
			}
			iconButton(c, "open-file", "打开 Markdown 文件", "打开", openIcon,
				ui.Cmd, ui.KeyO, false, a.pickAndOpen)
			iconButton(c, "save-file", "保存", "保存", saveIcon,
				ui.Cmd, ui.KeyS, !a.canSave(), a.saveFromUI)
			if a.path != "" {
				iconButton(c, "import-library", "存入简历库", "存入简历库", importIcon,
					0, 0, false, a.importExternalDocument)
			}
			ui.Row(c).Grow(1)
			ui.Row(c.Key("toolbar-right")).Gap(8).AlignItems(ui.Center).Children(func() {
				if a.fileError != "" {
					ui.Text(c.Key("file-error"), a.fileError).
						FontSize(12).TextColor(proofVermilion).Role(ui.RoleAlert).
						SingleLine().MaxWidth(160)
				}
				if a.diagnosticErrors > 0 {
					ui.Text(c.Key("syntax-errors"), fmt.Sprintf("%d 处语法错误", a.diagnosticErrors)).
						FontSize(12).TextColor(proofVermilion).Role(ui.RoleStatus).
						SingleLine().MaxWidth(120)
				}
				a.layoutNoticeText(c)
				a.artifactErrorText(c)
				a.smartOnePageToggle(c)
				a.layoutPopover(c)
				a.exportMenu(c)
			})
		})
}

func (a *App) editorPane(c *ui.Context) {
	focused := a.editorFocus.Focused(c)
	border := ui.Transparent
	if focused {
		border = editorialCobalt
	}
	ui.Box(c.Key("editor-pane")).Fill().
		Padding(20, 24).Background(manuscriptPaper).
		Border(1, border).
		Role(ui.RoleMain).Label("简历编辑").
		Children(func() {
			editor.View(c.Key("source-editor"), &a.sourceEditor).
				Fill().Bind(&a.editorFocus).AutoFocus()
		})
}

func (a *App) previewPane(c *ui.Context) {
	ui.Box(c.Key("preview-stage")).Fill().
		Background(proofingStage).
		Role(ui.RoleComplementary).Label("A4 预览").
		Children(func() {
			if a.preview != nil {
				a.preview.View(c)
			} else {
				ui.Box(c.Key("preview-empty")).Fill().Background(proofingStage)
			}
			ui.Row(c.Key("page-count")).Absolute().Right(16).Bottom(12).
				Height(24).Padding(0, 10).Radius(12).
				Background(manuscriptPaper).Border(1, hairlineStone).
				AlignItems(ui.Center).Children(func() {
				count := ui.Text(c.Key("page-count-text"), fmt.Sprintf("1 / %d", a.pageCount)).
					Font(monoFontFamily).FontFeatures("tnum").
					FontSize(11).FontWeight(500).
					Role(ui.RoleStatus)
				count.TextColor(count.AnimateColor("page-count-color",
					pageCountColor(a.pageCount), stateColorAnimation()))
			})
		})
}

func (a *App) syncPreview(c *ui.Context) {
	version := a.buffer.Version()
	if !a.analyzed || version != a.analysisVersion {
		a.refreshAnalysis(version)
		a.previewPending = true
		if a.previewHTML == "" {
			a.previewDue = time.Time{}
		} else {
			a.previewDue = c.Now().Add(PreviewDebounce)
		}
		if a.smartOnePage && !a.smartSolving && version != a.smartFitVersion &&
			!a.smartRefitPending {
			a.smartRefitPending = true
			a.smartRefitDue = c.Now().Add(PreviewDebounce)
		}
	}
	if !a.previewPending {
		return
	}
	if a.previewDue.IsZero() || !c.Now().Before(a.previewDue) {
		a.previewPending = false
		frame := render.PreviewFrameFor(a.analysis.Document, a.layout)
		a.previewHTML = frame.HTML
		if a.preview != nil {
			a.preview.Update(frame)
		}
		if a.smartRefitPending &&
			(a.smartRefitDue.IsZero() || !c.Now().Before(a.smartRefitDue)) {
			a.smartRefitPending = false
			a.startSmartOnePageSolve()
		}
		return
	}
	c.After(a.previewDue.Sub(c.Now()))
}

func (a *App) ensureAnalysis() {
	if !a.analyzed || a.buffer.Version() != a.analysisVersion {
		a.refreshAnalysis(a.buffer.Version())
	}
}

func (a *App) refreshAnalysis(version uint64) {
	a.analysis = dialect.Analyze(a.buffer.String())
	a.analysisVersion = version
	a.analyzed = true

	marks := make([]editor.GutterMark, 0, len(a.analysis.Diagnostics))
	seen := make(map[int]bool)
	for _, diagnostic := range a.analysis.Diagnostics {
		if diagnostic.Line < 1 || diagnostic.Line > a.buffer.LineCount() {
			continue
		}
		line := diagnostic.Line - 1
		if !seen[line] {
			seen[line] = true
			marks = append(marks, editor.GutterMark{Line: line, Color: proofVermilion})
		}
	}
	a.sourceEditor.GutterMarks = marks
	a.diagnosticErrors = 0
	for _, diagnostic := range a.analysis.Diagnostics {
		if diagnostic.Severity == dialect.SeverityError {
			a.diagnosticErrors++
		}
	}
	a.sourceEditor.SetDiagnostics(version, editorDiagnostics(a.buffer, a.analysis.Diagnostics))
}

func (a *App) pickAndOpen() {
	if a.picker == nil {
		return
	}
	path, err := a.picker()
	if err != nil || path == "" {
		return
	}
	if err := a.openFromUI(path); err != nil {
		logDesktopError("open", err)
		a.setFileError("打开失败")
	}
}

func (a *App) notifyTitle() {
	if a.onTitle == nil {
		return
	}
	title := "tokicv"
	if a.path != "" {
		title += " — " + filepath.Base(a.path)
	}
	a.onTitle(title)
}

func (a *App) invalidate() {
	if a.onInvalidate != nil {
		a.onInvalidate()
	}
}

func pageCountColor(n int) ui.Color {
	if n > 1 {
		return proofVermilion
	}
	return carbonInk
}

func markdownPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("缺少 Markdown 路径")
	}
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return "", fmt.Errorf("不是 Markdown 文件: %s", path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析路径 %q: %w", path, err)
	}
	return absolute, nil
}

func writeDocument(path string, snapshot editor.Snapshot) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("保存 Resume Document %q: %w", path, err)
	}
	if _, err := snapshot.WriteTo(file); err != nil {
		file.Close()
		return fmt.Errorf("保存 Resume Document %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("保存 Resume Document %q: %w", path, err)
	}
	return nil
}

func iconButton(c *ui.Context, key, label, tooltip string, icon *ui.SVG,
	mods ui.Modifiers, shortcut ui.Key, disabled bool, onClick func()) ui.Element {
	button := ui.ButtonBase(c.Key(key)).Size(28, 28).Radius(4).
		Label(label).Tooltip(tooltip).Disabled(disabled).
		FocusRing(true).FocusRingColor(editorialCobalt).FocusRingOffset(-1)
	if button.Hovered() && !disabled {
		button.Background(sidebarTray)
	}
	if button.Pressed() && !disabled {
		button.Background(hairlineStone)
	}
	if shortcut != 0 {
		button.KeyShortcuts(mods, shortcut)
	}
	if onClick != nil {
		button.OnClick(onClick)
	}
	button.Children(func() {
		ui.Icon(c, icon).FontSize(14).TextColor(carbonInk)
	})
	return button
}
