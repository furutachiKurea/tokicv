package desktop

import (
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"

	"tokicv/internal/dialect"
)

func resumeLanguage() *editor.Language {
	return &editor.Language{
		Name:       "Resume Dialect",
		Extensions: []string{".md"},
		TabSize:    2,
		Tokenizer:  editor.TokenizerFunc(tokenizeResumeLine),
	}
}

func tokenizeResumeLine(line string, _ uint64, tokens []editor.Token) ([]editor.Token, uint64) {
	if line == "" {
		return tokens, 0
	}
	runes := []rune(line)
	kinds := make([]editor.TokenKind, len(runes))
	priorities := make([]int, len(runes))
	analysis := dialect.Analyze(line)
	if len(analysis.Lines) > 0 {
		for _, token := range analysis.Lines[0].Tokens {
			kind, priority := resumeTokenKind(token.Kind)
			start := clamp(token.StartColumn-1, 0, len(runes))
			end := clamp(token.EndColumn-1, start, len(runes))
			for i := start; i < end; i++ {
				if priority >= priorities[i] {
					kinds[i], priorities[i] = kind, priority
				}
			}
		}
	}

	byteEnd := 0
	runKind := kinds[0]
	for i, r := range runes {
		byteEnd += utf8.RuneLen(r)
		if i == len(runes)-1 || kinds[i+1] != runKind {
			tokens = append(tokens, editor.Token{End: byteEnd, Kind: runKind})
			if i < len(runes)-1 {
				runKind = kinds[i+1]
			}
		}
	}
	return tokens, 0
}

func resumeTokenKind(kind dialect.TokenKind) (editor.TokenKind, int) {
	switch kind {
	case dialect.TokenSplitMarker:
		return editor.TokenKeyword, 5
	case dialect.TokenIcon:
		return editor.TokenAttribute, 4
	case dialect.TokenHeading:
		return editor.TokenHeading, 3
	case dialect.TokenStrong:
		return editor.TokenStrong, 2
	case dialect.TokenLink:
		return editor.TokenLink, 1
	case dialect.TokenPill:
		return editor.TokenCode, 1
	default:
		return editor.TokenText, 0
	}
}

func editorDiagnostics(buffer *editor.Buffer, diagnostics []dialect.Diagnostic) []editor.Diagnostic {
	out := make([]editor.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Line < 1 || diagnostic.Line > buffer.LineCount() {
			continue
		}
		line := buffer.Line(diagnostic.Line - 1)
		start := runeColumnByte(line, diagnostic.Column)
		end := runeColumnByte(line, diagnostic.EndColumn)
		if end < start {
			end = start
		}
		base := buffer.Offset(editor.Pos{Line: diagnostic.Line - 1})
		severity := editor.SeverityWarning
		if diagnostic.Severity == dialect.SeverityError {
			severity = editor.SeverityError
		}
		out = append(out, editor.Diagnostic{
			Start:    base + start,
			End:      base + end,
			Severity: severity,
			Message:  diagnostic.Message,
			Source:   "Resume Dialect",
		})
	}
	return out
}

func runeColumnByte(line string, column int) int {
	if column <= 1 {
		return 0
	}
	count := 1
	for byteOffset := range line {
		if count == column {
			return byteOffset
		}
		count++
	}
	return len(line)
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
