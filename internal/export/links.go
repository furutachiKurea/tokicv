package export

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type linkAnnotation struct {
	Page int
	Rect [4]float64
	URI  string
}

type pdfObject struct {
	Number int
	Gen    int
	Body   []byte
}

var (
	pdfObjectRE    = regexp.MustCompile(`(?m)^(\d+) (\d+) obj\b`)
	pdfPageRE      = regexp.MustCompile(`(?s)/Type\s*/Page(?:\s|/|>)`)
	pdfLinkRE      = regexp.MustCompile(`(?s)/Subtype\s*/Link\b`)
	pdfRefRE       = regexp.MustCompile(`\d+\s+\d+\s+R`)
	pdfAnnotsRE    = regexp.MustCompile(`(?s)/Annots\s*(\[[^\]]*\]|\d+\s+\d+\s+R)`)
	pdfStartXrefRE = regexp.MustCompile(`(?m)startxref\s+(\d+)`)
)

func addLinkAnnotations(pdf []byte, links []linkAnnotation) ([]byte, error) {
	if len(links) == 0 {
		return pdf, nil
	}
	if pdfLinkRE.Match(pdf) {
		return pdf, nil
	}

	pages := findPDFPages(pdf)
	if len(pages) == 0 {
		return nil, errors.New("打印结果中没有 PDF 页面")
	}
	byPage := make(map[int][]linkAnnotation)
	for _, link := range links {
		if link.Page >= 0 && link.Page < len(pages) {
			byPage[link.Page] = append(byPage[link.Page], link)
		}
	}
	if len(byPage) == 0 {
		return pdf, nil
	}

	maxObject := maxPDFObject(pdf)
	nextObject := maxObject + 1

	var updates []pdfObjectUpdate
	for pageIndex, page := range pages {
		linksOnPage := byPage[pageIndex]
		if len(linksOnPage) == 0 {
			continue
		}

		refs := existingAnnotationRefs(pdf, page.Body)
		for _, link := range linksOnPage {
			ref := fmt.Sprintf("%d 0 R", nextObject)
			refs = append(refs, ref)
			updates = append(updates, pdfObjectUpdate{
				number: nextObject,
				body: []byte(fmt.Sprintf(`<< /Type /Annot /Subtype /Link /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] /A << /S /URI /URI %s >> >>`,
					link.Rect[0], link.Rect[1], link.Rect[2], link.Rect[3], pdfLiteralString(link.URI)),
				),
			})
			nextObject++
		}
		updates = append(updates, pdfObjectUpdate{
			number: page.Number,
			gen:    page.Gen,
			body:   []byte(addAnnotsToPage(page.Body, refs)),
		})
	}
	if len(updates) == 0 {
		return pdf, nil
	}
	return appendPDFUpdates(pdf, updates)
}

func findPDFPages(pdf []byte) []pdfObject {
	matches := pdfObjectRE.FindAllSubmatchIndex(pdf, -1)
	var pages []pdfObject
	for _, match := range matches {
		number, _ := strconv.Atoi(string(pdf[match[2]:match[3]]))
		gen, _ := strconv.Atoi(string(pdf[match[4]:match[5]]))
		bodyStart := match[1]
		end := bytes.Index(pdf[bodyStart:], []byte("endobj"))
		if end < 0 {
			continue
		}
		body := bytes.TrimSpace(pdf[bodyStart : bodyStart+end])
		if !pdfPageRE.Match(body) {
			continue
		}
		pages = append(pages, pdfObject{Number: number, Gen: gen, Body: body})
	}
	return pages
}

func existingAnnotationRefs(pdf, page []byte) []string {
	match := pdfAnnotsRE.FindSubmatch(page)
	if match == nil {
		return nil
	}
	value := match[1]
	if value[0] != '[' {
		object := findPDFObject(pdf, string(value))
		if object == nil {
			return nil
		}
		nested := pdfAnnotsRE.FindSubmatch(object)
		if nested == nil || nested[1][0] != '[' {
			return nil
		}
		value = nested[1]
	}
	var refs []string
	for _, ref := range pdfRefRE.FindAll(value, -1) {
		refs = append(refs, string(ref))
	}
	return refs
}

func findPDFObject(pdf []byte, ref string) []byte {
	match := regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+R\s*$`).FindStringSubmatch(ref)
	if match == nil {
		return nil
	}
	header := regexp.MustCompile(fmt.Sprintf(`(?m)^%s %s obj\b`, match[1], match[2]))
	matches := header.FindAllIndex(pdf, -1)
	if len(matches) == 0 {
		return nil
	}
	start := matches[len(matches)-1][1]
	end := bytes.Index(pdf[start:], []byte("endobj"))
	if end < 0 {
		return nil
	}
	return bytes.TrimSpace(pdf[start : start+end])
}

func addAnnotsToPage(page []byte, refs []string) string {
	annots := "/Annots [" + strings.Join(refs, " ") + "]"
	if match := pdfAnnotsRE.FindIndex(page); match != nil {
		return string(page[:match[0]]) + annots + string(page[match[1]:])
	}
	end := bytes.LastIndex(page, []byte(">>"))
	if end < 0 {
		return string(append(page, []byte(" "+annots)...))
	}
	var out bytes.Buffer
	out.Write(page[:end])
	out.WriteString(" ")
	out.WriteString(annots)
	out.Write(page[end:])
	return out.String()
}

func lastPDFReference(pdf []byte, expression string) string {
	matches := regexp.MustCompile(expression).FindAllSubmatch(pdf, -1)
	if len(matches) == 0 {
		return ""
	}
	return string(matches[len(matches)-1][1])
}

func lastPDFInteger(pdf []byte, expression *regexp.Regexp) (int, bool) {
	matches := expression.FindAllSubmatch(pdf, -1)
	if len(matches) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(string(matches[len(matches)-1][1]))
	return n, err == nil
}

func maxPDFObject(pdf []byte) int {
	max := 0
	for _, match := range pdfObjectRE.FindAllSubmatch(pdf, -1) {
		n, _ := strconv.Atoi(string(match[1]))
		if n > max {
			max = n
		}
	}
	return max
}

func pdfLiteralString(value string) string {
	var out strings.Builder
	out.WriteByte('(')
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c == '\\' || c == '(' || c == ')':
			out.WriteByte('\\')
			out.WriteByte(c)
		case c < 0x20 || c > 0x7e:
			fmt.Fprintf(&out, `\%03o`, c)
		default:
			out.WriteByte(c)
		}
	}
	out.WriteByte(')')
	return out.String()
}
