package desktop

import "github.com/egoist/mygo/ui"

const (
	defaultEditorShare float32 = 40
	minEditorWidth     float32 = 420
	minPreviewWidth    float32 = 526
	splitLineWidth     float32 = 1
	splitHandleWidth   float32 = 7
	splitStep          float32 = 16
)

const documentRailFlowMinWidth = documentRailWidth + splitLineWidth + minEditorWidth + minPreviewWidth

func (a *App) workbench(c *ui.Context, room float32) {
	editorWidth := a.editorWidth(room)
	ui.Row(c.Key("workbench")).Grow(1).MinWidth(0).FillHeight().
		AlignItems(ui.Stretch).Children(func() {
		ui.Box(c.Key("editor-pane-slot")).Width(editorWidth).Shrink(0).FillHeight().
			Children(func() {
				a.editorPane(c)
			})
		ui.Box(c.Key("split-line-space")).Width(splitLineWidth).Shrink(0).FillHeight()
		ui.Box(c.Key("preview-pane-slot")).Grow(1).MinWidth(0).FillHeight().
			Children(func() {
				a.previewPane(c)
			})
		a.splitHandle(c, editorWidth, room)
	})
}

func (a *App) editorWidth(room float32) float32 {
	if room <= 0 {
		return minEditorWidth
	}
	width := room * a.splitShare / 100
	maxWidth := max(minEditorWidth, room-splitLineWidth-minPreviewWidth)
	return min(max(width, minEditorWidth), maxWidth)
}

func (a *App) splitHandle(c *ui.Context, editorWidth, room float32) {
	handle := ui.Box(c.Key("editor-preview-split")).Absolute().
		Left(editorWidth - (splitHandleWidth-splitLineWidth)/2).
		Top(0).Bottom(0).Width(splitHandleWidth).
		Cursor(ui.CursorResizeEW).Focusable().Role(ui.RoleSplitter).
		Label("调整编辑区与预览区宽度").Tooltip("拖动调整宽度")
	dx, _, held := handle.Dragged()
	if held && dx != 0 {
		a.moveSplit(dx, room)
	}
	if handle.Shortcut(0, ui.KeyLeft) {
		a.moveSplit(-splitStep, room)
		a.invalidate()
	}
	if handle.Shortcut(0, ui.KeyRight) {
		a.moveSplit(splitStep, room)
		a.invalidate()
	}
	if handle.DoubleClicked() {
		a.splitShare = defaultEditorShare
		a.invalidate()
	}
	line := hairlineStone
	if handle.Hovered() || held || handle.FocusVisible() {
		line = editorialCobalt
	}
	handle.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{
			X: r.X + (r.W-splitLineWidth)/2,
			Y: r.Y,
			W: splitLineWidth,
			H: r.H,
		}, line, 0)
	})
}

func (a *App) moveSplit(dx, room float32) {
	if room <= 0 {
		return
	}
	width := min(max(a.editorWidth(room)+dx, minEditorWidth),
		max(minEditorWidth, room-splitLineWidth-minPreviewWidth))
	a.splitShare = min(max(width/room*100, 0), 100)
}
