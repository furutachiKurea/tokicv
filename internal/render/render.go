package render

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"

	"tokicv/internal/dialect"
)

const (
	pageMarginXMM   = 13.229
	MinLineHeight   = 1.20
	MaxLineHeight   = 2.20
	MinMarginMM     = 7.5
	MaxMarginMM     = 30.0
	MinSectionGapPX = 5.0
	MinItemGapPX    = 1.5
	MaxSectionGapPX = 40.0
	MaxItemGapPX    = 20.0
)

// FontStack selects the Resume Sheet font family without changing locked font
// sizes.
type FontStack string

// Supported Sheet font stacks.
const (
	FontClassic FontStack = "classic"
	FontNoto    FontStack = "noto"
	FontSerif   FontStack = "serif"
	FontSystem  FontStack = "system"
)

// ParseFontStack validates a CLI or Layout Profile font-stack name.
func ParseFontStack(value string) (FontStack, bool) {
	switch FontStack(value) {
	case FontClassic, FontNoto, FontSerif, FontSystem:
		return FontStack(value), true
	default:
		return "", false
	}
}

func (font FontStack) cssFamily() string {
	switch font {
	case FontNoto:
		return `"Nunito", "Noto Sans CJK SC", sans-serif`
	case FontSerif:
		return `"Nunito", "Noto Serif CJK SC", serif`
	case FontSystem:
		return `"Nunito", system-ui, sans-serif`
	default:
		return `"Nunito", "PingFang SC", "Noto Sans CJK SC", "Microsoft YaHei", sans-serif`
	}
}

// Layout is the non-destructive Resume Sheet spatial and font-stack profile.
// MarginVMM sets both vertical margins unless MarginTopMM or MarginBottomMM is
// set explicitly. Zero values select defaults.
type Layout struct {
	FontStack      FontStack `json:"fontStack"`
	LineHeight     float64   `json:"lineHeight"`
	MarginTopMM    float64   `json:"marginTopMM"`
	MarginBottomMM float64   `json:"marginBottomMM"`
	MarginVMM      float64   `json:"marginVMM"`
	SectionGapPX   float64   `json:"sectionGapPX"`
	ItemGapPX      float64   `json:"itemGapPX"`
}

// LayoutPatch carries explicit, partial Layout Profile overrides. A nil field
// inherits the value from the profile beneath it.
type LayoutPatch struct {
	FontStack      *FontStack `json:"fontStack,omitempty"`
	LineHeight     *float64   `json:"lineHeight,omitempty"`
	MarginTopMM    *float64   `json:"marginTopMM,omitempty"`
	MarginBottomMM *float64   `json:"marginBottomMM,omitempty"`
	SectionGapPX   *float64   `json:"sectionGapPX,omitempty"`
	ItemGapPX      *float64   `json:"itemGapPX,omitempty"`
}

// Any reports whether the patch contains an explicit override.
func (patch LayoutPatch) Any() bool {
	return patch.FontStack != nil ||
		patch.LineHeight != nil ||
		patch.MarginTopMM != nil ||
		patch.MarginBottomMM != nil ||
		patch.SectionGapPX != nil ||
		patch.ItemGapPX != nil
}

// Apply returns base with every explicit patch field replaced.
func (patch LayoutPatch) Apply(base Layout) Layout {
	if patch.FontStack != nil {
		base.FontStack = *patch.FontStack
	}
	if patch.LineHeight != nil {
		base.LineHeight = *patch.LineHeight
	}
	if patch.MarginTopMM != nil {
		base.MarginTopMM = *patch.MarginTopMM
	}
	if patch.MarginBottomMM != nil {
		base.MarginBottomMM = *patch.MarginBottomMM
	}
	if patch.SectionGapPX != nil {
		base.SectionGapPX = *patch.SectionGapPX
	}
	if patch.ItemGapPX != nil {
		base.ItemGapPX = *patch.ItemGapPX
	}
	return base
}

// Validate rejects explicit Layout Profile values outside readable bounds.
// Zero values remain valid so partial profiles can inherit defaults.
func (patch LayoutPatch) Validate() error {
	if patch.FontStack != nil {
		if _, ok := ParseFontStack(string(*patch.FontStack)); !ok {
			return fmt.Errorf("无效字体栈 %q", *patch.FontStack)
		}
	}
	if patch.LineHeight != nil {
		if err := validateRangeValue("行高", *patch.LineHeight, MinLineHeight, MaxLineHeight, false); err != nil {
			return err
		}
	}
	if patch.MarginTopMM != nil {
		if err := validateRangeValue("上边距", *patch.MarginTopMM, MinMarginMM, MaxMarginMM, false); err != nil {
			return err
		}
	}
	if patch.MarginBottomMM != nil {
		if err := validateRangeValue("下边距", *patch.MarginBottomMM, MinMarginMM, MaxMarginMM, false); err != nil {
			return err
		}
	}
	if patch.SectionGapPX != nil {
		if err := validateRangeValue("节间距", *patch.SectionGapPX, MinSectionGapPX, MaxSectionGapPX, false); err != nil {
			return err
		}
	}
	if patch.ItemGapPX != nil {
		if err := validateRangeValue("条目间距", *patch.ItemGapPX, MinItemGapPX, MaxItemGapPX, false); err != nil {
			return err
		}
	}
	return nil
}

// DefaultLayout returns the comfortable default Sheet layout.
func DefaultLayout() Layout {
	return Layout{
		FontStack:    FontClassic,
		LineHeight:   1.40,
		MarginVMM:    10,
		SectionGapPX: 12,
		ItemGapPX:    5,
	}
}

// WithPatch returns the Layout with patch fields applied.
func (layout Layout) WithPatch(patch LayoutPatch) Layout {
	return patch.Apply(layout)
}

// Normalized returns the Layout Profile with defaults filled in.
func (layout Layout) Normalized() Layout {
	return layout.normalized()
}

// Validate rejects Layout Profile fields outside their supported bounds. Zero
// values are allowed for partial profiles.
func (layout Layout) Validate() error {
	if layout.FontStack != "" {
		if _, ok := ParseFontStack(string(layout.FontStack)); !ok {
			return fmt.Errorf("无效字体栈 %q", layout.FontStack)
		}
	}
	for _, field := range []struct {
		name      string
		value     float64
		min, max  float64
		allowZero bool
	}{
		{"行高", layout.LineHeight, MinLineHeight, MaxLineHeight, true},
		{"上边距", layout.MarginTopMM, MinMarginMM, MaxMarginMM, true},
		{"下边距", layout.MarginBottomMM, MinMarginMM, MaxMarginMM, true},
		{"垂直边距", layout.MarginVMM, MinMarginMM, MaxMarginMM, true},
		{"节间距", layout.SectionGapPX, MinSectionGapPX, MaxSectionGapPX, true},
		{"条目间距", layout.ItemGapPX, MinItemGapPX, MaxItemGapPX, true},
	} {
		if err := validateRangeValue(field.name, field.value, field.min, field.max, field.allowZero); err != nil {
			return err
		}
	}
	return nil
}

func (layout Layout) normalized() Layout {
	defaults := DefaultLayout()
	switch layout.FontStack {
	case FontClassic, FontNoto, FontSerif, FontSystem:
	default:
		layout.FontStack = defaults.FontStack
	}
	if layout.LineHeight == 0 || !finite(layout.LineHeight) {
		layout.LineHeight = defaults.LineHeight
	}
	layout.LineHeight = clamp(layout.LineHeight, MinLineHeight, MaxLineHeight)
	if layout.MarginVMM <= 0 || !finite(layout.MarginVMM) {
		layout.MarginVMM = defaults.MarginVMM
	}
	layout.MarginVMM = clamp(layout.MarginVMM, MinMarginMM, MaxMarginMM)
	if layout.MarginTopMM <= 0 || !finite(layout.MarginTopMM) {
		layout.MarginTopMM = layout.MarginVMM
	}
	layout.MarginTopMM = clamp(layout.MarginTopMM, MinMarginMM, MaxMarginMM)
	if layout.MarginBottomMM <= 0 || !finite(layout.MarginBottomMM) {
		layout.MarginBottomMM = layout.MarginVMM
	}
	layout.MarginBottomMM = clamp(layout.MarginBottomMM, MinMarginMM, MaxMarginMM)
	if layout.MarginTopMM == layout.MarginBottomMM {
		layout.MarginVMM = layout.MarginTopMM
	}
	if layout.SectionGapPX <= 0 || !finite(layout.SectionGapPX) {
		layout.SectionGapPX = defaults.SectionGapPX
	}
	layout.SectionGapPX = clamp(layout.SectionGapPX, MinSectionGapPX, MaxSectionGapPX)
	if layout.ItemGapPX <= 0 || !finite(layout.ItemGapPX) {
		layout.ItemGapPX = defaults.ItemGapPX
	}
	layout.ItemGapPX = clamp(layout.ItemGapPX, MinItemGapPX, MaxItemGapPX)
	return layout
}

// Render renders a Resume Document as a standalone HTML document with the
// default Layout Profile.
func Render(doc dialect.Document) string {
	return RenderWithLayout(doc, DefaultLayout())
}

// RenderWithLayout renders a Resume Document with an explicit Layout Profile.
func RenderWithLayout(doc dialect.Document, layout Layout) string {
	layout = layout.normalized()
	var out strings.Builder
	out.Grow(len(stylesheet(layout)) + len(doc.Blocks)*192 + 512)

	out.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light">
<title>tokicv</title>
<style id="tokicv-layout">
`)
	out.WriteString(stylesheet(layout))
	out.WriteString(`</style>
</head>
<body>
<main id="sheet-root">
`)
	for _, block := range doc.Blocks {
		renderBlock(&out, block)
	}
	out.WriteString(`</main>
</body>
</html>
`)
	return out.String()
}

func renderBlock(out *strings.Builder, block dialect.Block) {
	switch block := block.(type) {
	case dialect.Heading:
		level := block.Level
		if level < 1 || level > 3 {
			level = 3
		}
		fmt.Fprintf(out, "<h%d>", level)
		renderInlines(out, block.Children)
		fmt.Fprintf(out, "</h%d>\n", level)
	case dialect.Paragraph:
		out.WriteString("<p>")
		renderInlines(out, block.Children)
		out.WriteString("</p>\n")
	case dialect.List:
		renderList(out, block.Items, 1)
	case dialect.SplitBlock:
		renderSplitBlock(out, block)
	}
}

func renderList(out *strings.Builder, items []dialect.ListItem, level int) {
	fmt.Fprintf(out, `<ul class="resume-list level-%d">`+"\n", level)
	for _, item := range items {
		out.WriteString("<li>")
		renderInlines(out, item.Children)
		if len(item.Items) > 0 {
			out.WriteByte('\n')
			renderList(out, item.Items, level+1)
		}
		out.WriteString("</li>\n")
	}
	out.WriteString("</ul>\n")
}

func renderSplitBlock(out *strings.Builder, block dialect.SplitBlock) {
	paired := len(block.Left) > 0 && len(block.Right) > 0
	className := "split-block"
	if !paired {
		className += " split-single"
	}
	fmt.Fprintf(out, `<div class="%s">`+"\n", className)
	if len(block.Left) > 0 {
		out.WriteString(`<div class="split-column split-left">` + "\n")
		for _, child := range block.Left {
			renderBlock(out, child)
		}
		out.WriteString("</div>\n")
	}
	if len(block.Right) > 0 {
		out.WriteString(`<div class="split-column split-right">` + "\n")
		for _, child := range block.Right {
			if paragraph, ok := child.(dialect.Paragraph); ok && isDateRange(paragraph.Children) {
				out.WriteString(`<p class="date">`)
				renderInlines(out, paragraph.Children)
				out.WriteString("</p>\n")
				continue
			}
			renderBlock(out, child)
		}
		out.WriteString("</div>\n")
	}
	out.WriteString("</div>\n")
}

var dateRangePattern = regexp.MustCompile(`^\d{4}\.\d{2}\s*[-–—]\s*\d{4}\.\d{2}$`)

func isDateRange(inlines []dialect.Inline) bool {
	var text strings.Builder
	var appendText func([]dialect.Inline)
	appendText = func(items []dialect.Inline) {
		for _, inline := range items {
			switch inline := inline.(type) {
			case dialect.Text:
				text.WriteString(inline.Value)
			case dialect.Strong:
				appendText(inline.Children)
			case dialect.Link:
				appendText(inline.Children)
			}
		}
	}
	appendText(inlines)
	return dateRangePattern.MatchString(strings.TrimSpace(text.String()))
}

func renderInlines(out *strings.Builder, inlines []dialect.Inline) {
	for _, inline := range inlines {
		switch inline := inline.(type) {
		case dialect.Text:
			out.WriteString(html.EscapeString(inline.Value))
		case dialect.Strong:
			out.WriteString("<strong>")
			renderInlines(out, inline.Children)
			out.WriteString("</strong>")
		case dialect.Link:
			out.WriteString(`<a href="`)
			out.WriteString(html.EscapeString(inline.URL))
			out.WriteString(`">`)
			renderInlines(out, inline.Children)
			out.WriteString("</a>")
		case dialect.Pill:
			out.WriteString(`<code class="pill">`)
			out.WriteString(html.EscapeString(inline.Value))
			out.WriteString("</code>")
		case dialect.Icon:
			if inline.Known {
				renderIcon(out, inline.Name)
			} else {
				out.WriteString(html.EscapeString(inline.Raw))
			}
		}
	}
}

func stylesheet(layout Layout) string {
	lineHeight := formatNumber(layout.LineHeight)
	sectionGap := formatNumber(layout.SectionGapPX)
	itemGap := formatNumber(layout.ItemGapPX)

	var css strings.Builder
	css.Grow(12000)
	css.WriteString(fontFaceCSS())
	fmt.Fprintf(&css, `
@page { size: 210mm 297mm; margin: 0; }
:root {
  color-scheme: light;
  --font-family: %s;
  --line-height: %s;
  --margin-top: %smm;
  --margin-bottom: %smm;
  --section-gap: %spx;
  --item-gap: %spx;
}
* { box-sizing: border-box; }
html {
  margin: 0;
  padding: 0;
  background: #ffffff;
}
body {
  margin: 0;
  padding: 0;
  color: #010101;
  background: #ffffff;
  font-family: var(--font-family);
  font-size: 10.5pt;
  font-weight: 400;
  line-height: var(--line-height);
  text-rendering: geometricPrecision;
  -webkit-font-smoothing: antialiased;
  -webkit-print-color-adjust: exact;
  print-color-adjust: exact;
  overflow-wrap: break-word;
}
#sheet-root {
  width: 210mm;
  min-height: 297mm;
  padding: var(--margin-top) %smm var(--margin-bottom);
  background: #ffffff;
  color: #010101;
  overflow-wrap: break-word;
}
h1 {
  margin: 0 0 7px;
  color: #000000;
  font-size: 22.5pt;
  font-weight: 500;
  line-height: 1.20;
  break-after: avoid;
  page-break-after: avoid;
}
h2 {
  width: 100%%;
  margin: var(--section-gap) 0 5px;
  padding: 0 0 3px;
  color: #000000;
  border-bottom: 1px solid #000000;
  font-size: 15pt;
  font-weight: 500;
  line-height: 1.30;
  break-after: avoid;
  page-break-after: avoid;
}
h3 {
  margin: calc(var(--section-gap) * 0.67) 0 3px;
  color: #111111;
  font-size: 11.5pt;
  font-weight: 500;
  line-height: 1.30;
  break-after: avoid;
  page-break-after: avoid;
}
p {
  margin: 0 0 var(--item-gap);
  color: #010101;
  font-size: 10.5pt;
  line-height: var(--line-height);
  orphans: 2;
  widows: 2;
}
.resume-list {
  margin: 0 0 var(--item-gap);
  padding: 0 0 0 1.3em;
  list-style: none;
}
.resume-list .resume-list {
  margin: 0;
  padding-left: 2.06em;
}
.resume-list > li {
  position: relative;
  margin: 0 0 calc(var(--item-gap) * 0.4);
  color: #010101;
  font-size: 10.5pt;
  line-height: var(--line-height);
  break-inside: avoid;
}
.resume-list > li::before {
  content: "";
  position: absolute;
  display: block;
  box-sizing: border-box;
  left: -15.7px;
  top: 0.33em;
  width: 5px;
  height: 5px;
  background: #010101;
}
.resume-list .resume-list > li::before {
  left: -15.7px;
  top: 0.33em;
  width: 5px;
  height: 5px;
  background: transparent;
  border: 1px solid #010101;
  border-radius: 50%%;
}
strong {
  color: inherit;
  font-size: inherit;
  font-weight: 500;
}
a {
  color: inherit;
  text-decoration: none;
}
code.pill {
  display: inline;
  margin: 0 1px;
  padding: 2px 5px;
  color: #333333;
  background: #f3f3f3;
  border-radius: 3px;
  font-family: inherit;
  font-size: 10pt;
  line-height: 1.30;
  white-space: nowrap;
}
.subtitle {
  color: #010101;
  font-size: 10pt;
  line-height: 1.30;
}
.date {
  color: #010101;
  font-size: 10pt;
  line-height: 1.30;
}
.split-block {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin: 0 0 2px;
  break-inside: avoid;
}
.split-column {
  min-width: 0;
}
.split-left {
  flex: 1 1 auto;
}
.split-right {
  flex: 0 1 auto;
  margin-left: auto;
  text-align: right;
}
.split-single .split-column {
  width: 100%%;
}
.split-single .split-right {
  text-align: right;
}
.split-column p {
  margin: 0;
  color: #010101;
}
.split-column p.date {
  color: #010101;
  font-size: 10pt;
  line-height: 1.30;
}
.inline-icon {
  width: 1.1em;
  height: 1.1em;
  margin-right: 3px;
  overflow: visible;
  fill: currentColor;
  vertical-align: -0.16em;
}
@media print { #sheet-root { min-height: 0; } }
`, layout.FontStack.cssFamily(), lineHeight, formatNumber(layout.MarginTopMM),
		formatNumber(layout.MarginBottomMM), sectionGap, itemGap, formatNumber(pageMarginXMM))
	return css.String()
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func validateRangeValue(name string, value, min, max float64, allowZero bool) error {
	if !finite(value) {
		return fmt.Errorf("%s 必须是有限数值", name)
	}
	if value == 0 && allowZero {
		return nil
	}
	if value < min || value > max {
		return fmt.Errorf("%s 必须在 %s 到 %s 之间", name, formatNumber(min), formatNumber(max))
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
