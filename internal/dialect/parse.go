package dialect

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Document is a parsed Resume Document.
type Document struct {
	Blocks []Block
}

// Block is a top-level Resume Dialect construct.
type Block interface {
	isBlock()
}

// Inline is content that appears within a Block.
type Inline interface {
	isInline()
}

// Heading is an H1, H2, or H3.
type Heading struct {
	Level    int
	Children []Inline
}

func (Heading) isBlock() {}

// Paragraph is a run of inline content separated from other blocks by a
// blank line.
type Paragraph struct {
	Children []Inline
}

func (Paragraph) isBlock() {}

// SplitBlock is a paired two-column band. A Split Block with only one side is
// rendered as a single column.
type SplitBlock struct {
	Left  []Block
	Right []Block
}

func (SplitBlock) isBlock() {}

// List is an unordered list. Items may contain nested Lists.
type List struct {
	Items []ListItem
}

func (List) isBlock() {}

// ListItem is one item of a List.
type ListItem struct {
	Children []Inline
	Items    []ListItem
}

// Text is literal inline content.
type Text struct {
	Value string
}

func (Text) isInline() {}

// Strong is inline emphasis written as **text**.
type Strong struct {
	Children []Inline
}

func (Strong) isInline() {}

// Link is an inline link written as [text](url).
type Link struct {
	URL      string
	Children []Inline
}

func (Link) isInline() {}

// Pill is an inline code span rendered as a compact badge.
type Pill struct {
	Value string
}

func (Pill) isInline() {}

// Icon is an inline icon directive. Unknown directives keep Raw for plain-text
// fallback rendering.
type Icon struct {
	Name  string
	Raw   string
	Known bool
}

func (Icon) isInline() {}

// Parse parses a Resume Document through Goldmark and the Resume Dialect
// extensions.
func Parse(src string) Document {
	return Analyze(src).Document
}

type parser struct {
	lines       []string
	diagnostics []Diagnostic
	markdown    gmparser.Parser
}

type splitSide int

const (
	splitNone splitSide = iota
	splitLeft
	splitRight
)

type splitMarker struct {
	side      splitSide
	closing   bool
	start     int
	endColumn int
}

type rawBlock struct {
	block    Block
	side     splitSide
	openLine int
	openCol  int
	closed   bool
}

type headingLine struct {
	level         int
	content       string
	contentColumn int
}

type listItemLine struct {
	indent        int
	content       string
	contentColumn int
}

func newParser(src string) *parser {
	lines := strings.Split(normalizeSource(src), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return &parser{
		lines:    lines,
		markdown: goldmark.New().Parser(),
	}
}

func normalizeSource(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	return strings.ReplaceAll(src, "\r", "\n")
}

func (p *parser) parseDocument() Document {
	return Document{Blocks: p.finalize(p.parseRange(0, len(p.lines), false))}
}

func (p *parser) parseRange(start, end int, splitContent bool) []rawBlock {
	var blocks []rawBlock
	for i := start; i < end; {
		if strings.TrimSpace(p.lines[i]) == "" {
			i++
			continue
		}

		if marker, ok := parseSplitMarker(p.lines[i]); ok {
			if marker.closing {
				p.diagnostic(i+1, marker.start, marker.endColumn, SeverityError,
					CodeUnexpectedSplitClose, "意外的 ::: 结束标记")
				i++
				continue
			}
			block, next := p.parseSplit(i, end, marker)
			blocks = append(blocks, block)
			i = next
			continue
		}

		next := p.nextSplitMarker(i, end)
		for _, block := range p.parseMarkdown(i, next, splitContent) {
			blocks = append(blocks, rawBlock{block: block})
		}
		i = next
	}
	return blocks
}

func (p *parser) parseSplit(open, end int, marker splitMarker) (rawBlock, int) {
	closeLine := -1
	next := end
	fence := fenceState{}
	for i := open + 1; i < end; i++ {
		if fence.consumeLine(p.lines[i]) {
			continue
		}
		other, ok := parseSplitMarker(p.lines[i])
		if !ok {
			continue
		}
		if other.closing {
			closeLine = i
			next = i + 1
		} else {
			next = i
		}
		break
	}

	if closeLine < 0 {
		p.diagnostic(open+1, marker.start, marker.endColumn, SeverityError,
			CodeUnclosedSplit, "Split Block 未闭合，按单列渲染")
	}
	contentEnd := next
	if closeLine >= 0 {
		contentEnd = closeLine
	}
	content := p.finalize(p.parseRange(open+1, contentEnd, true))
	block := SplitBlock{}
	if marker.side == splitLeft {
		block.Left = content
	} else {
		block.Right = content
	}
	return rawBlock{
		block:    block,
		side:     marker.side,
		openLine: open + 1,
		openCol:  marker.start,
		closed:   closeLine >= 0,
	}, next
}

func (p *parser) nextSplitMarker(start, end int) int {
	fence := fenceState{}
	for i := start; i < end; i++ {
		if fence.consumeLine(p.lines[i]) {
			continue
		}
		if _, ok := parseSplitMarker(p.lines[i]); ok {
			return i
		}
	}
	return end
}

func (p *parser) parseMarkdown(start, end int, splitParagraphs bool) []Block {
	if start >= end {
		return nil
	}
	source := []byte(strings.Join(p.lines[start:end], "\n"))
	root := p.markdown.Parse(text.NewReader(source))
	converter := astConverter{source: source, splitParagraphs: splitParagraphs}
	var blocks []Block
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		blocks = append(blocks, converter.blocks(node)...)
	}
	return blocks
}

func (p *parser) finalize(raw []rawBlock) []Block {
	blocks := make([]Block, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		current := raw[i]
		if current.side == splitLeft && current.closed &&
			i+1 < len(raw) && raw[i+1].side == splitRight && raw[i+1].closed {
			right := raw[i+1]
			blocks = append(blocks, SplitBlock{
				Left:  current.block.(SplitBlock).Left,
				Right: right.block.(SplitBlock).Right,
			})
			i++
			continue
		}
		if current.side != splitNone && current.closed {
			p.diagnostic(current.openLine, current.openCol, current.openCol+3, SeverityWarning,
				CodeUnpairedSplit, "Split Block 未配对，按单列渲染")
		}
		blocks = append(blocks, current.block)
	}
	return blocks
}

type astConverter struct {
	source          []byte
	splitParagraphs bool
}

func (c astConverter) blocks(node ast.Node) []Block {
	switch node := node.(type) {
	case *ast.Heading:
		children := c.inlines(node)
		if node.Level > 3 {
			return []Block{Paragraph{Children: children}}
		}
		return []Block{Heading{Level: node.Level, Children: children}}
	case *ast.Paragraph:
		return c.paragraphs(node)
	case *ast.TextBlock:
		return []Block{Paragraph{Children: c.inlines(node)}}
	case *ast.List:
		return []Block{c.list(node)}
	case *ast.Blockquote:
		var blocks []Block
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			blocks = append(blocks, c.blocks(child)...)
		}
		return blocks
	case *ast.CodeBlock:
		return []Block{Paragraph{Children: rawBlockText(node.Lines().Value(c.source))}}
	case *ast.FencedCodeBlock:
		return []Block{Paragraph{Children: rawBlockText(node.Lines().Value(c.source))}}
	case *ast.HTMLBlock:
		value := append([]byte(nil), node.Lines().Value(c.source)...)
		if node.HasClosure() {
			value = append(value, node.ClosureLine.Value(c.source)...)
		}
		return []Block{Paragraph{Children: rawBlockText(value)}}
	case *ast.ThematicBreak:
		return []Block{Paragraph{Children: []Inline{Text{Value: "---"}}}}
	default:
		if node.Type() == ast.TypeInline {
			return []Block{Paragraph{Children: c.inlines(node)}}
		}
		return nil
	}
}

func (c astConverter) paragraphs(node *ast.Paragraph) []Block {
	if !c.splitParagraphs {
		return []Block{Paragraph{Children: c.inlines(node)}}
	}

	var (
		blocks []Block
		group  []Inline
	)
	flush := func() {
		if len(group) == 0 {
			return
		}
		blocks = append(blocks, Paragraph{Children: group})
		group = nil
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*ast.Text); ok &&
			(textNode.SoftLineBreak() || textNode.HardLineBreak()) {
			value := resolveMarkdownText(textNode.Segment.Value(c.source))
			group = appendInlines(group, inlineText(value))
			flush()
			continue
		}
		group = appendInlines(group, c.inline(child))
	}
	flush()
	return blocks
}

func (c astConverter) list(node *ast.List) List {
	list := List{}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*ast.ListItem)
		if !ok {
			continue
		}
		list.Items = append(list.Items, c.listItem(item))
	}
	return list
}

func (c astConverter) listItem(node *ast.ListItem) ListItem {
	item := ListItem{}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child := child.(type) {
		case *ast.TextBlock, *ast.Paragraph:
			children := c.inlines(child)
			if len(item.Children) > 0 && len(children) > 0 {
				item.Children = appendText(item.Children, "\n")
			}
			item.Children = appendInlines(item.Children, children)
		case *ast.List:
			item.Items = append(item.Items, c.list(child).Items...)
		}
	}
	return item
}

func (c astConverter) inlines(parent ast.Node) []Inline {
	var out []Inline
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		out = appendInlines(out, c.inline(child))
	}
	return out
}

func (c astConverter) inline(node ast.Node) []Inline {
	switch node := node.(type) {
	case *ast.Text:
		value := resolveMarkdownText(node.Segment.Value(c.source))
		out := inlineText(value)
		if node.SoftLineBreak() || node.HardLineBreak() {
			out = appendText(out, "\n")
		}
		return out
	case *ast.String:
		return inlineText(string(node.Value))
	case *ast.Emphasis:
		if node.Level == 2 {
			return []Inline{Strong{Children: c.inlines(node)}}
		}
		return inlineText("*" + c.rawText(node) + "*")
	case *ast.CodeSpan:
		return []Inline{Pill{Value: c.codeSpanText(node)}}
	case *ast.Link:
		return []Inline{Link{
			URL:      resolveMarkdownURL(node.Destination),
			Children: c.inlines(node),
		}}
	case *ast.AutoLink:
		label := resolveMarkdownText(node.Label(c.source))
		return []Inline{Link{
			URL:      resolveMarkdownURL(node.URL(c.source)),
			Children: []Inline{Text{Value: label}},
		}}
	case *ast.RawHTML:
		return []Inline{Text{Value: string(node.Segments.Value(c.source))}}
	case *ast.Image:
		return inlineText(c.rawText(node))
	default:
		if node.Type() == ast.TypeInline {
			return c.inlines(node)
		}
		return nil
	}
}

func (c astConverter) codeSpanText(node *ast.CodeSpan) string {
	var value strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			continue
		}
		part := string(textNode.Segment.Value(c.source))
		if strings.HasSuffix(part, "\n") {
			part = strings.TrimSuffix(part, "\n") + " "
		}
		value.WriteString(part)
	}
	return value.String()
}

func (c astConverter) rawText(node ast.Node) string {
	var value strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child := child.(type) {
		case *ast.Text:
			value.Write(child.Segment.Value(c.source))
		case *ast.String:
			value.Write(child.Value)
		case *ast.RawHTML:
			value.Write(child.Segments.Value(c.source))
		default:
			value.WriteString(c.rawText(child))
		}
	}
	return value.String()
}

func resolveMarkdownText(value []byte) string {
	value = util.UnescapePunctuations(value)
	value = util.ResolveNumericReferences(value)
	return string(util.ResolveEntityNames(value))
}

func resolveMarkdownURL(value []byte) string {
	return resolveMarkdownText(value)
}

func rawBlockText(value []byte) []Inline {
	value = []byte(strings.TrimSuffix(string(value), "\n"))
	return []Inline{Text{Value: string(value)}}
}

func inlineText(value string) []Inline {
	var out []Inline
	start := 0
	for i := 0; i < len(value); {
		if strings.HasPrefix(value[i:], "icon:") {
			if directive, ok := parseIconDirective(value, i); ok {
				out = appendText(out, value[start:i])
				out = append(out, Icon{
					Name:  directive.name,
					Raw:   directive.raw,
					Known: directive.known,
				})
				i = directive.end
				start = i
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(value[i:])
		if size == 0 {
			break
		}
		i += size
	}
	return appendText(out, value[start:])
}

type linkParts struct {
	label      string
	labelStart int
	url        string
	end        int
}

func parseLink(src string, start int) (linkParts, bool) {
	textEnd := strings.IndexByte(src[start+1:], ']')
	if textEnd <= 0 {
		return linkParts{}, false
	}
	labelStart := start + 1
	urlStart := labelStart + textEnd + 1
	if urlStart >= len(src) || src[urlStart] != '(' {
		return linkParts{}, false
	}
	urlEnd := strings.IndexByte(src[urlStart+1:], ')')
	if urlEnd <= 0 {
		return linkParts{}, false
	}
	url := src[urlStart+1 : urlStart+1+urlEnd]
	label := src[labelStart : labelStart+textEnd]
	if label == "" || url == "" {
		return linkParts{}, false
	}
	return linkParts{
		label:      label,
		labelStart: labelStart,
		url:        url,
		end:        urlStart + urlEnd + 2,
	}, true
}

type iconDirective struct {
	name  string
	raw   string
	known bool
	end   int
}

func parseIconDirective(src string, start int) (iconDirective, bool) {
	nameStart := start + len("icon:")
	end := nameStart
	for end < len(src) && (isASCIILetter(src[end]) || isASCIIDigit(src[end])) {
		end++
	}
	if end == nameStart {
		return iconDirective{}, false
	}
	name := strings.ToLower(src[nameStart:end])
	rawEnd := end
	if name == "email" {
		spaces := end
		for spaces < len(src) && (src[spaces] == ' ' || src[spaces] == '\t') {
			spaces++
		}
		if spaces < len(src) && src[spaces] == '2' &&
			(spaces+1 == len(src) ||
				!isASCIILetter(src[spaces+1]) && !isASCIIDigit(src[spaces+1])) {
			name = "email2"
			rawEnd = spaces + 1
		}
	}
	known := false
	switch name {
	case "info", "github", "phone", "email", "email2", "weixin", "blog":
		known = true
	}
	return iconDirective{
		name:  name,
		raw:   src[start:rawEnd],
		known: known,
		end:   rawEnd,
	}, true
}

func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func appendText(out []Inline, value string) []Inline {
	if value == "" {
		return out
	}
	if len(out) > 0 {
		if text, ok := out[len(out)-1].(Text); ok {
			out[len(out)-1] = Text{Value: text.Value + value}
			return out
		}
	}
	return append(out, Text{Value: value})
}

func appendInlines(out []Inline, more []Inline) []Inline {
	for _, inline := range more {
		if text, ok := inline.(Text); ok {
			out = appendText(out, text.Value)
			continue
		}
		out = append(out, inline)
	}
	return out
}

func (p *parser) diagnostic(line, start, end int, severity Severity, code, message string) {
	p.diagnostics = append(p.diagnostics, Diagnostic{
		Line:      line,
		Column:    start,
		EndColumn: end,
		Severity:  severity,
		Code:      code,
		Message:   message,
	})
}

func parseSplitMarker(line string) (splitMarker, bool) {
	leading := 0
	for leading < len(line) && (line[leading] == ' ' || line[leading] == '\t') {
		leading++
	}
	if !strings.HasPrefix(line[leading:], ":::") {
		return splitMarker{}, false
	}
	marker := splitMarker{
		start:     runeCount(line[:leading]) + 1,
		endColumn: runeCount(line[:leading]) + 4,
	}
	rest := line[leading+3:]
	if strings.TrimSpace(rest) == "" {
		marker.closing = true
		return marker, true
	}
	switch strings.ToLower(strings.TrimSpace(rest)) {
	case "left":
		marker.side = splitLeft
	case "right":
		marker.side = splitRight
	default:
		return splitMarker{}, false
	}
	return marker, true
}

func parseHeading(line string) (headingLine, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 3 || level == len(line) {
		return headingLine{}, false
	}
	if line[level] != ' ' && line[level] != '\t' {
		return headingLine{}, false
	}
	content, column := trimInlineContent(line[level:], level)
	if content == "" {
		return headingLine{}, false
	}
	return headingLine{level: level, content: content, contentColumn: column}, true
}

func parseListItem(line string) (listItemLine, bool) {
	indent := 0
	i := 0
	for i < len(line) {
		switch line[i] {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			goto marker
		}
		i++
	}
marker:
	if i+1 >= len(line) {
		return listItemLine{}, false
	}
	switch line[i] {
	case '-', '*', '+':
	default:
		return listItemLine{}, false
	}
	if line[i+1] != ' ' && line[i+1] != '\t' {
		return listItemLine{}, false
	}
	content, column := trimInlineContent(line[i+2:], runeCount(line[:i+2]))
	if content == "" {
		return listItemLine{}, false
	}
	return listItemLine{indent: indent, content: content, contentColumn: column}, true
}

func trimInlineContent(src string, column int) (string, int) {
	trimmed := strings.TrimLeftFunc(src, unicode.IsSpace)
	leadingBytes := len(src) - len(trimmed)
	return strings.TrimSpace(trimmed), column + runeCount(src[:leadingBytes])
}

func runeCount(src string) int {
	return utf8.RuneCountInString(src)
}

type fenceState struct {
	open   bool
	marker byte
	length int
}

func (f *fenceState) consumeLine(line string) bool {
	if f.open {
		if fenceClosing(line, f.marker, f.length) {
			f.open = false
		}
		return true
	}
	marker, length, ok := fenceOpening(line)
	if !ok {
		return false
	}
	f.open = true
	f.marker = marker
	f.length = length
	return true
}

func fenceOpening(line string) (byte, int, bool) {
	trimmed := trimFenceIndent(line)
	if len(trimmed) < 3 {
		return 0, 0, false
	}
	marker := trimmed[0]
	if marker != '`' && marker != '~' {
		return 0, 0, false
	}
	length := 0
	for length < len(trimmed) && trimmed[length] == marker {
		length++
	}
	return marker, length, length >= 3
}

func fenceClosing(line string, marker byte, length int) bool {
	trimmed := trimFenceIndent(line)
	count := 0
	for count < len(trimmed) && trimmed[count] == marker {
		count++
	}
	return count >= length && strings.TrimSpace(trimmed[count:]) == ""
}

func trimFenceIndent(line string) string {
	indent := 0
	for indent < len(line) && indent < 4 && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return ""
	}
	return line[indent:]
}
