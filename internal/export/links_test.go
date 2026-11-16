package export

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestAddLinkAnnotations(t *testing.T) {
	original := testPDF()
	links := []linkAnnotation{{
		Page: 0,
		Rect: [4]float64{50, 700, 300, 720},
		URI:  "https://example.com/docs",
	}}

	got, err := addLinkAnnotations(original, links)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, original) {
		t.Fatal("the update does not preserve the printed PDF")
	}

	page := testObject(t, got, 1)
	annots := regexp.MustCompile(`/Annots\s*\[\s*(\d+) 0 R\s*\]`).FindSubmatch(page)
	if annots == nil {
		t.Fatalf("page has no link annotation: %s", page)
	}
	annotation := testObject(t, got, mustAtoi(t, annots[1]))
	for _, want := range []string{
		`/Type /Annot`,
		`/Subtype /Link`,
		`/Rect [50.00 700.00 300.00 720.00]`,
		`/S /URI`,
		`/URI (https://example.com/docs)`,
	} {
		if !bytes.Contains(annotation, []byte(want)) {
			t.Errorf("annotation does not contain %q: %s", want, annotation)
		}
	}
	testPatchXref(t, got)
}

func TestAddLinkAnnotationsLeavesPDFsWithoutLinksUnchanged(t *testing.T) {
	original := testPDF()
	got, err := addLinkAnnotations(original, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("a PDF without links changed")
	}
}

func testPDF() []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, 4)
	objects := []string{
		1: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>",
		2: "<< /Type /Pages /Count 1 /Kids [1 0 R] >>",
		3: "<< /Type /Catalog /Pages 2 0 R >>",
	}
	for n := 1; n < len(objects); n++ {
		offsets[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, objects[n])
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects))
	for n := 1; n < len(objects); n++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[n])
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 3 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects), xref)
	return b.Bytes()
}

func testObject(t *testing.T, pdf []byte, number int) []byte {
	t.Helper()
	header := regexp.MustCompile(fmt.Sprintf(`(?m)^%d 0 obj\s*`, number))
	matches := header.FindAllIndex(pdf, -1)
	if len(matches) == 0 {
		t.Fatalf("no object %d", number)
	}
	start := matches[len(matches)-1][1]
	end := bytes.Index(pdf[start:], []byte("endobj"))
	if end < 0 {
		t.Fatalf("object %d has no end", number)
	}
	return bytes.TrimSpace(pdf[start : start+end])
}

func testPatchXref(t *testing.T, pdf []byte) {
	t.Helper()
	startxrefs := regexp.MustCompile(`(?m)startxref\s+(\d+)\s*%%EOF\s*$`).FindAllSubmatch(pdf, -1)
	if len(startxrefs) == 0 {
		t.Fatal("no final startxref")
	}
	startxref := startxrefs[len(startxrefs)-1]
	offset := mustAtoi(t, startxref[1])
	if offset < 0 || offset >= len(pdf) || !bytes.HasPrefix(pdf[offset:], []byte("xref")) {
		t.Fatalf("startxref %d does not point at the xref table", offset)
	}

	entries := regexp.MustCompile(`(?m)^(\d{10}) (\d{5}) n $`).FindAllSubmatch(pdf[offset:], -1)
	if len(entries) == 0 {
		t.Fatal("the update has no in-use xref entries")
	}
	for _, entry := range entries {
		objectOffset := mustAtoi(t, entry[1])
		header := regexp.MustCompile(`(?m)^\d+ \d+ obj\b`).Match(pdf[objectOffset:])
		if objectOffset >= offset || !header {
			end := min(objectOffset+24, len(pdf))
			t.Errorf("xref offset %d does not point at an updated object: %q", objectOffset, pdf[objectOffset:end])
		}
	}
	if !strings.Contains(string(pdf[offset:]), "/Prev ") {
		t.Fatal("the update trailer has no /Prev")
	}
}

func mustAtoi(t *testing.T, b []byte) int {
	t.Helper()
	n, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return n
}
