package export

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"

	pdfdoc "rsc.io/pdf"
)

type linkTarget struct {
	URI      string
	X        float64
	Width    float64
	Height   float64
	FontSize float64
	Y        float64
}

type layoutLine struct {
	Left     float64
	Top      float64
	Right    float64
	Bottom   float64
	FontSize float64
}

type pageLayout struct {
	Lines []layoutLine
	Links []linkTarget
	Text  string
}

type pdfGlyph struct {
	page     int
	baseline float64
	x        float64
	width    float64
	fontSize float64
}

type pdfLine struct {
	page     int
	baseline float64
	glyphs   []pdfGlyph
}

const (
	glyphSpanEpsilon      = 0.05
	linkBaselineTolerance = 1.5
	linkVerticalAscent    = 0.72
	linkLineGapCost       = 40.0
	linkColumnGap         = 80.0
)

func locateLinkAnnotations(pdf []byte, layout pageLayout) ([]linkAnnotation, error) {
	if len(layout.Links) == 0 {
		return nil, nil
	}
	sort.SliceStable(layout.Lines, func(i, j int) bool {
		if layout.Lines[i].Top != layout.Lines[j].Top {
			return layout.Lines[i].Top < layout.Lines[j].Top
		}
		return layout.Lines[i].Left < layout.Lines[j].Left
	})
	lines, err := extractPDFLines(pdf)
	if err != nil {
		return nil, fmt.Errorf("读取 PDF 链接位置: %w", err)
	}
	lineMap, err := mapLayoutLines(layout.Lines, lines)
	if err != nil {
		return nil, err
	}
	annotations := make([]linkAnnotation, 0, len(layout.Links))
	for _, target := range layout.Links {
		layoutIndex := layoutLineForTarget(target, layout.Lines)
		if layoutIndex < 0 {
			return nil, fmt.Errorf("读取 PDF 链接位置: 找不到 %q 的布局行", target.URI)
		}
		pdfIndex := lineMap[layoutIndex]
		if pdfIndex < 0 || !targetInLine(target, lines[pdfIndex]) {
			if peer := mappedPeerLine(layoutIndex, layout.Lines, lineMap); peer >= 0 &&
				targetInLine(target, lines[peer]) {
				pdfIndex = peer
			}
		}
		if pdfIndex < 0 {
			line := layout.Lines[layoutIndex]
			return nil, fmt.Errorf(
				"读取 PDF 链接位置: 找不到 %q 的打印行（布局行 %.2f-%.2f，共 %d 个布局行、%d 个打印行）",
				target.URI, line.Top, line.Bottom, len(layout.Lines), len(lines))
		}
		line := lines[pdfIndex]
		expectedLeft := target.X / cssPixelsPerInch * pdfPointsPerInch
		expectedRight := (target.X + target.Width) / cssPixelsPerInch * pdfPointsPerInch
		foundLeft, foundRight, ok := glyphSpan(line.glyphs, expectedLeft, expectedRight)
		if !ok {
			return nil, fmt.Errorf("读取 PDF 链接位置: %q 在 %.2f-%.2f pt 没有字形", target.URI, expectedLeft, expectedRight)
		}

		height := target.Height / cssPixelsPerInch * pdfPointsPerInch
		top := line.baseline + height*linkVerticalAscent
		bottom := top - height
		if top > pageHeightPT {
			top = pageHeightPT
			bottom = top - height
		}
		if bottom < 0 {
			bottom = 0
			top = height
		}
		rectLeft, rectRight := foundLeft, foundRight
		if rectRight <= rectLeft {
			rectLeft, rectRight = expectedLeft, expectedRight
		}

		annotations = append(annotations, linkAnnotation{
			Page: line.page,
			Rect: [4]float64{
				rectLeft,
				bottom,
				rectRight,
				top,
			},
			URI: target.URI,
		})
	}
	return annotations, nil
}

func mappedPeerLine(layoutIndex int, lines []layoutLine, lineMap []int) int {
	current := lines[layoutIndex]
	best := -1
	bestOverlap := 0.0
	for index := range lines {
		if index == layoutIndex || lineMap[index] < 0 {
			continue
		}
		overlap := overlapLength(current.Top, current.Bottom, lines[index].Top, lines[index].Bottom)
		if overlap > bestOverlap {
			best = lineMap[index]
			bestOverlap = overlap
		}
	}
	return best
}

func targetInLine(target linkTarget, line pdfLine) bool {
	left := target.X / cssPixelsPerInch * pdfPointsPerInch
	right := (target.X + target.Width) / cssPixelsPerInch * pdfPointsPerInch
	_, _, ok := glyphSpan(line.glyphs, left, right)
	return ok
}

func layoutLineForTarget(target linkTarget, lines []layoutLine) int {
	targetTop := target.Y
	targetBottom := target.Y + target.Height
	targetLeft := target.X
	targetRight := target.X + target.Width

	best := -1
	bestScore := math.Inf(-1)
	for index, line := range lines {
		vertical := overlapLength(targetTop, targetBottom, line.Top, line.Bottom)
		if vertical <= 0 {
			continue
		}
		horizontal := overlapLength(targetLeft, targetRight, line.Left, line.Right)
		score := vertical + math.Min(horizontal, 10)*0.25
		if score > bestScore {
			best = index
			bestScore = score
		}
	}
	if best >= 0 {
		return best
	}
	return -1
}

func overlapLength(start1, end1, start2, end2 float64) float64 {
	return math.Min(end1, end2) - math.Max(start1, start2)
}

func glyphSpan(glyphs []pdfGlyph, left, right float64) (float64, float64, bool) {
	foundLeft := math.Inf(1)
	foundRight := math.Inf(-1)
	for _, glyph := range glyphs {
		if !glyphWithinSpan(glyph, left, right) {
			continue
		}
		foundLeft = math.Min(foundLeft, glyph.x)
		foundRight = math.Max(foundRight, glyph.x+glyph.width)
	}
	if foundLeft == math.Inf(1) {
		return 0, 0, false
	}
	return math.Max(left, foundLeft), math.Min(right, foundRight), true
}

func glyphWithinSpan(glyph pdfGlyph, left, right float64) bool {
	if glyph.width <= 0 {
		return glyph.x >= left-glyphSpanEpsilon && glyph.x <= right+glyphSpanEpsilon
	}
	center := glyph.x + glyph.width/2
	if center >= left-glyphSpanEpsilon && center <= right+glyphSpanEpsilon {
		return true
	}
	overlap := math.Min(right, glyph.x+glyph.width) - math.Max(left, glyph.x)
	return overlap > 0 && overlap >= glyph.width/2
}

func mapLayoutLines(layoutLines []layoutLine, pdfLines []pdfLine) ([]int, error) {
	if len(pdfLines) == 0 {
		return nil, errors.New("读取 PDF 链接位置: 打印结果没有文本行")
	}
	lineMap := make([]int, len(layoutLines))
	if len(layoutLines) == len(pdfLines) {
		for index := range lineMap {
			lineMap[index] = index
		}
		return lineMap, nil
	}

	const (
		stepMatch = iota + 1
		stepSkipLayout
		stepSkipPDF
	)
	type cell struct {
		cost float64
		step int
	}
	cells := make([][]cell, len(layoutLines)+1)
	for index := range cells {
		cells[index] = make([]cell, len(pdfLines)+1)
	}
	for index := 1; index <= len(layoutLines); index++ {
		cells[index][0] = cell{cost: float64(index) * linkLineGapCost, step: stepSkipLayout}
	}
	for index := 1; index <= len(pdfLines); index++ {
		cells[0][index] = cell{cost: float64(index) * linkLineGapCost, step: stepSkipPDF}
	}
	for layoutIndex := 1; layoutIndex <= len(layoutLines); layoutIndex++ {
		for pdfIndex := 1; pdfIndex <= len(pdfLines); pdfIndex++ {
			match := cells[layoutIndex-1][pdfIndex-1].cost +
				lineMatchCost(layoutLines[layoutIndex-1], pdfLines[pdfIndex-1])
			skipLayout := cells[layoutIndex-1][pdfIndex].cost + linkLineGapCost
			skipPDF := cells[layoutIndex][pdfIndex-1].cost + linkLineGapCost
			best := cell{cost: match, step: stepMatch}
			if skipLayout < best.cost {
				best = cell{cost: skipLayout, step: stepSkipLayout}
			}
			if skipPDF < best.cost {
				best = cell{cost: skipPDF, step: stepSkipPDF}
			}
			cells[layoutIndex][pdfIndex] = best
		}
	}

	for index := range lineMap {
		lineMap[index] = -1
	}
	for layoutIndex, pdfIndex := len(layoutLines), len(pdfLines); layoutIndex > 0 || pdfIndex > 0; {
		switch cells[layoutIndex][pdfIndex].step {
		case stepMatch:
			lineMap[layoutIndex-1] = pdfIndex - 1
			layoutIndex--
			pdfIndex--
		case stepSkipLayout:
			layoutIndex--
		case stepSkipPDF:
			pdfIndex--
		default:
			return nil, errors.New("读取 PDF 链接位置: 无法对齐打印文本行")
		}
	}
	return lineMap, nil
}

func lineMatchCost(layout layoutLine, line pdfLine) float64 {
	const pointsPerPixel = pdfPointsPerInch / cssPixelsPerInch
	right := math.Abs(layout.Right*pointsPerPixel - line.right())
	left := math.Abs(layout.Left*pointsPerPixel - line.left())
	fontSize := math.Abs(layout.FontSize*pointsPerPixel - line.fontSize())
	return right + math.Min(left, 20)*0.25 + fontSize*0.5
}

func (line pdfLine) left() float64 {
	left := math.Inf(1)
	for _, glyph := range line.glyphs {
		left = math.Min(left, glyph.x)
	}
	return left
}

func (line pdfLine) right() float64 {
	right := math.Inf(-1)
	for _, glyph := range line.glyphs {
		right = math.Max(right, glyph.x+glyph.width)
	}
	return right
}

func (line pdfLine) fontSize() float64 {
	if len(line.glyphs) == 0 {
		return 0
	}
	return line.glyphs[0].fontSize
}

func extractPDFLines(pdf []byte) ([]pdfLine, error) {
	reader, err := pdfdoc.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return nil, err
	}

	var glyphs []pdfGlyph
	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		content := reader.Page(pageNumber).Content()
		for _, text := range content.Text {
			glyphs = append(glyphs, pdfGlyph{
				page:     pageNumber - 1,
				baseline: text.Y,
				x:        text.X,
				width:    text.W,
				fontSize: text.FontSize,
			})
		}
	}
	sort.SliceStable(glyphs, func(i, j int) bool {
		if glyphs[i].page != glyphs[j].page {
			return glyphs[i].page < glyphs[j].page
		}
		if glyphs[i].baseline != glyphs[j].baseline {
			return glyphs[i].baseline > glyphs[j].baseline
		}
		return glyphs[i].x < glyphs[j].x
	})

	var lines []pdfLine
	for _, glyph := range glyphs {
		if len(lines) == 0 ||
			glyph.page != lines[len(lines)-1].page ||
			math.Abs(glyph.baseline-lines[len(lines)-1].baseline) > linkBaselineTolerance ||
			glyph.x-lines[len(lines)-1].right() > linkColumnGap {
			lines = append(lines, pdfLine{
				page:     glyph.page,
				baseline: glyph.baseline,
			})
		}
		line := &lines[len(lines)-1]
		line.glyphs = append(line.glyphs, glyph)
	}
	return lines, nil
}
