package desktop

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	uiFontFamily     = `"PingFang SC", "Noto Sans CJK SC", "Microsoft YaHei", sans-serif`
	monoFontFamily   = `"JetBrains Mono", "SFMono-Regular", Menlo, monospace`
	editorFontFamily = `"JetBrains Mono", "PingFang SC", "Noto Sans CJK SC", monospace`
)

var (
	ashShell        = ui.Hex("#EDECE8")
	toolbarLinen    = ui.Hex("#E6E4DF")
	sidebarTray     = ui.Hex("#E9E7E2")
	proofingStage   = ui.Hex("#E0DFDB")
	manuscriptPaper = ui.Hex("#FBFBF9")
	carbonInk       = ui.Hex("#18191B")
	mutedGraphite   = ui.Hex("#3A3C42")
	softGraphite    = ui.Hex("#A8ABB2")
	hairlineStone   = ui.Hex("#D3D0C8")
	activeCardEdge  = ui.Hex("#CFCBC2")
	editorialCobalt = ui.Hex("#1E4D8C")
	proofVermilion  = ui.Hex("#C43B2B")
	dialectOchre    = ui.Hex("#9E4B16")
)

func shellTheme() *ui.Theme {
	theme := ui.LightTheme()
	theme.Background = ashShell
	theme.Surface = manuscriptPaper
	theme.SurfaceHover = sidebarTray
	theme.SurfacePressed = hairlineStone
	theme.Border = hairlineStone
	theme.Text = carbonInk
	theme.TextMuted = mutedGraphite
	theme.Accent = editorialCobalt
	theme.AccentHover = editorialCobalt
	theme.AccentPressed = editorialCobalt
	theme.AccentText = ui.Hex("#FFFFFF")
	theme.Focus = editorialCobalt
	theme.Danger = proofVermilion
	theme.Selection = ui.RGBA(30, 77, 140, 0.18)
	theme.Radius = 4
	theme.Font = uiFontFamily
	theme.FontSize = 13
	return theme
}

func resumeEditorTheme() *editor.Theme {
	theme := editor.DefaultTheme(ui.LightTheme())
	theme.Background = manuscriptPaper
	theme.Text = carbonInk
	theme.LineNumber = softGraphite
	theme.ActiveLineNumber = mutedGraphite
	theme.ActiveLine = ui.Transparent
	theme.Selection = ui.RGBA(30, 77, 140, 0.18)
	theme.InactiveSelection = ui.RGBA(30, 77, 140, 0.10)
	theme.Cursor = editorialCobalt
	theme.MatchingBracket = editorialCobalt
	theme.Error = proofVermilion
	theme.Warning = dialectOchre
	for kind := range theme.Tokens {
		theme.Tokens[kind] = editor.TokenStyle{Color: carbonInk}
	}
	theme.Tokens[editor.TokenHeading] = editor.TokenStyle{Color: carbonInk, Bold: true}
	theme.Tokens[editor.TokenStrong] = editor.TokenStyle{Color: carbonInk, Bold: true}
	theme.Tokens[editor.TokenLink] = editor.TokenStyle{Color: carbonInk}
	theme.Tokens[editor.TokenCode] = editor.TokenStyle{Color: carbonInk}
	theme.Tokens[editor.TokenKeyword] = editor.TokenStyle{Color: editorialCobalt, Bold: true}
	theme.Tokens[editor.TokenAttribute] = editor.TokenStyle{Color: dialectOchre}
	return theme
}
