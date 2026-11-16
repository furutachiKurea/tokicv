package dialect

import (
	"reflect"
	"testing"
)

func TestParseHeadingsAndParagraphs(t *testing.T) {
	src := "# 示例文档\n\n## 工作经历\n第一行\n第二行\n\n### tokicv\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		Heading{Level: 1, Children: []Inline{Text{Value: "示例文档"}}},
		Heading{Level: 2, Children: []Inline{Text{Value: "工作经历"}}},
		Paragraph{Children: []Inline{Text{Value: "第一行\n第二行"}}},
		Heading{Level: 3, Children: []Inline{Text{Value: "tokicv"}}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseSingleLevelUnorderedLists(t *testing.T) {
	src := "## 技能\n- Go\n* TypeScript\n\n普通段落\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		Heading{Level: 2, Children: []Inline{Text{Value: "技能"}}},
		List{Items: []ListItem{{Children: []Inline{Text{Value: "Go"}}}}},
		List{Items: []ListItem{{Children: []Inline{Text{Value: "TypeScript"}}}}},
		Paragraph{Children: []Inline{Text{Value: "普通段落"}}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseInlineStrongAndLinks(t *testing.T) {
	src := "## **Go** 与 [tokicv](https://example.com/tokicv)\n\n- 熟悉 **WebView**\n- 访问 [文档](https://example.com/docs)\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		Heading{Level: 2, Children: []Inline{
			Strong{Children: []Inline{Text{Value: "Go"}}},
			Text{Value: " 与 "},
			Link{URL: "https://example.com/tokicv", Children: []Inline{Text{Value: "tokicv"}}},
		}},
		List{Items: []ListItem{
			{Children: []Inline{
				Text{Value: "熟悉 "},
				Strong{Children: []Inline{Text{Value: "WebView"}}},
			}},
			{Children: []Inline{
				Text{Value: "访问 "},
				Link{URL: "https://example.com/docs", Children: []Inline{Text{Value: "文档"}}},
			}},
		}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseMalformedInlineSyntaxAsText(t *testing.T) {
	src := "未闭合 **粗体 和 [链接](缺失\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		Paragraph{Children: []Inline{Text{Value: "未闭合 **粗体 和 [链接](缺失"}}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParsePairedSplitBlocks(t *testing.T) {
	src := "::: left\nicon:info 信息字段\nicon:github [主页](https://example.com/me)\n:::\n:::right\nicon:phone 联系方式字段\nicon:email 2 [邮箱](mailto:hello@example.com)\n:::\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		SplitBlock{
			Left: []Block{
				Paragraph{Children: []Inline{
					Icon{Name: "info", Raw: "icon:info", Known: true},
					Text{Value: " 信息字段"},
				}},
				Paragraph{Children: []Inline{
					Icon{Name: "github", Raw: "icon:github", Known: true},
					Text{Value: " "},
					Link{URL: "https://example.com/me", Children: []Inline{Text{Value: "主页"}}},
				}},
			},
			Right: []Block{
				Paragraph{Children: []Inline{
					Icon{Name: "phone", Raw: "icon:phone", Known: true},
					Text{Value: " 联系方式字段"},
				}},
				Paragraph{Children: []Inline{
					Icon{Name: "email2", Raw: "icon:email 2", Known: true},
					Text{Value: " "},
					Link{URL: "mailto:hello@example.com", Children: []Inline{Text{Value: "邮箱"}}},
				}},
			},
		},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseUnpairedAndUnclosedSplitBlocksFallBackToSingleColumn(t *testing.T) {
	src := "::: left\n左侧内容\n:::\n\n::: right\n右侧内容\n\n::: left\n未闭合内容\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		SplitBlock{Left: []Block{Paragraph{Children: []Inline{Text{Value: "左侧内容"}}}}},
		SplitBlock{Right: []Block{Paragraph{Children: []Inline{Text{Value: "右侧内容"}}}}},
		SplitBlock{Left: []Block{Paragraph{Children: []Inline{Text{Value: "未闭合内容"}}}}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParsePillBadgesAndIconDirectives(t *testing.T) {
	src := "# 示例文档 `后端开发` [icon:blog 博客](https://example.com/blog)\n\n- 熟悉 `Go`、icon:github 与 icon:email 2\n- icon:unknown 保持原文\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		Heading{Level: 1, Children: []Inline{
			Text{Value: "示例文档 "},
			Pill{Value: "后端开发"},
			Text{Value: " "},
			Link{URL: "https://example.com/blog", Children: []Inline{
				Icon{Name: "blog", Raw: "icon:blog", Known: true},
				Text{Value: " 博客"},
			}},
		}},
		List{Items: []ListItem{
			{Children: []Inline{
				Text{Value: "熟悉 "},
				Pill{Value: "Go"},
				Text{Value: "、"},
				Icon{Name: "github", Raw: "icon:github", Known: true},
				Text{Value: " 与 "},
				Icon{Name: "email2", Raw: "icon:email 2", Known: true},
			}},
			{Children: []Inline{
				Icon{Name: "unknown", Raw: "icon:unknown", Known: false},
				Text{Value: " 保持原文"},
			}},
		}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseNestedUnorderedLists(t *testing.T) {
	src := "## 经历\n\n- 一级\n  - 二级 A\n  - 二级 B\n    - 三级\n- 下一个一级\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		Heading{Level: 2, Children: []Inline{Text{Value: "经历"}}},
		List{Items: []ListItem{
			{
				Children: []Inline{Text{Value: "一级"}},
				Items: []ListItem{
					{Children: []Inline{Text{Value: "二级 A"}}},
					{
						Children: []Inline{Text{Value: "二级 B"}},
						Items:    []ListItem{{Children: []Inline{Text{Value: "三级"}}}},
					},
				},
			},
			{Children: []Inline{Text{Value: "下一个一级"}}},
		}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseListsFollowCommonMarkNesting(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want []ListItem
	}{
		{
			name: "starting indented then dedented",
			src:  "  - A\n- B\n",
			want: []ListItem{
				{Children: []Inline{Text{Value: "A"}}},
				{Children: []Inline{Text{Value: "B"}}},
			},
		},
		{
			name: "deep nesting rolls back through an intermediate indent",
			src:  "- A\n    - B\n  - C\n- D\n",
			want: []ListItem{
				{
					Children: []Inline{Text{Value: "A"}},
					Items: []ListItem{
						{Children: []Inline{Text{Value: "B"}}},
						{Children: []Inline{Text{Value: "C"}}},
					},
				},
				{Children: []Inline{Text{Value: "D"}}},
			},
		},
		{
			name: "mixed indent increases and decreases",
			src:  "- A\n  - B\n    - C\n    - D\n  - E\n- F\n   - G\n - H\n",
			want: []ListItem{
				{
					Children: []Inline{Text{Value: "A"}},
					Items: []ListItem{
						{
							Children: []Inline{Text{Value: "B"}},
							Items: []ListItem{
								{Children: []Inline{Text{Value: "C"}}},
								{Children: []Inline{Text{Value: "D"}}},
							},
						},
						{Children: []Inline{Text{Value: "E"}}},
					},
				},
				{
					Children: []Inline{Text{Value: "F"}},
					Items: []ListItem{
						{Children: []Inline{Text{Value: "G"}}},
					},
				},
				{Children: []Inline{Text{Value: "H"}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.src)
			want := Document{Blocks: []Block{List{Items: tc.want}}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Parse() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestParseNormalizesCRLF(t *testing.T) {
	got := Parse("# 标题\r\n\r\n- 项目\r\n  - 子项\r\n")
	want := Document{Blocks: []Block{
		Heading{Level: 1, Children: []Inline{Text{Value: "标题"}}},
		List{Items: []ListItem{{
			Children: []Inline{Text{Value: "项目"}},
			Items:    []ListItem{{Children: []Inline{Text{Value: "子项"}}}},
		}}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestAnalyzeReportsLineIndexedSyntaxDiagnostics(t *testing.T) {
	src := "::: left\nicon:info 信息\n:::\nicon:unknown 内容\n:::\n"

	analysis := Analyze(src)
	want := []Diagnostic{
		{
			Line:      1,
			Column:    1,
			EndColumn: 4,
			Severity:  SeverityWarning,
			Code:      CodeUnpairedSplit,
			Message:   "Split Block 未配对，按单列渲染",
		},
		{
			Line:      4,
			Column:    1,
			EndColumn: 13,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:unknown",
		},
		{
			Line:      5,
			Column:    1,
			EndColumn: 4,
			Severity:  SeverityError,
			Code:      CodeUnexpectedSplitClose,
			Message:   "意外的 ::: 结束标记",
		},
	}

	if !reflect.DeepEqual(analysis.Diagnostics, want) {
		t.Fatalf("Analyze().Diagnostics = %#v, want %#v", analysis.Diagnostics, want)
	}
	if len(analysis.Document.Blocks) != 2 {
		t.Fatalf("Analyze() parsed %d blocks, want a single-column Split Block and a paragraph", len(analysis.Document.Blocks))
	}
}

func TestAnalyzeReportsUnclosedSplitAtEndOfDocument(t *testing.T) {
	analysis := Analyze("::: left\nicon:info 信息")

	want := []Diagnostic{{
		Line:      1,
		Column:    1,
		EndColumn: 4,
		Severity:  SeverityError,
		Code:      CodeUnclosedSplit,
		Message:   "Split Block 未闭合，按单列渲染",
	}}
	if !reflect.DeepEqual(analysis.Diagnostics, want) {
		t.Fatalf("Analyze().Diagnostics = %#v, want %#v", analysis.Diagnostics, want)
	}
	if len(analysis.Document.Blocks) != 1 {
		t.Fatalf("Analyze() parsed %d blocks, want one single-column Split Block", len(analysis.Document.Blocks))
	}
}

func TestAnalyzeExposesEditorLineRolesAndTokens(t *testing.T) {
	src := "## 技能 `Go`\n\n:::left\nicon:email 2 [邮箱](mailto:hello@example.com)\n:::\n"

	analysis := Analyze(src)
	if len(analysis.Lines) != 5 {
		t.Fatalf("Analyze() returned %d line infos, want 5", len(analysis.Lines))
	}
	if kind := analysis.Lines[2].Kind; kind != LineSplitOpen {
		t.Errorf("line 3 kind = %q, want %q", kind, LineSplitOpen)
	}
	if kind := analysis.Lines[4].Kind; kind != LineSplitClose {
		t.Errorf("line 5 kind = %q, want %q", kind, LineSplitClose)
	}
	if got := analysis.Lines[2].Tokens; !reflect.DeepEqual(got, []Token{
		{Kind: TokenSplitMarker, StartColumn: 1, EndColumn: 4},
	}) {
		t.Errorf("split line tokens = %#v", got)
	}
	if got := analysis.Lines[3].Tokens; !reflect.DeepEqual(got, []Token{
		{Kind: TokenIcon, StartColumn: 1, EndColumn: 13},
		{Kind: TokenLink, StartColumn: 14, EndColumn: 44},
	}) {
		t.Errorf("icon/link line tokens = %#v", got)
	}
}

func TestAnalyzeDiagnosticColumnsFollowRawSource(t *testing.T) {
	src := "## icon:heading\n\n  - icon:list\n\n::: left\n  icon:split\n:::\n\n正文 icon:paragraph\n"
	analysis := Analyze(src)

	var got []Diagnostic
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == CodeUnknownIcon {
			got = append(got, diagnostic)
		}
	}
	want := []Diagnostic{
		{
			Line:      1,
			Column:    4,
			EndColumn: 16,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:heading",
		},
		{
			Line:      3,
			Column:    5,
			EndColumn: 14,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:list",
		},
		{
			Line:      6,
			Column:    3,
			EndColumn: 13,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:split",
		},
		{
			Line:      9,
			Column:    4,
			EndColumn: 18,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:paragraph",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown icon diagnostics = %#v, want %#v", got, want)
	}
}

func TestAnalyzeTokenizesNestedInlineSyntax(t *testing.T) {
	analysis := Analyze("**icon:github** [icon:blog](u)\n")
	got := analysis.Lines[0].Tokens
	want := []Token{
		{Kind: TokenStrong, StartColumn: 1, EndColumn: 16},
		{Kind: TokenIcon, StartColumn: 3, EndColumn: 14},
		{Kind: TokenLink, StartColumn: 17, EndColumn: 31},
		{Kind: TokenIcon, StartColumn: 18, EndColumn: 27},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nested inline tokens = %#v, want %#v", got, want)
	}
}

func TestParseUsesCommonMarkBoundariesForHeadingsEscapesAndLinks(t *testing.T) {
	src := "# 标题 ###\n\n段落第一行\n第二行 **粗 *斜* 体** 与 [链接](https://example.com/a_(b))\n\n\\*星号\\* &amp;\n"

	got := Parse(src)
	want := Document{Blocks: []Block{
		Heading{Level: 1, Children: []Inline{Text{Value: "标题"}}},
		Paragraph{Children: []Inline{
			Text{Value: "段落第一行\n第二行 "},
			Strong{Children: []Inline{
				Text{Value: "粗 *斜* 体"},
			}},
			Text{Value: " 与 "},
			Link{
				URL:      "https://example.com/a_(b)",
				Children: []Inline{Text{Value: "链接"}},
			},
		}},
		Paragraph{Children: []Inline{Text{Value: "*星号* &"}}},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseDecodesCommonMarkLinkDestinationReferences(t *testing.T) {
	got := Parse("[文档](https://example.com/?a=1&amp;b=2\\_x)\n")
	want := Document{Blocks: []Block{
		Paragraph{Children: []Inline{
			Link{
				URL:      "https://example.com/?a=1&b=2_x",
				Children: []Inline{Text{Value: "文档"}},
			},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseKeepsRawHTMLAsEscapedLiteralText(t *testing.T) {
	got := Parse("<script>alert(1)</script>\n")
	want := Document{Blocks: []Block{
		Paragraph{Children: []Inline{Text{Value: "<script>alert(1)</script>"}}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseKeepsIconDirectivesInsideRawHTMLLiteral(t *testing.T) {
	src := `<span data-x="icon:github">正文</span>` + "\n"
	got := Parse(src)
	want := Document{Blocks: []Block{
		Paragraph{Children: []Inline{Text{Value: `<span data-x="icon:github">正文</span>`}}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestAnalyzeTreatsFencedCodeAsLiteralText(t *testing.T) {
	src := "```md\n# 不是标题\n- 不是列表\nicon:unknown\n::: left\n```\n"
	analysis := Analyze(src)
	for index, line := range analysis.Lines {
		if line.Kind != LineText {
			t.Errorf("fenced line %d kind = %q, want %q", index+1, line.Kind, LineText)
		}
		if len(line.Tokens) != 0 {
			t.Errorf("fenced line %d tokens = %#v, want none", index+1, line.Tokens)
		}
	}
	if len(analysis.Diagnostics) != 0 {
		t.Fatalf("fenced content diagnostics = %#v, want none", analysis.Diagnostics)
	}
}

func TestParseNormalizesIconWhitespaceCaseAndBoundaries(t *testing.T) {
	src := "icon:EMAIL\t2 邮箱 icon:EMAIL-23 icon:blog.2 icon:unknown\n"
	got := Parse(src)
	want := Document{Blocks: []Block{
		Paragraph{Children: []Inline{
			Icon{Name: "email2", Raw: "icon:EMAIL\t2", Known: true},
			Text{Value: " 邮箱 "},
			Icon{Name: "email", Raw: "icon:EMAIL", Known: true},
			Text{Value: "-23 "},
			Icon{Name: "blog", Raw: "icon:blog", Known: true},
			Text{Value: ".2 "},
			Icon{Name: "unknown", Raw: "icon:unknown", Known: false},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestAnalyzeReportsExactIconColumnsAndIgnoresCodeSpans(t *testing.T) {
	src := "## 标题 icon:heading\n\n- 前缀 **icon:strong** [icon:link](u)\n\n`icon:code` icon:tail\n"
	analysis := Analyze(src)

	var got []Diagnostic
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == CodeUnknownIcon {
			got = append(got, diagnostic)
		}
	}
	want := []Diagnostic{
		{
			Line:      1,
			Column:    7,
			EndColumn: 19,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:heading",
		},
		{
			Line:      3,
			Column:    8,
			EndColumn: 19,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:strong",
		},
		{
			Line:      3,
			Column:    23,
			EndColumn: 32,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:link",
		},
		{
			Line:      5,
			Column:    13,
			EndColumn: 22,
			Severity:  SeverityError,
			Code:      CodeUnknownIcon,
			Message:   "未知 Icon Directive: icon:tail",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown icon diagnostics = %#v, want %#v", got, want)
	}
}

func TestParseMalformedSplitSyntaxFallsBackWithoutLosingContent(t *testing.T) {
	src := "::: middle\n原文\n:::\n\n::: left trailing\n继续原文\n"
	analysis := Analyze(src)

	wantBlocks := []Block{
		Paragraph{Children: []Inline{Text{Value: "::: middle\n原文"}}},
		Paragraph{Children: []Inline{Text{Value: "::: left trailing\n继续原文"}}},
	}
	if !reflect.DeepEqual(analysis.Document.Blocks, wantBlocks) {
		t.Fatalf("blocks = %#v, want %#v", analysis.Document.Blocks, wantBlocks)
	}
	wantDiagnostics := []Diagnostic{{
		Line:      3,
		Column:    1,
		EndColumn: 4,
		Severity:  SeverityError,
		Code:      CodeUnexpectedSplitClose,
		Message:   "意外的 ::: 结束标记",
	}}
	if !reflect.DeepEqual(analysis.Diagnostics, wantDiagnostics) {
		t.Fatalf("diagnostics = %#v, want %#v", analysis.Diagnostics, wantDiagnostics)
	}
}
