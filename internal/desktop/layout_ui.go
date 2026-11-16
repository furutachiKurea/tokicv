package desktop

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"tokicv/internal/library"
	"tokicv/internal/render"
)

var fontStacks = []render.FontStack{
	render.FontClassic,
	render.FontNoto,
	render.FontSerif,
	render.FontSystem,
}

var fontStackLabels = []string{"经典", "思源", "衬线", "系统"}

type layoutFieldState struct {
	text   string
	synced float64
}

func (a *App) layoutPopover(c *ui.Context) {
	button := a.toolbarButton(c, "layout-profile", "排版", "排版", layoutIcon, !a.hasDocument(), func() {
		a.layoutMenuOpen = !a.layoutMenuOpen
	})
	popover := ui.Popover(c.Key("layout-profile-popover"), button, &a.layoutMenuOpen, func() {
		ui.Column(c.Key("layout-profile-panel")).Width(340).Gap(10).Padding(14).
			Children(func() {
				a.fontStackField(c)
				ui.Row(c.Key("layout-spacing-fields")).FillWidth().Gap(10).Children(func() {
					a.layoutNumberField(c, "layout-section-gap", "分节间距 (px)",
						a.layout.SectionGapPX, render.MinSectionGapPX, render.MaxSectionGapPX, 2,
						func(value float64) {
							next := a.layout
							next.SectionGapPX = value
							a.changeBaseLayout(next)
						}).Grow(1)
					a.layoutNumberField(c, "layout-item-gap", "条目间距 (px)",
						a.layout.ItemGapPX, render.MinItemGapPX, render.MaxItemGapPX, 1,
						func(value float64) {
							next := a.layout
							next.ItemGapPX = value
							a.changeBaseLayout(next)
						}).Grow(1)
				})
				ui.Row(c.Key("layout-margin-fields")).FillWidth().Gap(10).Children(func() {
					a.layoutNumberField(c, "layout-margin-top", "上边距 (mm)",
						a.layout.MarginTopMM, render.MinMarginMM, render.MaxMarginMM, 1,
						func(value float64) {
							next := a.layout
							next.MarginTopMM = value
							a.changeBaseLayout(next)
						}).Grow(1)
					a.layoutNumberField(c, "layout-margin-bottom", "下边距 (mm)",
						a.layout.MarginBottomMM, render.MinMarginMM, render.MaxMarginMM, 1,
						func(value float64) {
							next := a.layout
							next.MarginBottomMM = value
							a.changeBaseLayout(next)
						}).Grow(1)
				})
				a.layoutNumberField(c, "layout-line-height", "行高",
					a.layout.LineHeight, render.MinLineHeight, render.MaxLineHeight, 0.05,
					func(value float64) {
						next := a.layout
						next.LineHeight = value
						a.changeBaseLayout(next)
					})
				ui.Box(c.Key("layout-actions-separator")).FillWidth().Height(1).
					Background(hairlineStone)
				ui.Row(c.Key("layout-save-actions")).FillWidth().Gap(8).Children(func() {
					ui.Button(c.Key("save-global-layout"), "设为全局默认").Grow(1).
						OnClick(a.saveGlobalLayout)
					ui.Button(c.Key("save-target-layout"), "设为当前简历").Grow(1).
						Disabled(!a.hasDocument()).OnClick(a.saveTargetLayout)
				})
				ui.Row(c.Key("layout-reset-actions")).FillWidth().Gap(8).Children(func() {
					ui.Button(c.Key("clear-target-layout"), "清除当前覆盖").Grow(1).
						Disabled(a.layoutOrigin != library.LayoutOriginDocument &&
							a.layoutOrigin != library.LayoutOriginPath).
						OnClick(a.clearTargetLayout)
					ui.Button(c.Key("reset-layout"), "恢复默认").Grow(1).OnClick(func() {
						a.loadEffectiveLayout()
						a.previewNow()
					})
				})
			})
	})
	if popover.Valid() {
		popover.Transition(popoverTransition())
	}
}

func (a *App) fontStackField(c *ui.Context) {
	ui.Field(c, "字体", func() {
		choice := fontStackIndex(a.layout.FontStack)
		segmented := ui.Segmented(c.Key("layout-font-stack"), &choice, fontStackLabels...).
			FillEqually().Changed()
		if segmented {
			next := a.layout
			next.FontStack = fontStacks[choice]
			a.changeBaseLayout(next)
		}
	}).FillWidth()
}

func (a *App) layoutNumberField(
	c *ui.Context,
	key, label string,
	value, lo, hi, step float64,
	set func(float64),
) ui.Element {
	state := a.layoutFields[key]
	if state == nil {
		state = &layoutFieldState{
			text:   formatLayoutField(value),
			synced: value,
		}
		a.layoutFields[key] = state
	}
	if state.synced != value {
		state.text = formatLayoutField(value)
		state.synced = value
	}
	decreaseLabel := "减少" + label
	increaseLabel := "增加" + label
	if key == "layout-line-height" {
		decreaseLabel = "降低行高"
		increaseLabel = "提高行高"
	}
	theme := c.Theme()
	controlSize := theme.Space(7)

	apply := func(next float64) {
		next = roundLayoutStep(next, step)
		next = math.Max(lo, math.Min(hi, next))
		state.synced = next
		state.text = formatLayoutField(next)
		set(next)
	}

	return ui.Field(c, label, func() {
		ui.Row(c.Key(key + "-control")).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
			input := ui.TextInput(c.Key(key), &state.text).
				Label(label).Height(controlSize).FillWidth().MinWidth(0).
				Padding(theme.Space(1), theme.Space(2.5)).
				Font(monoFontFamily).FontSize(12).FontFeatures("tnum").
				TextAlign(ui.Center)
			if input.Changed() {
				if next, err := strconv.ParseFloat(strings.TrimSpace(state.text), 64); err == nil &&
					!math.IsNaN(next) && !math.IsInf(next, 0) && next >= lo && next <= hi {
					state.synced = next
					set(next)
				}
			}
			if !input.Focused() && state.synced == value {
				state.text = formatLayoutField(value)
			}

			decrease := ui.Button(c.Key(key+"-decrease"), "").
				Label(decreaseLabel).Size(controlSize, controlSize).Padding(0).
				Tooltip(decreaseLabel).
				Disabled(value <= lo).
				OnClick(func() { apply(value - step) })
			decrease.Children(func() {
				ui.Icon(c, minusIcon).FontSize(14).TextColor(carbonInk)
			})
			increase := ui.Button(c.Key(key+"-increase"), "").
				Label(increaseLabel).Size(controlSize, controlSize).Padding(0).
				Tooltip(increaseLabel).
				Disabled(value >= hi).
				OnClick(func() { apply(value + step) })
			increase.Children(func() {
				ui.Icon(c, plusIcon).FontSize(14).TextColor(carbonInk)
			})
		})
	})
}

func formatLayoutField(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func roundLayoutStep(value, step float64) float64 {
	if step <= 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		return value
	}
	scale := 1.0
	for scale < 1e9 && math.Abs(step*scale-math.Round(step*scale)) > 1e-9 {
		scale *= 10
	}
	return math.Round(value*scale) / scale
}

func fontStackIndex(font render.FontStack) int {
	for index, candidate := range fontStacks {
		if candidate == font {
			return index
		}
	}
	return 0
}

func (a *App) toolbarButton(
	c *ui.Context,
	key, label, tooltip string,
	icon *ui.SVG,
	disabled bool,
	onClick func(),
) ui.Element {
	button := ui.ButtonBase(c.Key(key)).Height(30).Padding(0, 10).Gap(6).
		Radius(4).AlignItems(ui.Center).Label(label).Tooltip(tooltip).
		Disabled(disabled).FocusRing(true).FocusRingColor(editorialCobalt).
		FocusRingOffset(1)
	if disabled {
		button.Background(ui.Transparent)
	}
	if button.Hovered() && !disabled {
		button.Background(sidebarTray)
	}
	if button.Pressed() && !disabled {
		button.Background(hairlineStone)
	}
	button.OnClick(onClick)
	button.Children(func() {
		color := carbonInk
		if disabled {
			color = softGraphite
		}
		ui.Icon(c, icon).FontSize(13).TextColor(color)
		ui.Text(c, label).Font(uiFontFamily).FontSize(12).
			FontWeight(500).TextColor(color)
	})
	return button
}

func (a *App) smartOnePageToggle(c *ui.Context) {
	on := a.smartOnePage
	label := "智能一页"
	tooltip := "智能一页"
	if a.smartSolving {
		label = "正在压缩为一页"
		tooltip = "正在压缩为一页"
	}
	toggle := ui.ToggleBase(c.Key("smart-one-page"), &on).
		Height(30).Padding(0, 10).Gap(6).Radius(4).AlignItems(ui.Center).
		Label(label).Tooltip(tooltip).Disabled(!a.hasDocument() || a.artifacts == nil || a.smartSolving).
		FocusRing(true).FocusRingColor(editorialCobalt).FocusRingOffset(1)
	background := manuscriptPaper
	border := hairlineStone
	foreground := carbonInk
	if on || a.smartSolving {
		background, border, foreground = editorialCobalt, editorialCobalt, ui.Hex("#FFFFFF")
	} else if !a.hasDocument() || a.artifacts == nil {
		foreground = softGraphite
	} else if toggle.Hovered() {
		background = sidebarTray
	}
	colorAnim := stateColorAnimation()
	toggle.Background(toggle.AnimateColor("smart-background", background, colorAnim)).
		Border(1, toggle.AnimateColor("smart-border", border, colorAnim))
	toggle.Children(func() {
		textColor := toggle.AnimateColor("smart-foreground", foreground, colorAnim)
		ui.Icon(c, onePageIcon).FontSize(13).TextColor(textColor)
		ui.Text(c, label).Font(uiFontFamily).FontSize(12).
			FontWeight(500).TextColor(textColor)
	})
	toggle.OnChange(func() {
		a.setSmartOnePage(on)
	})
}

func (a *App) exportMenu(c *ui.Context) {
	disabled := !a.canExport()
	button := ui.ButtonBase(c.Key("export-action")).Height(30).Padding(0, 12).
		Gap(6).Radius(4).AlignItems(ui.Center).Label("导出").
		Disabled(disabled).FocusRing(true).FocusRingColor(editorialCobalt).
		FocusRingOffset(1)
	button.Background(editorialCobalt)
	if button.Hovered() && !disabled {
		button.Background(ui.Hex("#183F72"))
	}
	if button.Pressed() && !disabled {
		button.Background(ui.Hex("#12345F"))
	}
	if disabled {
		button.Background(ui.Hex("#B8C4D4"))
	}
	button.PopoverMenuButton(func(menu *ui.Menu) {
		if menu.Item("导出 PDF").Disabled(disabled || a.exportPaths[ArtifactPDF] == nil).Chosen() {
			a.exportArtifact(ArtifactPDF)
		}
		if menu.Item("导出 PNG").Disabled(disabled || a.exportPaths[ArtifactPNG] == nil).Chosen() {
			a.exportArtifact(ArtifactPNG)
		}
		if menu.Item("导出 Markdown").Disabled(disabled || a.exportPaths[ArtifactMarkdown] == nil).Chosen() {
			a.exportArtifact(ArtifactMarkdown)
		}
	}, ui.PopoverMenuButtonOptions{
		At:   ui.AnchorBottomEnd,
		Self: ui.AnchorTopEnd,
		Open: &a.exportMenuOpen,
		Panel: func(panel ui.Element) {
			panel.Background(manuscriptPaper).Border(1, hairlineStone).Radius(4)
			panel.Transition(popoverTransition())
		},
	})
	button.Children(func() {
		ui.Icon(c, exportIcon).FontSize(13).TextColor(ui.Hex("#FFFFFF"))
		ui.Text(c, "导出").Font(uiFontFamily).FontSize(12).
			FontWeight(600).TextColor(ui.Hex("#FFFFFF"))
	})
}

func (a *App) layoutNoticeText(c *ui.Context) {
	if a.layoutNotice == "" {
		return
	}
	ui.Text(c.Key("layout-notice"), a.layoutNotice).FontSize(12).
		TextColor(proofVermilion).Role(ui.RoleStatus)
}

func (a *App) artifactErrorText(c *ui.Context) {
	if a.artifactError == "" {
		return
	}
	ui.Text(c.Key("artifact-error"), a.artifactError).FontSize(12).
		TextColor(proofVermilion).Role(ui.RoleAlert)
}
