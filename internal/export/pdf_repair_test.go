package export

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/egoist/mygo"
)

func TestRepairPrintedPDFFixesType3RadicalMappingsWithoutNormalizingOtherText(t *testing.T) {
	pdf := syntheticType3PDF(`/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
1 begincodespacerange
<00> <FF>
endcodespacerange
4 beginbfchar
<01> <2F3E>
<02> <2ED4>
<03> <2F63>
<04> <FF21>
endbfchar
endcmap
CMapName currentdict /CMap defineresource pop
end
end`, "Type3")

	repaired, err := repairPrintedPDF(pdf, "户门生Ａ①")
	if err != nil {
		t.Fatal(err)
	}
	cmap := latestPDFObjectStream(t, repaired, 5)
	for _, want := range []string{"<6237>", "<95E8>", "<751F>", "<FF21>"} {
		if !bytes.Contains(cmap, []byte(want)) {
			t.Errorf("repaired ToUnicode does not contain %s: %s", want, cmap)
		}
	}
	for _, unwanted := range []string{"<2F3E>", "<2ED4>", "<2F63>"} {
		if bytes.Contains(cmap, []byte(unwanted)) {
			t.Errorf("repaired ToUnicode still contains %s", unwanted)
		}
	}
	font := latestPDFObject(t, repaired, 4)
	for _, want := range []string{"/Ascent 520", "/Descent -20"} {
		if !bytes.Contains(font, []byte(want)) {
			t.Errorf("repaired Type3 font does not contain %s: %s", want, font)
		}
	}
	if !bytes.Contains(font, []byte("/FontBBox [100 -880 900 120]")) {
		t.Error("repair changed the Type3 FontBBox")
	}
	if !bytes.Contains(repaired, []byte("q 1 0 0 1 0 0 cm BT /F1 10 Tf (正文) Tj ET Q")) {
		t.Error("repair changed the page content drawing stream")
	}
}

func TestRepairType3CMapRewritesBfcharDestinationsForVisibleOrdinaryCharacters(t *testing.T) {
	cmap := []byte("beginbfchar\n<01> <2F3E>\n<02> <FF21>\n<03> <2EDA>\nendbfchar\n")
	got, changed := repairType3CMap(cmap, "户Ａ页")
	if !changed {
		t.Fatal("repairType3CMap did not report a change")
	}
	for _, want := range []string{"<01> <6237>", "<02> <FF21>", "<03> <9875>"} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("repaired CMap does not contain %q: %s", want, got)
		}
	}
}

func TestRepairType3CMapFixesAdditionalOrdinaryCJKCompatibilityRadicals(t *testing.T) {
	cmap := []byte("beginbfchar\n<01> <2F70>\n<02> <2ED3>\n<03> <2F06>\n<04> <FF21>\nendbfchar\n")
	got, changed := repairType3CMap(cmap, "示例增长第二页 ①Ａ")
	if !changed {
		t.Fatal("repairType3CMap did not report a change")
	}
	for _, want := range []string{"<01> <793A>", "<02> <957F>", "<03> <4E8C>", "<04> <FF21>"} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("repaired CMap does not contain %q: %s", want, got)
		}
	}
	for _, unwanted := range []string{"<2F70>", "<2ED3>", "<2F06>"} {
		if bytes.Contains(got, []byte(unwanted)) {
			t.Errorf("repaired CMap still contains %s", unwanted)
		}
	}
}

func TestRepairType3CMapPreservesAdditionalVisibleCompatibilityRadicals(t *testing.T) {
	cmap := []byte("beginbfchar\n<01> <2F70>\n<02> <2ED3>\n<03> <2F06>\nendbfchar\n")
	for _, tc := range []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "legal radicals",
			source: "⽰⻓⼆",
			want:   []string{"<2F70>", "<2ED3>", "<2F06>"},
		},
		{
			name:   "mixed ordinary and radical",
			source: "⽰示 ⻓长 ⼆二",
			want:   []string{"<2F70>", "<2ED3>", "<2F06>"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := repairType3CMap(cmap, tc.source)
			if changed {
				t.Error("repairType3CMap changed an ambiguous or legal radical mapping")
			}
			for _, want := range tc.want {
				if !bytes.Contains(got, []byte(want)) {
					t.Errorf("repaired CMap does not contain %q: %s", want, got)
				}
			}
		})
	}
}

func TestRepairPrintedPDFFixesAdditionalCompatibilityRadicalsAcrossPages(t *testing.T) {
	pdf := syntheticTwoPageType3PDF(`3 beginbfchar
<01> <2F70>
<02> <2ED3>
<03> <2F06>
endbfchar`, `2 beginbfchar
<04> <2F70>
<05> <2F06>
endbfchar`)

	repaired, err := repairPrintedPDF(pdf, "示例增长第二页 ①Ａ")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		object int
		want   []string
	}{
		{object: 9, want: []string{"<01> <793A>", "<02> <957F>", "<03> <4E8C>"}},
		{object: 10, want: []string{"<04> <793A>", "<05> <4E8C>"}},
	} {
		cmap := latestPDFObjectStream(t, repaired, tc.object)
		for _, want := range tc.want {
			if !bytes.Contains(cmap, []byte(want)) {
				t.Errorf("repaired page CMap object %d does not contain %q: %s", tc.object, want, cmap)
			}
		}
	}
	for _, want := range []string{
		"q 1 0 0 1 0 0 cm BT /F1 10 Tf (第一页) Tj ET Q",
		"q 1 0 0 1 0 0 cm BT /F2 10 Tf (第二页) Tj ET Q",
	} {
		if !bytes.Contains(repaired, []byte(want)) {
			t.Errorf("repair changed a page drawing stream, missing %q", want)
		}
	}
}

func TestRepairPrintedPDFPreservesVisibleCompatibilityRadicalsAcrossPages(t *testing.T) {
	pdf := syntheticTwoPageType3PDF(`3 beginbfchar
<01> <2F70>
<02> <2ED3>
<03> <2F06>
endbfchar`, `2 beginbfchar
<04> <2F70>
<05> <2F06>
endbfchar`)

	repaired, err := repairPrintedPDF(pdf, "⽰⻓⼆")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		object int
		want   []string
	}{
		{object: 9, want: []string{"<2F70>", "<2ED3>", "<2F06>"}},
		{object: 10, want: []string{"<2F70>", "<2F06>"}},
	} {
		cmap := latestPDFObjectStream(t, repaired, tc.object)
		for _, want := range tc.want {
			if !bytes.Contains(cmap, []byte(want)) {
				t.Errorf("repaired page CMap object %d lost legal radical %s: %s", tc.object, want, cmap)
			}
		}
		for _, unwanted := range []string{"<793A>", "<957F>", "<4E8C>"} {
			if bytes.Contains(cmap, []byte(unwanted)) {
				t.Errorf("repaired page CMap object %d introduced ordinary mapping %s", tc.object, unwanted)
			}
		}
	}
}

func TestRepairType3CMapPreservesVisibleCompatibilityRadicals(t *testing.T) {
	cmap := []byte("beginbfchar\n<01> <2F3E>\n<02> <2F63>\nendbfchar\n")
	for _, tc := range []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "legal radical",
			source: "⼾",
			want:   []string{"<2F3E>", "<2F63>"},
		},
		{
			name:   "mixed ordinary and radical",
			source: "户⼾ ⽣生",
			want:   []string{"<2F3E>", "<2F63>"},
		},
		{
			name:   "unrelated radical does not block another mapping",
			source: "⼤户",
			want:   []string{"<6237>", "<2F63>"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := repairType3CMap(cmap, tc.source)
			for _, want := range tc.want {
				if !bytes.Contains(got, []byte(want)) {
					t.Errorf("repaired CMap does not contain %q: %s", want, got)
				}
			}
		})
	}
}

func TestRepairPrintedPDFLeavesNonType3FontMappingsAlone(t *testing.T) {
	pdf := syntheticType3PDF(`2 beginbfchar
<01> <2F3E>
<02> <2F63>
endbfchar`, "Type0")

	repaired, err := repairPrintedPDF(pdf, "户生")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(repaired, pdf) {
		t.Error("repair changed a PDF without Type3 fonts")
	}
}

func TestRepairPrintedPDFAddsMetricsToIndirectFontDescriptors(t *testing.T) {
	pdf := syntheticIndirectDescriptorPDF()
	repaired, err := repairPrintedPDF(pdf, "文")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := latestPDFObject(t, repaired, 8)
	for _, want := range []string{"/Ascent 520", "/Descent -20"} {
		if !bytes.Contains(descriptor, []byte(want)) {
			t.Errorf("repaired indirect FontDescriptor does not contain %s: %s", want, descriptor)
		}
	}
}

func TestRenderPDFUsesVisibleDOMTextForSourceAwareRepair(t *testing.T) {
	pdf := syntheticType3PDF(`1 beginbfchar
<01> <2F3E>
endbfchar`, "Type3")
	page := &visibleTextPage{pdf: pdf, text: "户"}
	repaired, err := RenderPDF(
		context.Background(),
		fixedExportPageRunner{page: page},
		"<html><main id=\"sheet-root\">户</main></html>",
	)
	if err != nil {
		t.Fatal(err)
	}
	cmap := latestPDFObjectStream(t, repaired, 5)
	if !bytes.Contains(cmap, []byte("<6237>")) || bytes.Contains(cmap, []byte("<2F3E>")) {
		t.Fatalf("repaired CMap did not use the DOM source text: %s", cmap)
	}
}

func TestRepairPrintedPDFPreservesExistingLinkAnnotations(t *testing.T) {
	pdf := syntheticType3PDF(`1 beginbfchar
<01> <2F3E>
endbfchar`, "Type3")
	pdf = bytes.Replace(pdf,
		[]byte("/Contents 6 0 R >>"),
		[]byte("/Contents 6 0 R /Annots [8 0 R] >>"), 1)
	pdf = bytes.Replace(pdf,
		[]byte("xref\n0 8"),
		[]byte("8 0 obj\n<< /Type /Annot /Subtype /Link /Rect [0 0 20 20] /Border [0 0 0] /A << /S /URI /URI (https://example.com/preserved) >> >>\nendobj\nxref\n0 9"), 1)
	pdf = bytes.Replace(pdf, []byte("/Size 8"), []byte("/Size 9"), 1)

	repaired, err := repairPrintedPDF(pdf, "户")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(repaired, []byte("/Subtype /Link")) ||
		!bytes.Contains(repaired, []byte("https://example.com/preserved")) {
		t.Fatal("repair removed or changed an existing link annotation")
	}
	if !bytes.Contains(repaired, []byte("q 1 0 0 1 0 0 cm BT /F1 10 Tf (正文) Tj ET Q")) {
		t.Fatal("repair changed the page drawing stream")
	}
}

func TestType3MetricsUseFontEvidenceForLatinAndCJK(t *testing.T) {
	latin := []byte(`<< /FontName /AAAAAA+Nunito-Regular >>`)
	latinRepaired, changed := addPDFFontMetrics(latin)
	if !changed {
		t.Fatal("addPDFFontMetrics skipped the Latin descriptor")
	}
	for _, want := range []string{"/Ascent 120", "/Descent -880"} {
		if !bytes.Contains(latinRepaired, []byte(want)) {
			t.Errorf("Latin descriptor does not contain %s: %s", want, latinRepaired)
		}
	}

	cjk := []byte(`<< /FontName /AAAAAA+NotoSansCJKsc-Regular >>`)
	cjkRepaired, changed := addPDFFontMetrics(cjk)
	if !changed {
		t.Fatal("addPDFFontMetrics skipped the CJK descriptor")
	}
	for _, want := range []string{"/Ascent 520", "/Descent -20"} {
		if !bytes.Contains(cjkRepaired, []byte(want)) {
			t.Errorf("CJK descriptor does not contain %s: %s", want, cjkRepaired)
		}
	}
}

type visibleTextPage struct {
	pdf  []byte
	text string
}

func (p *visibleTextPage) LoadHTML(string, string) {}

func (p *visibleTextPage) EvalContext(_ context.Context, script string) (any, error) {
	switch {
	case bytes.Contains([]byte(script), []byte("document.fonts.ready")):
		return true, nil
	case bytes.Contains([]byte(script), []byte(`querySelectorAll("a[href]")`)):
		return map[string]any{
			"text":  p.text,
			"lines": []any{},
			"links": []any{},
		}, nil
	default:
		return true, nil
	}
}

func (p *visibleTextPage) PrintToPDF(mygo.PDFOptions) ([]byte, error) {
	return p.pdf, nil
}

type fixedExportPageRunner struct {
	page Page
}

func (r fixedExportPageRunner) WithPage(
	ctx context.Context,
	_ string,
	fn func(context.Context, Page) error,
) error {
	return fn(ctx, r.page)
}

func syntheticType3PDF(cmapEntries, subtype string) []byte {
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write([]byte(cmapEntries))
	_ = zw.Close()

	content := "q 1 0 0 1 0 0 cm BT /F1 10 Tf (正文) Tj ET Q"
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /Contents 6 0 R >>`,
		fmt.Sprintf(`<< /Type /Font /Subtype /%s /FontBBox [100 -880 900 120] /FontDescriptor << /Type /FontDescriptor /FontName /AAAAAA+NotoSansCJKsc-Regular >> /CharProcs << /g0 7 0 R /g699F 7 0 R >> /Encoding << /Type /Encoding /Differences [0 /g0] >> /FirstChar 0 /LastChar 0 /Widths [500] /ToUnicode 5 0 R >>`, subtype),
		fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%sendstream", compressed.Len(), compressed.String()),
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
		"<< /Length 1 >>\nstream\n q\nendstream",
	}
	for index, object := range objects {
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	pdf.WriteString("xref\n0 8\n0000000000 65535 f \ntrailer\n<< /Size 8 /Root 1 0 R >>\nstartxref\n0\n%%EOF\n")
	return pdf.Bytes()
}

func syntheticIndirectDescriptorPDF() []byte {
	cmap := `1 beginbfchar
<01> <2F42>
endbfchar`
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write([]byte(cmap))
	_ = zw.Close()

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /Contents 6 0 R >>`,
		`<< /Type /Font /Subtype /Type3 /FontBBox [100 -880 900 120] /FontDescriptor 8 0 R /CharProcs << /g0 7 0 R /g6587 7 0 R >> /Encoding << /Type /Encoding /Differences [0 /g0] >> /FirstChar 0 /LastChar 0 /Widths [500] /ToUnicode 5 0 R >>`,
		fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%sendstream", compressed.Len(), compressed.String()),
		"<< /Length 1 >>\nstream\n q\nendstream",
		"<< /Length 1 >>\nstream\n q\nendstream",
		`<< /Type /FontDescriptor /FontFamily (Noto Sans CJK SC) /FontWeight 400 /FontName /AAAAAA+NotoSansCJKsc-Regular /CapHeight 733 /StemV 109 /XHeight 543 /Flags 4 >>`,
	}
	for index, object := range objects {
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	pdf.WriteString("xref\n0 9\n0000000000 65535 f \ntrailer\n<< /Size 9 /Root 1 0 R >>\nstartxref\n0\n%%EOF\n")
	return pdf.Bytes()
}

func syntheticTwoPageType3PDF(firstCMap, secondCMap string) []byte {
	compress := func(text string) (*bytes.Buffer, int) {
		var out bytes.Buffer
		writer := zlib.NewWriter(&out)
		_, _ = writer.Write([]byte(text))
		_ = writer.Close()
		return &out, out.Len()
	}
	firstCompressed, firstLength := compress(firstCMap)
	secondCompressed, secondLength := compress(secondCMap)

	firstContent := "q 1 0 0 1 0 0 cm BT /F1 10 Tf (第一页) Tj ET Q"
	secondContent := "q 1 0 0 1 0 0 cm BT /F2 10 Tf (第二页) Tj ET Q"
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>`,
		`<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 5 0 R >> >> /Contents 7 0 R >>`,
		`<< /Type /Page /Parent 2 0 R /Resources << /Font << /F2 6 0 R >> >> /Contents 8 0 R >>`,
		`<< /Type /Font /Subtype /Type3 /FontBBox [100 -880 900 120] /FontDescriptor << /Type /FontDescriptor /FontName /AAAAAA+NotoSansCJKsc-Regular >> /CharProcs << /g0 7 0 R /g793A 7 0 R >> /Encoding << /Type /Encoding /Differences [0 /g0] >> /FirstChar 0 /LastChar 0 /Widths [500] /ToUnicode 9 0 R >>`,
		`<< /Type /Font /Subtype /Type3 /FontBBox [100 -880 900 120] /FontDescriptor << /Type /FontDescriptor /FontName /AAAAAA+NotoSansCJKsc-Regular >> /CharProcs << /g0 8 0 R /g4E8C 8 0 R >> /Encoding << /Type /Encoding /Differences [0 /g0] >> /FirstChar 0 /LastChar 0 /Widths [500] /ToUnicode 10 0 R >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(firstContent), firstContent),
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(secondContent), secondContent),
		fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%sendstream", firstLength, firstCompressed.String()),
		fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%sendstream", secondLength, secondCompressed.String()),
	}
	for index, object := range objects {
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	pdf.WriteString("xref\n0 11\n0000000000 65535 f \ntrailer\n<< /Size 11 /Root 1 0 R >>\nstartxref\n0\n%%EOF\n")
	return pdf.Bytes()
}

func latestPDFObject(t *testing.T, pdf []byte, number int) []byte {
	t.Helper()
	expression := regexp.MustCompile(fmt.Sprintf(`(?ms)^%d 0 obj\s*(.*?)\s*endobj`, number))
	matches := expression.FindAllSubmatch(pdf, -1)
	if len(matches) == 0 {
		t.Fatalf("PDF has no object %d", number)
	}
	return matches[len(matches)-1][1]
}

func latestPDFObjectStream(t *testing.T, pdf []byte, number int) []byte {
	t.Helper()
	body := latestPDFObject(t, pdf, number)
	start := bytes.Index(body, []byte("stream"))
	end := bytes.LastIndex(body, []byte("endstream"))
	if start < 0 || end < start {
		t.Fatalf("PDF object %d has no stream", number)
	}
	data := bytes.TrimLeft(body[start+len("stream"):end], "\r\n")
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decompress object %d: %v", number, err)
	}
	defer reader.Close()
	var out bytes.Buffer
	if _, err := out.ReadFrom(reader); err != nil {
		t.Fatalf("read object %d stream: %v", number, err)
	}
	return out.Bytes()
}
