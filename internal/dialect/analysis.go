package dialect

import (
	"sort"
	"strings"
)

// Severity is the user-facing importance of a syntax diagnostic.
type Severity string

// Diagnostic severities.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Syntax diagnostic codes.
const (
	CodeUnclosedSplit        = "unclosed_split"
	CodeUnexpectedSplitClose = "unexpected_split_close"
	CodeUnpairedSplit        = "unpaired_split"
	CodeUnknownIcon          = "unknown_icon"
)

// Diagnostic is a line-indexed Resume Dialect syntax issue. Lines and columns
// are 1-based, and EndColumn is exclusive.
type Diagnostic struct {
	Line      int      `json:"line"`
	Column    int      `json:"column"`
	EndColumn int      `json:"endColumn"`
	Severity  Severity `json:"severity"`
	Code      string   `json:"code"`
	Message   string   `json:"message"`
}

// LineKind is the editor role of one source line.
type LineKind string

// Resume Dialect line roles.
const (
	LineBlank      LineKind = "blank"
	LineText       LineKind = "text"
	LineHeading    LineKind = "heading"
	LineList       LineKind = "list"
	LineSplitOpen  LineKind = "split_open"
	LineSplitClose LineKind = "split_close"
)

// TokenKind is a syntax-highlightable Resume Dialect token.
type TokenKind string

// Resume Dialect token roles.
const (
	TokenHeading     TokenKind = "heading"
	TokenSplitMarker TokenKind = "split_marker"
	TokenIcon        TokenKind = "icon"
	TokenStrong      TokenKind = "strong"
	TokenLink        TokenKind = "link"
	TokenPill        TokenKind = "pill"
)

// Token is a 1-based, end-exclusive column span within one source line.
type Token struct {
	Kind        TokenKind `json:"kind"`
	StartColumn int       `json:"startColumn"`
	EndColumn   int       `json:"endColumn"`
}

// LineInfo is the tokenizer and diagnostic metadata for one source line.
type LineInfo struct {
	Number      int          `json:"number"`
	Kind        LineKind     `json:"kind"`
	Tokens      []Token      `json:"tokens,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// Analysis is the parsed document plus editor and diagnostic metadata.
type Analysis struct {
	Document    Document     `json:"-"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Lines       []LineInfo   `json:"lines"`
}

// Analyze parses a Resume Document and returns diagnostics plus line-role and
// token spans for editor syntax highlighting.
func Analyze(src string) Analysis {
	p := newParser(src)
	document := p.parseDocument()
	lines := make([]LineInfo, 0, len(p.lines))
	fence := fenceState{}
	for index, line := range p.lines {
		number := index + 1
		info := LineInfo{
			Number: number,
			Kind:   classifyLine(line),
		}
		if fence.consumeLine(line) {
			info.Kind = LineText
		} else {
			info.Tokens = tokenizeLine(line)
			for _, token := range info.Tokens {
				if token.Kind == TokenIcon {
					p.addUnknownIconDiagnostic(number, line, token)
				}
			}
		}
		lines = append(lines, info)
	}

	sort.SliceStable(p.diagnostics, func(i, j int) bool {
		left, right := p.diagnostics[i], p.diagnostics[j]
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		return left.Code < right.Code
	})
	for index := range lines {
		for _, diagnostic := range p.diagnostics {
			if diagnostic.Line == lines[index].Number {
				lines[index].Diagnostics = append(lines[index].Diagnostics, diagnostic)
			}
		}
	}

	return Analysis{
		Document:    document,
		Diagnostics: p.diagnostics,
		Lines:       lines,
	}
}

func (p *parser) addUnknownIconDiagnostic(lineNumber int, line string, token Token) {
	runes := []rune(line)
	start := clampColumn(token.StartColumn-1, len(runes))
	end := clampColumn(token.EndColumn-1, len(runes))
	raw := string(runes[start:end])
	directive, ok := parseIconDirective(raw, 0)
	if !ok || directive.known {
		return
	}
	p.diagnostic(lineNumber, token.StartColumn, token.EndColumn, SeverityError,
		CodeUnknownIcon, "未知 Icon Directive: "+directive.raw)
}

func clampColumn(column, length int) int {
	if column < 0 {
		return 0
	}
	if column > length {
		return length
	}
	return column
}

func classifyLine(line string) LineKind {
	if strings.TrimSpace(line) == "" {
		return LineBlank
	}
	if marker, ok := parseSplitMarker(line); ok {
		if marker.closing {
			return LineSplitClose
		}
		return LineSplitOpen
	}
	if _, ok := parseHeading(line); ok {
		return LineHeading
	}
	if _, ok := parseListItem(line); ok {
		return LineList
	}
	return LineText
}

func tokenizeLine(line string) []Token {
	var tokens []Token
	if marker, ok := parseSplitMarker(line); ok {
		tokens = append(tokens, Token{
			Kind:        TokenSplitMarker,
			StartColumn: marker.start,
			EndColumn:   marker.endColumn,
		})
		return tokens
	}
	if heading, ok := parseHeading(line); ok {
		tokens = append(tokens, Token{
			Kind:        TokenHeading,
			StartColumn: 1,
			EndColumn:   heading.level + 1,
		})
	}

	tokens = append(tokens, tokenizeInline(line, 0, len(line))...)
	sort.SliceStable(tokens, func(i, j int) bool {
		if tokens[i].StartColumn != tokens[j].StartColumn {
			return tokens[i].StartColumn < tokens[j].StartColumn
		}
		return tokens[i].EndColumn > tokens[j].EndColumn
	})
	return tokens
}

func tokenizeInline(line string, start, end int) []Token {
	var tokens []Token
	for i := start; i < end; {
		if line[i] == '`' {
			if close := strings.IndexByte(line[i+1:end], '`'); close > 0 {
				close += i + 1
				tokens = append(tokens, tokenSpan(line, i, close+1, TokenPill))
				i = close + 1
				continue
			}
		}
		if strings.HasPrefix(line[i:], "**") {
			if close := strings.Index(line[i+2:end], "**"); close > 0 {
				innerStart := i + 2
				innerEnd := innerStart + close
				tokens = append(tokens, tokenSpan(line, i, innerEnd+2, TokenStrong))
				tokens = append(tokens, tokenizeInline(line, innerStart, innerEnd)...)
				i = innerEnd + 2
				continue
			}
		}
		if line[i] == '[' {
			if link, ok := parseLink(line, i); ok {
				if link.end > end {
					i++
					continue
				}
				tokens = append(tokens, tokenSpan(line, i, link.end, TokenLink))
				labelEnd := link.labelStart + len(link.label)
				tokens = append(tokens, tokenizeInline(line, link.labelStart, labelEnd)...)
				i = link.end
				continue
			}
		}
		if strings.HasPrefix(line[i:], "icon:") {
			if directive, ok := parseIconDirective(line, i); ok {
				if directive.end > end {
					i++
					continue
				}
				tokens = append(tokens, tokenSpan(line, i, directive.end, TokenIcon))
				i = directive.end
				continue
			}
		}
		i++
	}
	return tokens
}

func tokenSpan(line string, startByte, endByte int, kind TokenKind) Token {
	return Token{
		Kind:        kind,
		StartColumn: runeCount(line[:startByte]) + 1,
		EndColumn:   runeCount(line[:endByte]) + 1,
	}
}
