package render

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tokicv/internal/dialect"
)

func TestRenderResumeDocumentHTML(t *testing.T) {
	doc := dialect.Parse("# 示例文档\n\n## 工作经历 <核心>\n\n熟悉 **Go** 与 [文档](https://example.com/docs?a=1&b=2)\n\n- 工程化\n")

	got := Render(doc)
	for _, want := range []string{
		`<main id="sheet-root">`,
		`<h1>示例文档</h1>`,
		`<h2>工作经历 &lt;核心&gt;</h2>`,
		`<p>熟悉 <strong>Go</strong> 与 <a href="https://example.com/docs?a=1&amp;b=2">文档</a></p>`,
		`<ul class="resume-list level-1">`,
		`<li>工程化</li>`,
		`</ul>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() does not contain %q", want)
		}
	}
}

func TestRenderSplitBlocksNestedListsPillsAndIcons(t *testing.T) {
	src := `# 示例文档

::: left
icon:info 信息字段
icon:github [主页](https://example.com/me)
:::
:::right
icon:phone 联系方式字段
icon:email 2 [邮箱](mailto:hello@example.com)
:::

## 经历 ` + "`后端开发`" + `

- 一级
  - 二级
    - 三级
- icon:weixin 微信
- icon:blog 博客
`

	got := Render(dialect.Parse(src))
	for _, want := range []string{
		`<div class="split-block">`,
		`<div class="split-column split-left">`,
		`<div class="split-column split-right">`,
		`<svg class="inline-icon icon-info"`,
		`<svg class="inline-icon icon-github"`,
		`<svg class="inline-icon icon-phone"`,
		`<svg class="inline-icon icon-email2"`,
		`<svg class="inline-icon icon-weixin"`,
		`<svg class="inline-icon icon-blog"`,
		`<code class="pill">后端开发</code>`,
		`<ul class="resume-list level-1">`,
		`<ul class="resume-list level-2">`,
		`<ul class="resume-list level-3">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() does not contain %q", want)
		}
	}
}

func TestRenderAllSevenIconDirectives(t *testing.T) {
	for _, name := range []string{"info", "github", "phone", "email", "email2", "weixin", "blog"} {
		t.Run(name, func(t *testing.T) {
			html := Render(dialect.Parse("icon:" + name + " 内容\n"))
			want := `<svg class="inline-icon icon-` + name + `"`
			if !strings.Contains(html, want) {
				t.Errorf("Render() does not contain %q", want)
			}
		})
	}
}

func TestContactIconGeometryMatchesTheReferenceSilhouettes(t *testing.T) {
	phone := iconGlyphs["phone"]
	if phone.viewBox != "0 0 16 16" ||
		!strings.Contains(phone.body, `<rect x="3.6" y="0.5" width="8.8" height="15" rx="1.5" fill="currentColor"/>`) ||
		!strings.Contains(phone.body, `<rect x="4.9" y="2.6" width="6.2" height="9.5" fill="#ffffff"/>`) ||
		!strings.Contains(phone.body, `<circle cx="8" cy="13.8" r=".85" fill="#ffffff"/>`) {
		t.Fatalf("phone icon is not the reference handset silhouette: %#v", phone)
	}
	email2 := iconGlyphs["email2"]
	if email2.viewBox != "0 0 16 16" ||
		!strings.Contains(email2.body, `translate(-.48 -.48) scale(1.03)`) ||
		!strings.Contains(email2.body, `<rect x="1.6" y="3.2"`) ||
		strings.Contains(email2.body, `fill="#ffffff"`) ||
		!strings.Contains(email2.body, `stroke="currentColor"`) {
		t.Fatalf("email2 icon is not the outlined envelope: %#v", email2)
	}
	for name, glyph := range iconGlyphs {
		if glyph.viewBox != "0 0 16 16" {
			t.Errorf("icon %s has viewBox %q, want 0 0 16 16", name, glyph.viewBox)
		}
	}
}

func TestRenderCalibratedSplitPillRuleAndAlignment(t *testing.T) {
	src := `::: left
**示例科技有限公司** ` + "`平台后端实习生`" + `
:::
::: right
**2025.02 - 2025.08**
:::
`
	html := Render(dialect.Parse(src))
	for _, want := range []string{
		`<p><strong>示例科技有限公司</strong> <code class="pill">平台后端实习生</code></p>`,
		`<p class="date"><strong>2025.02 - 2025.08</strong></p>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Render() does not contain %q", want)
		}
	}

	for _, tc := range []struct {
		selector string
		want     []string
	}{
		{".split-block", []string{`display: flex`, `align-items: flex-start`, `gap: 12px`}},
		{".split-right", []string{`margin-left: auto`, `text-align: right`}},
		{"code.pill", []string{
			`color: #333333`,
			`background: #f3f3f3`,
			`border-radius: 3px`,
			`font-size: 10pt`,
		}},
		{".split-column p.date", []string{`color: #010101`, `font-size: 10pt`}},
		{".inline-icon", []string{`width: 1.1em`, `height: 1.1em`, `vertical-align: -0.16em`}},
		{".resume-list", []string{`padding: 0 0 0 1.3em`}},
		{".resume-list .resume-list", []string{`padding-left: 2.06em`}},
		{".resume-list > li::before", []string{`left: -15.7px`, `top: 0.33em`, `width: 5px`, `height: 5px`}},
		{".resume-list .resume-list > li::before", []string{`left: -15.7px`, `top: 0.33em`, `width: 5px`, `height: 5px`, `border: 1px solid #010101`, `border-radius: 50%`}},
	} {
		rule := cssRule(t, html, tc.selector)
		for _, want := range tc.want {
			if !strings.Contains(rule, want) {
				t.Errorf("%s rule %q does not contain %q", tc.selector, rule, want)
			}
		}
	}
	pillRule := cssRule(t, html, "code.pill")
	if strings.Contains(pillRule, "border:") || strings.Contains(pillRule, "border-width:") {
		t.Errorf("code.pill rule %q still draws a border", pillRule)
	}
}

func TestRenderLocksSheetAndTypography(t *testing.T) {
	html := Render(dialect.Parse("# 标题\n\n正文\n\n- 列表\n"))

	for _, want := range []string{
		`@page { size: 210mm 297mm; margin: 0; }`,
		`width: 210mm`,
		`min-height: 297mm`,
		`--margin-top: 10mm`,
		`--margin-bottom: 10mm`,
		`padding: var(--margin-top) 13.229mm var(--margin-bottom)`,
		`"Nunito", "PingFang SC", "Noto Sans CJK SC", "Microsoft YaHei", sans-serif`,
		`@media print { #sheet-root { min-height: 0; } }`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Render() does not lock %q", want)
		}
	}

	for _, tc := range []struct {
		selector string
		want     []string
	}{
		{":root", []string{`--section-gap: 12px`, `--item-gap: 5px`}},
		{"body", []string{`overflow-wrap: break-word`}},
		{"h1", []string{`font-size: 22.5pt`, `font-weight: 500`, `color: #000000`}},
		{"h2", []string{`font-size: 15pt`, `font-weight: 500`, `color: #000000`, `width: 100%`, `border-bottom: 1px solid #000000`}},
		{"h3", []string{`font-size: 11.5pt`, `font-weight: 500`, `color: #111111`}},
		{"body", []string{`font-size: 10.5pt`}},
		{"p", []string{`font-size: 10.5pt`}},
		{".resume-list > li", []string{`font-size: 10.5pt`}},
		{"strong", []string{`font-size: inherit`, `font-weight: 500`}},
		{".subtitle", []string{`font-size: 10pt`}},
		{".date", []string{`font-size: 10pt`, `color: #010101`}},
	} {
		rule := cssRule(t, html, tc.selector)
		for _, want := range tc.want {
			if !strings.Contains(rule, want) {
				t.Errorf("%s rule %q does not contain %q", tc.selector, rule, want)
			}
		}
	}
}

func TestRenderFontStacks(t *testing.T) {
	for _, tc := range []struct {
		font FontStack
		want string
	}{
		{FontClassic, `"Nunito", "PingFang SC", "Noto Sans CJK SC", "Microsoft YaHei", sans-serif`},
		{FontNoto, `"Nunito", "Noto Sans CJK SC", sans-serif`},
		{FontSerif, `"Nunito", "Noto Serif CJK SC", serif`},
		{FontSystem, `"Nunito", system-ui, sans-serif`},
	} {
		t.Run(string(tc.font), func(t *testing.T) {
			layout := DefaultLayout()
			layout.FontStack = tc.font
			html := RenderWithLayout(dialect.Document{}, layout)
			if !strings.Contains(html, `font-family: `+tc.want+`;`) {
				t.Errorf("RenderWithLayout() does not contain font stack %q", tc.want)
			}
		})
	}
}

func TestRenderLayoutOptions(t *testing.T) {
	layout := Layout{
		FontStack:      FontSerif,
		LineHeight:     1.30,
		MarginTopMM:    9,
		MarginBottomMM: 7.5,
		SectionGapPX:   8,
		ItemGapPX:      3,
	}
	html := RenderWithLayout(dialect.Document{}, layout)
	for _, want := range []string{
		`--margin-top: 9mm`,
		`--margin-bottom: 7.5mm`,
		`font-family: "Nunito", "Noto Serif CJK SC", serif;`,
		`line-height: 1.3;`,
		`--section-gap: 8px;`,
		`--item-gap: 3px;`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("RenderWithLayout() does not contain %q", want)
		}
	}
}

func TestRenderEnforcesReadableLineHeightFloor(t *testing.T) {
	layout := DefaultLayout()
	layout.LineHeight = 1.0
	html := RenderWithLayout(dialect.Document{}, layout)
	if !strings.Contains(html, `line-height: 1.2;`) {
		t.Error("RenderWithLayout() allowed line-height below the readable floor")
	}
}

func TestRenderKeepsFontSizesLockedWithLayoutOptions(t *testing.T) {
	layout := Layout{
		FontStack:    FontSystem,
		LineHeight:   1.30,
		MarginVMM:    6,
		SectionGapPX: 5,
		ItemGapPX:    1.5,
	}
	html := RenderWithLayout(dialect.Parse("# 标题\n\n## 二级\n\n### 三级\n\n正文\n\n- 列表\n"), layout)

	for _, tc := range []struct {
		selector string
		want     []string
	}{
		{"h1", []string{`font-size: 22.5pt`}},
		{"h2", []string{`font-size: 15pt`}},
		{"h3", []string{`font-size: 11.5pt`}},
		{"body", []string{`font-size: 10.5pt`}},
		{"p", []string{`font-size: 10.5pt`}},
		{".resume-list > li", []string{`font-size: 10.5pt`}},
		{"strong", []string{`font-size: inherit`}},
		{"code.pill", []string{`font-size: 10pt`}},
	} {
		rule := cssRule(t, html, tc.selector)
		for _, want := range tc.want {
			if !strings.Contains(rule, want) {
				t.Errorf("%s rule %q does not contain %q", tc.selector, rule, want)
			}
		}
	}
	splitParagraphRule := cssRule(t, html, ".split-column p")
	if strings.Contains(splitParagraphRule, "font-size:") ||
		strings.Contains(splitParagraphRule, "line-height:") {
		t.Errorf(".split-column p rule %q overrides the locked body typography", splitParagraphRule)
	}
}

func TestRenderEscapesRawHTMLAndFallsBackForUnknownIcon(t *testing.T) {
	html := Render(dialect.Parse("# <script>alert(1)</script>\n\nicon:unknown 内容\n"))
	for _, want := range []string{
		`&lt;script&gt;alert(1)&lt;/script&gt;`,
		`icon:unknown 内容`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Render() does not contain %q", want)
		}
	}
	if strings.Contains(html, `<script>`) {
		t.Error("Render() emitted raw HTML from user text")
	}
	if strings.Contains(html, `icon-unknown`) {
		t.Error("Render() rendered an SVG for an unknown icon")
	}
}

func TestRenderStrongInheritsParentFontSize(t *testing.T) {
	doc := dialect.Parse("# **一级标题**\n\n## **二级标题**\n\n### **三级标题**\n\n正文 **粗体**\n\n- 列表 **粗体**\n")
	got := Render(doc)

	for _, want := range []string{
		`<h1><strong>一级标题</strong></h1>`,
		`<h2><strong>二级标题</strong></h2>`,
		`<h3><strong>三级标题</strong></h3>`,
		`<p>正文 <strong>粗体</strong></p>`,
		`<li>列表 <strong>粗体</strong></li>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() does not contain %q", want)
		}
	}
	if rule := cssRule(t, got, "strong"); !strings.Contains(rule, `font-size: inherit`) {
		t.Errorf("strong rule %q does not inherit its parent font size", rule)
	}
}

func TestRenderEmbedsBundledNunitoFonts(t *testing.T) {
	html := Render(dialect.Document{})

	for _, weight := range []string{"400", "500", "600", "700"} {
		name := "nunito-latin-" + weight + "-normal.woff2"
		data, err := os.ReadFile(filepath.Join("assets", "fonts", name))
		if err != nil {
			t.Fatal(err)
		}
		want := "font-weight: " + weight + ";" + "\n" +
			"  font-display: block;\n" +
			"  src: url(data:font/woff2;base64," + base64.StdEncoding.EncodeToString(data) + `) format("woff2");`
		if !strings.Contains(html, want) {
			t.Errorf("Render() does not embed %s", name)
		}
	}
}

func cssRule(t *testing.T, html, selector string) string {
	t.Helper()
	start := strings.Index(html, "\n"+selector+" {")
	if start < 0 {
		t.Fatalf("no %s rule", selector)
	}
	start += len(selector) + 2
	end := strings.IndexByte(html[start:], '}')
	if end < 0 {
		t.Fatalf("unterminated %s rule", selector)
	}
	return html[start : start+end]
}
