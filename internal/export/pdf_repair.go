package export

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
)

// The embedded Type3 subsets retain glyph identity while Skia writes some CJK
// compatibility-radical ToUnicode destinations. These replacements follow the
// source glyph rather than applying NFKC to all extracted text.
var type3ToUnicodeFixes = map[rune]rune{
	'\u2F00': '一',
	'\u2F06': '二',
	'\u2F08': '人',
	'\u2F0A': '入',
	'\u2F12': '力',
	'\u2F1D': '口',
	'\u2F29': '小',
	'\u2F24': '大',
	'\u2F26': '子',
	'\u2F2F': '工',
	'\u2F3C': '心',
	'\u2F3E': '户',
	'\u2F3F': '手',
	'\u2F40': '支',
	'\u2F42': '文',
	'\u2F45': '方',
	'\u2F46': '无',
	'\u2F47': '日',
	'\u2F4C': '止',
	'\u2F54': '水',
	'\u2F63': '生',
	'\u2F64': '用',
	'\u2F6C': '目',
	'\u2F70': '示',
	'\u2F74': '立',
	'\u2F79': '网',
	'\u2F83': '自',
	'\u2F84': '至',
	'\u2F8A': '色',
	'\u2F8F': '行',
	'\u2F94': '言',
	'\u2FA6': '金',
	'\u2FAF': '面',
	'\u2FAE': '非',
	'\u2FB8': '首',
	'\u2FBC': '高',
	'\u2EC5': '见',
	'\u2EC6': '角',
	'\u2ED3': '长',
	'\u2ED4': '门',
	'\u2EDA': '页',
	'\u2EDB': '风',
	'\u2EEC': '齐',
}

var (
	pdfType3FontRE         = regexp.MustCompile(`(?s)/Subtype\s*/Type3\b`)
	pdfToUnicodeRE         = regexp.MustCompile(`/ToUnicode\s+(\d+)\s+(\d+)\s+R`)
	pdfFontDescriptorRefRE = regexp.MustCompile(`/FontDescriptor\s+(\d+)\s+(\d+)\s+R`)
	pdfFontDescriptorRE    = regexp.MustCompile(`(?s)/FontDescriptor\s*<<(.*?)>>`)
	pdfFlateRE             = regexp.MustCompile(`/Filter\s*(?:\[[^\]]*\]|/\w+)`)
	pdfBfcharBlockRE       = regexp.MustCompile(`(?s)beginbfchar(.*?)endbfchar`)
	pdfBfcharPairRE        = regexp.MustCompile(`<[0-9A-Fa-f]+>\s*(<[0-9A-Fa-f]+>)`)
)

const (
	// Poppler derives Type3 word boxes from twice the font size. These
	// thousand-unit metrics keep body boxes inside the 12.75pt line pitch.
	type3CJKAscent    = "520"
	type3CJKDescent   = "-20"
	type3LatinAscent  = "120"
	type3LatinDescent = "-880"
)

// repairPrintedPDF fixes the cosmetic text layer Skia writes for Type3 CJK
// subsets: compatibility-radical ToUnicode destinations and missing vertical
// font metrics. visibleText protects source characters whose glyph shares a
// subset code with the ordinary Repair target. It appends incremental objects
// and leaves page drawing data untouched.
func repairPrintedPDF(pdf []byte, visibleText string) ([]byte, error) {
	objects := pdfObjectsByNumber(pdf)
	toUnicode := make(map[int]bool)
	updates := make(map[int][]byte)
	descriptors := make(map[int]bool)

	for number, object := range objects {
		if !pdfType3FontRE.Match(object.Body) {
			continue
		}
		if updated, ok := addType3FontMetrics(object.Body); ok {
			updates[number] = updated
		}
		if match := pdfToUnicodeRE.FindSubmatch(object.Body); match != nil {
			ref, _ := strconv.Atoi(string(match[1]))
			toUnicode[ref] = true
		}
		descriptor := pdfFontDescriptorRefRE.FindSubmatch(object.Body)
		if descriptor == nil {
			continue
		}
		ref, _ := strconv.Atoi(string(descriptor[1]))
		descriptors[ref] = true
	}

	for number := range toUnicode {
		object, ok := objects[number]
		if !ok {
			continue
		}
		data, compressed, err := decodePDFStream(object.Body)
		if err != nil {
			return nil, fmt.Errorf("读取 Type3 ToUnicode %d: %w", number, err)
		}
		repaired, changed := repairType3CMap(data, visibleText)
		if !changed {
			continue
		}
		body, err := encodePDFStream(repaired, compressed)
		if err != nil {
			return nil, fmt.Errorf("编码 Type3 ToUnicode %d: %w", number, err)
		}
		updates[number] = body
	}
	for number := range descriptors {
		object, ok := objects[number]
		if !ok {
			continue
		}
		updated, changed := addPDFFontMetrics(object.Body)
		if changed {
			updates[number] = updated
		}
	}
	if len(updates) == 0 {
		return pdf, nil
	}

	changes := make([]pdfObjectUpdate, 0, len(updates))
	for number, body := range updates {
		object := objects[number]
		changes = append(changes, pdfObjectUpdate{
			number: number,
			gen:    object.Gen,
			body:   body,
		})
	}
	return appendPDFUpdates(pdf, changes)
}

func addPDFFontMetrics(descriptor []byte) ([]byte, bool) {
	if bytes.Contains(descriptor, []byte("/Ascent")) || bytes.Contains(descriptor, []byte("/Descent")) {
		return descriptor, false
	}
	end := bytes.LastIndex(descriptor, []byte(">>"))
	if end < 0 {
		return descriptor, false
	}
	var out bytes.Buffer
	out.Grow(len(descriptor) + 48)
	out.Write(descriptor[:end])
	ascent, descent := type3Metrics(descriptor)
	fmt.Fprintf(&out, " /Ascent %s /Descent %s ", ascent, descent)
	out.Write(descriptor[end:])
	return out.Bytes(), true
}

func addType3FontMetrics(font []byte) ([]byte, bool) {
	descriptor := pdfFontDescriptorRE.FindSubmatchIndex(font)
	if descriptor == nil {
		return nil, false
	}
	content := font[descriptor[2]:descriptor[3]]
	if bytes.Contains(content, []byte("/Ascent")) || bytes.Contains(content, []byte("/Descent")) {
		return nil, false
	}

	var out bytes.Buffer
	out.Grow(len(font) + 48)
	out.Write(font[:descriptor[1]-2])
	ascent, descent := type3Metrics(content)
	fmt.Fprintf(&out, " /Ascent %s /Descent %s ", ascent, descent)
	out.Write(font[descriptor[1]-2:])
	return out.Bytes(), true
}

func type3Metrics(descriptor []byte) (string, string) {
	if bytes.Contains(descriptor, []byte("Nunito")) {
		return type3LatinAscent, type3LatinDescent
	}
	return type3CJKAscent, type3CJKDescent
}

func repairType3CMap(data []byte, visibleText string) ([]byte, bool) {
	source := make(map[rune]bool)
	for _, r := range visibleText {
		source[r] = true
	}
	changed := false
	var out bytes.Buffer
	last := 0
	for _, block := range pdfBfcharBlockRE.FindAllSubmatchIndex(data, -1) {
		out.Write(data[last:block[0]])
		out.Write(data[block[0]:block[2]])
		out.Write(repairBfcharPairs(data[block[2]:block[3]], source, &changed))
		out.Write(data[block[3]:block[1]])
		last = block[1]
	}
	if !changed {
		return data, false
	}
	out.Write(data[last:])
	return out.Bytes(), true
}

func repairBfcharPairs(section []byte, source map[rune]bool, changed *bool) []byte {
	var out bytes.Buffer
	last := 0
	for _, pair := range pdfBfcharPairRE.FindAllSubmatchIndex(section, -1) {
		out.Write(section[last:pair[2]])
		token := section[pair[2]:pair[3]]
		if replacement, ok := repairRadicalToken(token, source); ok {
			out.WriteString(replacement)
			*changed = true
		} else {
			out.Write(token)
		}
		last = pair[3]
	}
	out.Write(section[last:])
	return out.Bytes()
}

func repairRadicalToken(token []byte, source map[rune]bool) (string, bool) {
	if len(token) != 6 || token[0] != '<' || token[5] != '>' {
		return "", false
	}
	value, err := strconv.ParseUint(string(token[1:5]), 16, 16)
	if err != nil {
		return "", false
	}
	replacement, ok := type3ToUnicodeFixes[rune(value)]
	if !ok {
		return "", false
	}
	if !source[replacement] || source[rune(value)] {
		return "", false
	}
	return fmt.Sprintf("<%04X>", replacement), true
}

func decodePDFStream(body []byte) ([]byte, bool, error) {
	start := bytes.Index(body, []byte("stream"))
	end := bytes.LastIndex(body, []byte("endstream"))
	if start < 0 || end < start {
		return nil, false, errors.New("PDF 流缺少 stream 边界")
	}
	data := bytes.TrimLeft(body[start+len("stream"):end], "\r\n")
	filter := pdfFlateRE.Find(body[:start])
	compressed := bytes.Contains(filter, []byte("/FlateDecode"))
	if !compressed {
		return data, false, nil
	}
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, false, err
	}
	defer reader.Close()
	out, err := io.ReadAll(reader)
	return out, true, err
}

func encodePDFStream(data []byte, compressed bool) ([]byte, error) {
	if !compressed {
		return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(data), data)), nil
	}
	var out bytes.Buffer
	writer := zlib.NewWriter(&out)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%sendstream", out.Len(), out.Bytes())), nil
}

func pdfObjectsByNumber(pdf []byte) map[int]pdfObject {
	objects := make(map[int]pdfObject)
	for _, match := range pdfObjectRE.FindAllSubmatchIndex(pdf, -1) {
		number, err := strconv.Atoi(string(pdf[match[2]:match[3]]))
		if err != nil {
			continue
		}
		gen, err := strconv.Atoi(string(pdf[match[4]:match[5]]))
		if err != nil {
			continue
		}
		start := match[1]
		end := bytes.Index(pdf[start:], []byte("endobj"))
		if end < 0 {
			continue
		}
		objects[number] = pdfObject{
			Number: number,
			Gen:    gen,
			Body:   bytes.TrimSpace(pdf[start : start+end]),
		}
	}
	return objects
}

type pdfObjectUpdate struct {
	number int
	gen    int
	body   []byte
}

func appendPDFUpdates(pdf []byte, updates []pdfObjectUpdate) ([]byte, error) {
	if len(updates) == 0 {
		return pdf, nil
	}
	root := lastPDFReference(pdf, `/Root\s+(\d+\s+\d+\s+R)`)
	previousXref, ok := lastPDFInteger(pdf, pdfStartXrefRE)
	if root == "" || !ok || previousXref < 0 || previousXref >= len(pdf) {
		return nil, errors.New("打印结果缺少可更新的 PDF 交叉引用表")
	}
	info := lastPDFReference(pdf, `/Info\s+(\d+\s+\d+\s+R)`)
	size := maxPDFObject(pdf) + 1
	sort.Slice(updates, func(i, j int) bool {
		return updates[i].number < updates[j].number
	})
	for _, update := range updates {
		if update.number >= size {
			size = update.number + 1
		}
	}

	var out bytes.Buffer
	out.Grow(len(pdf) + len(updates)*256)
	out.Write(pdf)
	if out.Len() > 0 && out.Bytes()[out.Len()-1] != '\n' {
		out.WriteByte('\n')
	}
	offsets := make(map[int]int, len(updates))
	for _, update := range updates {
		offsets[update.number] = out.Len()
		fmt.Fprintf(&out, "%d %d obj\n", update.number, update.gen)
		out.Write(update.body)
		out.WriteString("\nendobj\n")
	}

	xrefOffset := out.Len()
	out.WriteString("xref\n")
	for _, update := range updates {
		fmt.Fprintf(&out, "%d 1\n%010d %05d n \n", update.number, offsets[update.number], update.gen)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %s /Prev %d", size, root, previousXref)
	if info != "" {
		fmt.Fprintf(&out, " /Info %s", info)
	}
	out.WriteString(" >>\n")
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return out.Bytes(), nil
}
