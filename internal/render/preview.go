package render

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"tokicv/internal/dialect"
)

// PreviewPageTitlePrefix prefixes the page count a preview reports through
// its document title.
const PreviewPageTitlePrefix = "tokicv:pages="

// PreviewFrame is one rendered Resume Document and Layout Profile for the
// persistent live preview shell.
type PreviewFrame struct {
	HTML    string
	Content string
	Layout  Layout
}

// PreviewHTML renders a Resume Document as a standalone live preview. The
// Resume Sheet keeps its locked A4 geometry while JavaScript paginates the
// rendered DOM into scaled Sheet slots.
func PreviewHTML(doc dialect.Document, layout Layout) string {
	return PreviewFrameFor(doc, layout).HTML
}

// PreviewFrameFor renders the persistent preview shell and its in-place update
// payload from one Document and Layout Profile.
func PreviewFrameFor(doc dialect.Document, layout Layout) PreviewFrame {
	layout = layout.normalized()
	var content strings.Builder
	for _, block := range doc.Blocks {
		renderBlock(&content, block)
	}

	var out strings.Builder
	out.Grow(len(stylesheet(layout)) + content.Len() + len(previewCSS) + len(previewScript) + 2048)
	out.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light">
<title>tokicv</title>
<style>
`)
	out.WriteString(stylesheet(layout))
	out.WriteString(previewCSS)
	fmt.Fprintf(&out, `
</style>
</head>
<body>
<div id="stage" role="region" aria-label="A4 预览"></div>
<div id="a4-probe" data-margin-top="%s" data-margin-bottom="%s" aria-hidden="true"></div>
<div id="measure" aria-hidden="true"></div>
<article id="accessible" class="visually-hidden" aria-label="简历预览"></article>
<template id="resume-source">
`, formatNumber(layout.MarginTopMM), formatNumber(layout.MarginBottomMM))
	out.WriteString(content.String())
	out.WriteString(`</template>
<script>
`)
	out.WriteString(previewScript)
	out.WriteString(`
</script>
</body>
</html>
`)
	return PreviewFrame{
		HTML:    out.String(),
		Content: content.String(),
		Layout:  layout,
	}
}

// PreviewUpdateScript asks the loaded preview shell to replace its document
// content and Layout Profile variables without navigating or rebuilding the
// page background.
func PreviewUpdateScript(frame PreviewFrame) string {
	layout := frame.Layout.normalized()
	payload, err := json.Marshal(struct {
		Content      string `json:"content"`
		FontFamily   string `json:"fontFamily"`
		LineHeight   string `json:"lineHeight"`
		MarginTop    string `json:"marginTop"`
		MarginBottom string `json:"marginBottom"`
		SectionGap   string `json:"sectionGap"`
		ItemGap      string `json:"itemGap"`
	}{
		Content:      frame.Content,
		FontFamily:   layout.FontStack.cssFamily(),
		LineHeight:   formatNumber(layout.LineHeight),
		MarginTop:    formatNumber(layout.MarginTopMM),
		MarginBottom: formatNumber(layout.MarginBottomMM),
		SectionGap:   formatNumber(layout.SectionGapPX),
		ItemGap:      formatNumber(layout.ItemGapPX),
	})
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`(() => {
  if (!window.tokicvPreview || typeof window.tokicvPreview.update !== "function") return false;
  window.tokicvPreview.update(%s);
  return true;
})()`, payload)
}

// PreviewPageTitle returns the document title that reports n preview pages.
func PreviewPageTitle(n int) string {
	if n < 1 {
		n = 1
	}
	return PreviewPageTitlePrefix + strconv.Itoa(n)
}

// PreviewPageCount reads the page count from a preview document title.
func PreviewPageCount(title string) (int, bool) {
	value, ok := strings.CutPrefix(strings.TrimSpace(title), PreviewPageTitlePrefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

const previewCSS = `
html {
  background: #E0DFDB;
}
body {
  margin: 0;
  padding: 0;
  overflow: auto;
  background: #E0DFDB;
}
#stage {
  box-sizing: border-box;
  min-height: 100vh;
  padding: 18px 20px 48px;
  display: flex;
  flex-direction: column;
  align-items: center;
  align-items: safe center;
  gap: 18px;
}
.sheet-slot {
  position: relative;
  flex: 0 0 auto;
  background: #FFFFFF;
  border-radius: 1px;
}
.sheet {
  position: absolute;
  top: 0;
  left: 0;
  width: 210mm;
  height: 297mm;
  overflow: hidden;
  background: #FFFFFF;
  transform-origin: top left;
  outline: 1px solid rgba(24, 25, 27, 0.08);
  outline-offset: -1px;
}
.page-flow {
  position: absolute;
  left: 13.229mm;
  width: 183.542mm;
  overflow: hidden;
}
.page-flow-content {
  position: relative;
  display: flow-root;
}
#a4-probe {
  position: fixed;
  top: 0;
  left: -10000px;
  width: 210mm;
  height: 297mm;
  visibility: hidden;
  pointer-events: none;
}
#measure {
  position: absolute;
  top: 0;
  left: -10000px;
  width: 183.542mm;
  visibility: hidden;
  pointer-events: none;
  display: flow-root;
}
.visually-hidden {
  position: absolute !important;
  width: 1px !important;
  height: 1px !important;
  padding: 0 !important;
  margin: -1px !important;
  overflow: hidden !important;
  clip: rect(0, 0, 0, 0) !important;
  white-space: nowrap !important;
  border: 0 !important;
}
`

const previewScript = `
(() => {
  const stage = document.getElementById("stage");
  const probe = document.getElementById("a4-probe");
  const measure = document.getElementById("measure");
  const source = document.getElementById("resume-source");
  const accessible = document.getElementById("accessible");
  if (!stage || !probe || !measure || !source || !accessible) return;

  let pageWidth = 0;
  let pageHeight = 0;
  let contentHeight = 0;
  let marginTop = 0;
  let slots = [];

  const mm = 96 / 25.4;

  function measureBlocks(measureTop) {
    const blocks = [];
    for (const element of Array.from(measure.children)) {
      const rect = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      const marginBefore = parseFloat(style.marginTop) || 0;
      const marginAfter = parseFloat(style.marginBottom) || 0;
      blocks.push({
        top: rect.top - measureTop - marginBefore,
        bottom: rect.bottom - measureTop + marginAfter
      });
    }
    return blocks;
  }

  function measureLines(measureTop) {
    const rects = [];
    const walker = document.createTreeWalker(measure, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      if (!node.nodeValue || !node.nodeValue.trim()) continue;
      const range = document.createRange();
      range.selectNodeContents(node);
      for (const rect of range.getClientRects()) {
        if (rect.height > 0 && rect.width > 0) {
          rects.push({
            top: rect.top - measureTop,
            bottom: rect.bottom - measureTop
          });
        }
      }
    }
    rects.sort((a, b) => a.top - b.top || a.bottom - b.bottom);
    const lines = [];
    for (const rect of rects) {
      const line = lines[lines.length - 1];
      const tolerance = Math.max(1.5, Math.min(rect.bottom - rect.top,
        line ? line.bottom - line.top : rect.bottom - rect.top) * 0.3);
      if (line && Math.abs(rect.top - line.top) <= tolerance) {
        line.top = Math.min(line.top, rect.top);
        line.bottom = Math.max(line.bottom, rect.bottom);
        continue;
      }
      lines.push({ top: rect.top, bottom: rect.bottom });
    }
    for (let index = 0; index + 1 < lines.length; index++) {
      lines[index].bottom = lines[index + 1].top;
    }
    return lines;
  }

  function nextBreak(start, flowHeight, blocks, lines) {
    const limit = start + contentHeight;
    if (flowHeight <= limit + 0.5) return flowHeight;

    let crossing = null;
    for (const block of blocks) {
      if (block.bottom > limit + 0.5 && block.top < limit + 0.5) {
        crossing = block;
        break;
      }
    }

    if (crossing && crossing.top > start + 0.5 &&
        crossing.bottom - crossing.top <= contentHeight + 0.5) {
      return crossing.top;
    }

    let candidate = -1;
    for (const line of lines) {
      if (line.bottom > start + 2 && line.bottom <= limit + 0.5) candidate = line.bottom;
      if (line.bottom > limit + 0.5) break;
    }
    if (candidate > start + 2) return candidate;
    return Math.min(limit, flowHeight);
  }

  function buildPages() {
    pageWidth = probe.getBoundingClientRect().width;
    pageHeight = probe.getBoundingClientRect().height;
    marginTop = (parseFloat(probe.dataset.marginTop) || 10) * mm;
    const marginBottom = (parseFloat(probe.dataset.marginBottom) || 10) * mm;
    contentHeight = Math.max(1, pageHeight - marginTop - marginBottom);
    measure.replaceChildren(source.content.cloneNode(true));
    const measureRect = measure.getBoundingClientRect();

    const blocks = measureBlocks(measureRect.top);
    const lines = measureLines(measureRect.top);
    let flowHeight = Math.max(
      measure.scrollHeight,
      measure.getBoundingClientRect().height
    );
    for (const block of blocks) flowHeight = Math.max(flowHeight, block.bottom);

    const ranges = [];
    let start = 0;
    while (start < flowHeight - 0.5 && ranges.length < 200) {
      let next = nextBreak(start, flowHeight, blocks, lines);
      if (next <= start + 1) next = Math.min(start + contentHeight, flowHeight);
      if (next >= flowHeight - 0.5) {
        ranges.push({ start, end: flowHeight });
        start = flowHeight;
        break;
      }
      ranges.push({ start, end: next });
      start = next;
    }
    if (ranges.length === 0) ranges.push({ start: 0, end: flowHeight });
    const fragment = document.createDocumentFragment();
    slots = [];
    for (const range of ranges) {
      const slot = document.createElement("section");
      slot.className = "sheet-slot";
      slot.setAttribute("aria-hidden", "true");

      const sheet = document.createElement("div");
      sheet.className = "sheet";
      const viewport = document.createElement("div");
      viewport.className = "page-flow";
      viewport.style.top = marginTop + "px";
      viewport.style.height = (range.end - range.start) + "px";
      viewport.dataset.pageStart = String(range.start);
      viewport.dataset.pageEnd = String(range.end);
      const flow = document.createElement("div");
      flow.className = "page-flow-content";
      flow.style.transform = "translateY(" + (-range.start) + "px)";
      flow.appendChild(source.content.cloneNode(true));
      viewport.appendChild(flow);
      sheet.appendChild(viewport);
      slot.appendChild(sheet);
      fragment.appendChild(slot);
      slots.push({ slot, sheet });
    }
    stage.replaceChildren(fragment);
    accessible.replaceChildren(source.content.cloneNode(true));
    document.documentElement.dataset.tokicvPages = String(ranges.length);
  }

  function fit() {
    if (!pageWidth || !pageHeight) return;
    const availableWidth = Math.max(1, stage.clientWidth - 40);
    const scale = Math.max(0.8, Math.min(availableWidth / pageWidth, 1));
    const width = pageWidth * scale;
    const height = pageHeight * scale;
    for (const { slot, sheet } of slots) {
      slot.style.width = width + "px";
      slot.style.height = height + "px";
      sheet.style.transform = "scale(" + scale + ")";
    }
  }

  function update(frame) {
    const root = document.documentElement;
    root.style.setProperty("--font-family", frame.fontFamily);
    root.style.setProperty("--line-height", frame.lineHeight);
    root.style.setProperty("--margin-top", frame.marginTop + "mm");
    root.style.setProperty("--margin-bottom", frame.marginBottom + "mm");
    root.style.setProperty("--section-gap", frame.sectionGap + "px");
    root.style.setProperty("--item-gap", frame.itemGap + "px");
    probe.dataset.marginTop = frame.marginTop;
    probe.dataset.marginBottom = frame.marginBottom;
    source.innerHTML = frame.content;
    buildPages();
    fit();
    document.title = "` + PreviewPageTitlePrefix + `" + document.documentElement.dataset.tokicvPages;
  }

  async function start() {
    measure.replaceChildren(source.content.cloneNode(true));
    await document.fonts.ready;
    await new Promise(resolve => {
      let settled = false;
      const done = () => {
        if (!settled) {
          settled = true;
          resolve();
        }
      };
      requestAnimationFrame(() => requestAnimationFrame(done));
      setTimeout(done, 50);
    });
    buildPages();
    fit();
    document.title = "` + PreviewPageTitlePrefix + `" + document.documentElement.dataset.tokicvPages;
    window.addEventListener("resize", fit);
    if (typeof ResizeObserver === "function") {
      new ResizeObserver(fit).observe(stage);
    }
  }

  window.tokicvPreview = { update };
  start().catch(error => console.error("tokicv preview:", error));
})();
`
