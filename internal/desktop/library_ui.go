package desktop

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"tokicv/internal/library"
)

const documentRailWidth float32 = 224

func (a *App) workspace(c *ui.Context) {
	a.syncRail(c)
	width, _ := c.Size()
	workspace := ui.Row(c.Key("workspace")).Grow(1).AlignItems(ui.Stretch)
	open := a.railVisible()
	target := float32(0)
	if open {
		target = 1
	}
	progress := workspace.AnimateTo("document-rail-progress", target, railAnimation(open))
	inFlow := a.railCanFlow(width)
	railWidth := float32(0)
	if inFlow {
		railWidth = progress * documentRailWidth
	}
	workspace.Children(func() {
		if railWidth > 0.01 {
			a.documentRail(c, railWidth, a.railOpen)
		}
		if a.emptyLibrary() {
			a.emptyLibraryOnboarding(c)
			return
		}
		a.workbench(c, width-railWidth)
	})
	if !inFlow && progress > 0.001 {
		a.documentRailOverlay(c, progress)
	}
}

func (a *App) syncRail(c *ui.Context) {
	width, _ := c.Size()
	narrow := width <= 1280
	if !a.railReady {
		a.railOpen = false
		a.railNarrow = narrow
		a.railReady = true
		return
	}
	if narrow != a.railNarrow {
		a.railNarrow = narrow
		if narrow {
			a.railOpen = false
		}
	}
}

func (a *App) railVisible() bool {
	return a.store != nil && len(a.documents) > 0 && a.railOpen
}

func (a *App) railCanFlow(width float32) bool {
	return a.store != nil && len(a.documents) > 0 && width >= documentRailFlowMinWidth
}

func (a *App) emptyLibrary() bool {
	return a.store != nil && a.path == "" && a.currentID == "" && len(a.documents) == 0
}

func (a *App) emptyLibraryOnboarding(c *ui.Context) {
	ui.Column(c.Key("empty-library")).Grow(1).FillHeight().Center().Gap(10).
		Background(ashShell).Role(ui.RoleMain).Label("空简历库").
		Children(func() {
			a.onboardingAction(c, "onboarding-blank", "新建空白简历", newIcon, a.createBlankDocument)
			a.onboardingAction(c, "onboarding-starter", "从示例模板创建", templateIcon, a.createStarterDocument)
			a.onboardingAction(c, "onboarding-open", "打开本地 .md 文件", folderIcon, a.pickAndOpen)
		})
}

func (a *App) onboardingAction(c *ui.Context, key, label string, icon *ui.SVG, onClick func()) {
	button := ui.ButtonBase(c.Key(key)).Width(360).Height(54).Padding(0, 14).
		Gap(12).AlignItems(ui.Center).Radius(4).
		Background(manuscriptPaper).Border(1, hairlineStone).
		FocusRing(true).FocusRingColor(editorialCobalt).FocusRingOffset(1).
		Label(label)
	if button.Hovered() {
		button.Background(sidebarTray)
	}
	if button.Pressed() {
		button.Background(hairlineStone)
	}
	button.OnClick(onClick)
	button.Children(func() {
		ui.Icon(c, icon).FontSize(17).TextColor(mutedGraphite)
		ui.Text(c, label).Font(uiFontFamily).FontSize(13).FontWeight(500).TextColor(carbonInk)
	})
}

func (a *App) documentRail(c *ui.Context, width float32, interactive bool) {
	rail := ui.Row(c.Key("document-rail")).Width(width).Shrink(0).FillHeight().
		ClipX().Background(sidebarTray).
		BorderWidth(0, 0, 0, 1).BorderColor(hairlineStone).
		Role(ui.RoleNavigation).Label("文档栏").IgnoreInput(!interactive)
	if !interactive {
		rail.AccessHidden(true)
	}
	rail.Children(func() {
		ui.Column(c.Key("document-rail-content")).Width(documentRailWidth-1).Shrink(0).
			FillHeight().Padding(8, 10).Children(func() {
			a.documentRailBody(c)
		})
	})
}

func (a *App) documentRailOverlay(c *ui.Context, progress float32) {
	ui.Overlay(c, func() {
		back := ui.Box(c.Key("document-rail-scrim")).Absolute().
			Left(0).Top(topBarHeight).Right(0).Bottom(0).
			Background(ui.RGBA(24, 25, 27, 0.18*progress)).
			Role(ui.RoleNone).AccessHidden(true).IgnoreInput(!a.railOpen)
		back.OnClick(a.closeRail)
		if back.OverlayShortcut(0, ui.KeyEscape) {
			a.closeRail()
		}
		panel := ui.Column(c.Key("document-rail")).Absolute().
			Left(-(1-progress)*documentRailWidth).
			Top(topBarHeight).Bottom(0).Width(documentRailWidth).
			Padding(8, 10).Background(sidebarTray).
			BorderWidth(0, 0, 0, 1).BorderColor(hairlineStone).
			Shadow(6, 0, 20, 0, ui.RGBA(24, 25, 27, 0.16)).
			Role(ui.RoleNavigation).Label("文档栏").
			IgnoreInput(!a.railOpen)
		if !a.railOpen {
			panel.AccessHidden(true)
		}
		panel.OnClick(func() {})
		panel.Children(func() {
			a.documentRailBody(c)
		})
	})
}

func (a *App) documentRailBody(c *ui.Context) {
	ui.Scroll(c.Key("document-list")).Grow(1).MinHeight(0).Gap(4).
		TrackScroll(&a.railScroll).Children(func() {
		for _, document := range a.documents {
			a.documentRow(c, document)
		}
	})
	ui.Row(c.Key("document-actions")).Height(36).FillWidth().Padding(8, 0, 0, 0).
		Gap(6).AlignItems(ui.End).
		BorderWidth(1, 0, 0, 0).BorderColor(hairlineStone).
		Children(func() {
			iconButton(c, "rail-new-blank", "新建空白简历", "新建空白简历",
				newIcon, 0, 0, false, a.createBlankDocument)
			iconButton(c, "rail-new-starter", "从示例模板创建", "从示例模板创建",
				templateIcon, 0, 0, false, a.createStarterDocument)
			current := a.currentID != "" && a.path == ""
			iconButton(c, "rail-rename", "重命名", "重命名",
				pencilIcon, 0, 0, !current, a.beginRename)
			iconButton(c, "rail-delete", "删除", "删除",
				trashIcon, 0, 0, !current, a.beginDelete)
		})
}

func (a *App) closeRail() {
	if !a.railOpen {
		return
	}
	a.railOpen = false
	a.railReady = true
	a.invalidate()
}

func (a *App) documentRow(c *ui.Context, document library.Document) {
	active := document.ID == a.currentID
	row := ui.ButtonBase(c.Key("document-"+document.ID)).FillWidth().Height(38).
		Padding(0, 10).Radius(4).AlignItems(ui.Center).MinWidth(0).
		Role(ui.RoleListItem).Label(document.Name).FocusRing(true).
		FocusRingColor(editorialCobalt).FocusRingOffset(1).AccessSelected(active)
	if active {
		row.Background(manuscriptPaper).Border(1, activeCardEdge).
			Shadow(0, 1, 3, 0, ui.RGBA(24, 25, 27, 0.06))
	} else {
		row.Background(ui.Transparent).Border(1, ui.Transparent)
		if row.Hovered() {
			row.Background(ui.RGBA(251, 251, 249, 0.48))
		}
	}
	row.OnClick(func() { a.switchToLibraryDocument(document) })
	row.Children(func() {
		ui.Text(c, document.Name).Font(uiFontFamily).FontSize(12.5).
			TextColor(carbonInk).SingleLine().Ellipsis("…").
			Grow(1).MinWidth(0)
	})
}

func (a *App) loadStoreDocuments() {
	if a.store == nil {
		a.documents = nil
		return
	}
	documents, err := a.store.List()
	if err != nil {
		a.fileError = "读取简历库失败"
		return
	}
	a.documents = documents
}

func (a *App) createBlankDocument() {
	a.createDocument(a.nextDocumentName("未命名简历"), "# 未命名简历\n\n")
}

func (a *App) createStarterDocument() {
	name := a.nextDocumentName("软件工程师简历")
	a.createDocument(name, library.StarterMarkdown())
}

func (a *App) createDocument(name, source string) {
	store := a.store
	snapshot := a.buffer.Snapshot()
	modified := a.buffer.Modified()
	path := a.path
	currentID := a.currentID
	writeFile := a.writeFile
	updateContent := a.updateContent
	var previous <-chan struct{}
	var writeDone chan struct{}
	writeKey := ""
	if modified {
		writeKey = documentWriteKey(path, currentID)
		previous, writeDone = a.enqueueWrite(writeKey)
	}
	version := a.buffer.Version()
	document := a.document
	a.libraryRequest++
	request := a.libraryRequest
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		defer a.finishWrite(writeKey, writeDone)
		if previous != nil {
			<-previous
		}
		if err := persistDocument(store, path, currentID, snapshot, modified, writeFile, updateContent); err != nil {
			a.post(func() { a.finishLibraryRequest(request, "保存失败") })
			return
		}
		if store == nil {
			opened, err := library.Open()
			if err != nil {
				a.post(func() { a.finishLibraryRequest(request, "新建失败") })
				return
			}
			store = opened
		}
		documents, err := store.List()
		if err != nil {
			a.post(func() { a.finishLibraryRequest(request, "新建失败") })
			return
		}
		created, err := store.Create(uniqueDocumentName(name, documents), []byte(source))
		if err != nil {
			a.post(func() { a.finishLibraryRequest(request, "新建失败") })
			return
		}
		documents, err = store.List()
		if err != nil {
			a.post(func() { a.finishLibraryRequest(request, "读取简历库失败") })
			return
		}
		a.post(func() {
			if a.closed || request != a.libraryRequest {
				return
			}
			a.store = store
			if a.layoutStore == nil {
				a.layoutStore = store
			}
			a.documents = documents
			if version != a.buffer.Version() || document != a.document {
				return
			}
			a.loadLibraryDocument(created, source)
			a.setFileError("")
		})
	}()
}

func (a *App) finishLibraryRequest(request uint64, message string) {
	if a.closed || request != a.libraryRequest {
		return
	}
	a.setFileError(message)
}

func (a *App) loadLibraryDocument(document library.Document, source string) {
	a.buffer.Reset(source)
	a.rawSource = []byte(source)
	a.rawVersion = a.buffer.Version()
	a.path = ""
	a.currentID = document.ID
	a.currentName = document.Name
	a.document++
	a.loadEffectiveLayout()
	a.ensureAnalysis()
	a.previewPending = true
	a.previewDue = time.Time{}
	a.notifyTitle()
	a.editorFocus.Focus()
	a.invalidate()
}

func (a *App) switchToLibraryDocument(document library.Document) {
	if a.store == nil || document.ID == a.currentID && a.path == "" {
		return
	}
	store := a.store
	snapshot := a.buffer.Snapshot()
	modified := a.buffer.Modified()
	path := a.path
	currentID := a.currentID
	writeFile := a.writeFile
	updateContent := a.updateContent
	var previous <-chan struct{}
	var writeDone chan struct{}
	writeKey := ""
	if modified {
		writeKey = documentWriteKey(path, currentID)
		previous, writeDone = a.enqueueWrite(writeKey)
	}
	version := a.buffer.Version()
	documentToken := a.document
	a.libraryRequest++
	request := a.libraryRequest
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		defer a.finishWrite(writeKey, writeDone)
		if previous != nil {
			<-previous
		}
		if err := persistDocument(store, path, currentID, snapshot, modified, writeFile, updateContent); err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("保存失败")
			})
			return
		}
		loaded, source, err := store.Show(document.ID)
		if err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("打开失败")
			})
			return
		}
		documents, err := store.List()
		if err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("读取简历库失败")
			})
			return
		}
		a.post(func() {
			if a.closed || request != a.libraryRequest ||
				version != a.buffer.Version() || documentToken != a.document {
				return
			}
			a.documents = documents
			a.loadLibraryDocument(loaded, string(source))
			a.setFileError("")
		})
	}()
}

func (a *App) openFromUI(path string) error {
	absolute, err := markdownPath(path)
	if err != nil {
		return err
	}
	a.libraryRequest++
	if !a.buffer.Modified() {
		return a.Open(absolute)
	}
	store := a.store
	snapshot := a.buffer.Snapshot()
	currentPath := a.path
	currentID := a.currentID
	writeFile := a.writeFile
	updateContent := a.updateContent
	readFile := a.readFile
	writeKey := documentWriteKey(currentPath, currentID)
	previous, writeDone := a.enqueueWrite(writeKey)
	version := a.buffer.Version()
	documentToken := a.document
	a.openRequest++
	request := a.openRequest
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		defer a.finishWrite(writeKey, writeDone)
		if previous != nil {
			<-previous
		}
		if err := persistDocument(store, currentPath, currentID,
			snapshot, true, writeFile, updateContent); err != nil {
			a.post(func() {
				if a.closed || request != a.openRequest {
					return
				}
				a.setFileError("保存失败")
			})
			return
		}
		data, err := readFile(absolute)
		a.post(func() {
			if a.closed || request != a.openRequest ||
				version != a.buffer.Version() || documentToken != a.document {
				return
			}
			if err != nil {
				a.setFileError("打开失败")
				return
			}
			a.loadDocument(absolute, string(data))
			a.setFileError("")
		})
	}()
	return nil
}

func persistDocument(store *library.Store, path, id string, snapshot editor.Snapshot,
	modified bool, writeFile func(string, editor.Snapshot) error,
	updateContent func(*library.Store, string, []byte) (library.Document, error)) error {
	if !modified {
		return nil
	}
	if path != "" {
		return writeFile(path, snapshot)
	}
	if id != "" {
		if updateContent == nil {
			updateContent = func(store *library.Store, id string, source []byte) (library.Document, error) {
				return store.UpdateContent(id, source)
			}
		}
		_, err := updateContent(store, id, []byte(snapshot.String()))
		return err
	}
	return nil
}

func (a *App) nextDocumentName(base string) string {
	return uniqueDocumentName(base, a.documents)
}

func uniqueDocumentName(base string, documents []library.Document) string {
	used := make(map[string]bool, len(documents))
	for _, document := range documents {
		used[document.Name] = true
	}
	if !used[base] {
		return base
	}
	for n := 2; ; n++ {
		name := fmt.Sprintf("%s %d", base, n)
		if !used[name] {
			return name
		}
	}
}

func (a *App) hasDocument() bool {
	return a.path != "" || a.currentID != ""
}

func (a *App) importExternalDocument() {
	if a.path == "" {
		return
	}
	store := a.store
	path := a.path
	source := a.exactSource()
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.TrimSpace(base) == "" {
		base = "导入简历"
	}
	version := a.buffer.Version()
	document := a.document
	a.libraryRequest++
	request := a.libraryRequest
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		if store == nil {
			opened, err := library.Open()
			if err != nil {
				a.post(func() {
					if a.closed || request != a.libraryRequest {
						return
					}
					a.setFileError("导入失败")
				})
				return
			}
			store = opened
		}
		documents, err := store.List()
		if err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("导入失败")
			})
			return
		}
		created, err := store.Create(uniqueDocumentName(base, documents), source)
		if err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("导入失败")
			})
			return
		}
		documents, err = store.List()
		if err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("导入失败")
			})
			return
		}
		a.post(func() {
			if a.closed || request != a.libraryRequest {
				return
			}
			a.store = store
			if a.layoutStore == nil {
				a.layoutStore = store
			}
			a.documents = documents
			if version != a.buffer.Version() || document != a.document {
				return
			}
			a.loadLibraryDocument(created, string(source))
			a.setFileError("")
		})
	}()
}

func (a *App) markdownExportName() string {
	if a.path != "" {
		return filepath.Base(a.path)
	}
	name := strings.NewReplacer("/", "-", `\`, "-").Replace(a.currentName)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "简历"
	}
	if !strings.EqualFold(filepath.Ext(name), ".md") {
		name += ".md"
	}
	return name
}

func (a *App) exactSource() []byte {
	if a.rawSource != nil && a.buffer.Version() == a.rawVersion {
		return append([]byte(nil), a.rawSource...)
	}
	return []byte(a.buffer.Snapshot().String())
}

func (a *App) dialogs(c *ui.Context) {
	if a.renameOpen {
		theme := c.Theme()
		width, height := c.Size()
		ui.DialogBase(c.Key("rename-modal"), &a.renameOpen, func(backdrop, panel ui.Element) {
			backdrop.Background(ui.RGBA(0, 0, 0, 0.4)).
				Transition(dialogBackdropTransition())
			panel.Padding(theme.Space(5)).Gap(theme.Space(3)).
				Radius(theme.Space(2.5)).Background(theme.Background).
				MaxWidth(width-theme.Space(10)).
				MaxHeight(height-theme.Space(10)).
				Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3)).
				Children(func() {
					ui.Column(c.Key("rename-panel")).Width(380).Gap(12).Children(func() {
						ui.Text(c, "重命名简历").Font(uiFontFamily).FontSize(15).
							FontWeight(600).TextColor(carbonInk)
						input := ui.TextInput(c.Key("rename-name"), &a.renameValue).
							Label("文档名称").FillWidth().AutoFocus()
						input.OnSubmit(a.confirmRename)
						ui.Row(c.Key("rename-actions")).FillWidth().Gap(8).Justify(ui.End).
							Children(func() {
								ui.Button(c.Key("rename-cancel"), "取消").OnClick(func() {
									a.renameOpen = false
								})
								ui.PrimaryButton(c.Key("rename-confirm"), "确定").
									Disabled(strings.TrimSpace(a.renameValue) == "").
									OnClick(a.confirmRename)
							})
					})
				})
		})
	}
	if a.deleteOpen {
		choice := ui.AlertDialog(c.Key("delete-dialog"), &a.deleteOpen,
			"删除简历？", a.currentName, "取消", "删除")
		if choice == 1 {
			a.deleteDocument(a.deleteID)
		}
	}
}

func (a *App) beginRename() {
	if a.currentID == "" || a.path != "" {
		return
	}
	a.renameID = a.currentID
	a.renameValue = a.currentName
	a.renameOpen = true
}

func (a *App) confirmRename() {
	name := strings.TrimSpace(a.renameValue)
	if a.store == nil || a.renameID == "" || name == "" {
		return
	}
	store := a.store
	target := a.renameID
	a.renameOpen = false
	a.renameID = ""
	a.libraryRequest++
	request := a.libraryRequest
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		renamed, err := store.Rename(target, name)
		if err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("重命名失败")
			})
			return
		}
		documents, err := store.List()
		if err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("重命名失败")
			})
			return
		}
		a.post(func() {
			if a.closed || request != a.libraryRequest {
				return
			}
			a.documents = documents
			if a.currentID == renamed.ID {
				a.currentName = renamed.Name
				a.notifyTitle()
			}
			a.setFileError("")
		})
	}()
}

func (a *App) beginDelete() {
	if a.currentID == "" || a.path != "" {
		return
	}
	a.deleteID = a.currentID
	a.deleteOpen = true
}

func (a *App) deleteDocument(id string) {
	if a.store == nil || id == "" {
		return
	}
	store := a.store
	wasCurrent := a.currentID == id
	a.deleteID = ""
	a.libraryRequest++
	request := a.libraryRequest
	a.fileIO.Add(1)
	go func() {
		defer a.fileIO.Done()
		if _, err := store.Delete(id); err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("删除失败")
			})
			return
		}
		documents, err := store.List()
		if err != nil {
			a.post(func() {
				if a.closed || request != a.libraryRequest {
					return
				}
				a.setFileError("删除失败")
			})
			return
		}
		var next library.Document
		var source []byte
		if wasCurrent && len(documents) > 0 {
			next, source, err = store.Show(documents[0].ID)
			if err != nil {
				a.post(func() {
					if a.closed || request != a.libraryRequest {
						return
					}
					a.setFileError("打开失败")
				})
				return
			}
		}
		a.post(func() {
			if a.closed || request != a.libraryRequest {
				return
			}
			a.documents = documents
			if wasCurrent && a.currentID == id && a.path == "" {
				if len(documents) == 0 {
					a.clearDocument()
				} else {
					a.loadLibraryDocument(next, string(source))
				}
			}
			a.setFileError("")
		})
	}()
}

func (a *App) clearDocument() {
	a.buffer.Reset("")
	a.rawSource = nil
	a.rawVersion = 0
	a.path = ""
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

func (a *App) toggleRail() {
	a.railOpen = !a.railOpen
	a.railReady = true
	if a.sidebarToggle != nil {
		a.sidebarToggle()
	}
	if a.onInvalidate != nil {
		a.onInvalidate()
	}
}
