---
name: tokicv
description: Warm-ash editorial proofing desk for Markdown resume authoring and 1:1 A4 Sheet export.
colors:
  shell-ground: "#EDECE8"
  toolbar-linen: "#E6E4DF"
  sidebar-tray: "#E9E7E2"
  proofing-stage: "#E0DFDB"
  manuscript-paper: "#FBFBF9"
  sheet-paper: "#FFFFFF"
  carbon-ink: "#18191B"
  muted-graphite: "#3A3C42"
  soft-graphite: "#A8ABB2"
  hairline-stone: "#D3D0C8"
  active-card-edge: "#CFCBC2"
  editorial-cobalt: "#1E4D8C"
  cobalt-hover: "#183F72"
  cobalt-pressed: "#12345F"
  cobalt-disabled: "#B8C4D4"
  proof-vermilion: "#C43B2B"
  dialect-ochre: "#9E4B16"
  pill-fill: "#F3F3F3"
  heading-rule: "#000000"
typography:
  display:
    fontFamily: "Nunito, PingFang SC, Noto Sans CJK SC, Microsoft YaHei, sans-serif"
    fontSize: "22.5pt"
    fontWeight: 500
    lineHeight: 1.2
  headline:
    fontFamily: "Nunito, PingFang SC, Noto Sans CJK SC, Microsoft YaHei, sans-serif"
    fontSize: "15pt"
    fontWeight: 500
    lineHeight: 1.3
  title:
    fontFamily: "Nunito, PingFang SC, Noto Sans CJK SC, Microsoft YaHei, sans-serif"
    fontSize: "11.5pt"
    fontWeight: 500
    lineHeight: 1.3
  body:
    fontFamily: "Nunito, PingFang SC, Noto Sans CJK SC, Microsoft YaHei, sans-serif"
    fontSize: "10.5pt"
    fontWeight: 400
    lineHeight: 1.4
  label:
    fontFamily: "PingFang SC, Noto Sans CJK SC, Microsoft YaHei, sans-serif"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: 1.4
  mono:
    fontFamily: "JetBrains Mono, SFMono-Regular, Menlo, monospace"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.65
rounded:
  sheet: "1px"
  control: "4px"
  pill: "12px"
spacing:
  tight: "4px"
  control-gap: "8px"
  panel-padding: "14px"
  stage-padding: "18px"
  rail-width: "224px"
  topbar-height: "46px"
components:
  button-export:
    backgroundColor: "{colors.editorial-cobalt}"
    textColor: "{colors.sheet-paper}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "30px"
  button-export-hover:
    backgroundColor: "{colors.cobalt-hover}"
    textColor: "{colors.sheet-paper}"
    rounded: "{rounded.control}"
    height: "30px"
  button-export-disabled:
    backgroundColor: "{colors.cobalt-disabled}"
    textColor: "{colors.sheet-paper}"
    rounded: "{rounded.control}"
    height: "30px"
  button-toolbar:
    backgroundColor: "transparent"
    textColor: "{colors.carbon-ink}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "30px"
  toggle-smart-active:
    backgroundColor: "{colors.editorial-cobalt}"
    textColor: "{colors.sheet-paper}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "30px"
  document-card-active:
    backgroundColor: "{colors.manuscript-paper}"
    textColor: "{colors.carbon-ink}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "38px"
    width: "204px"
  popover-panel:
    backgroundColor: "{colors.manuscript-paper}"
    textColor: "{colors.carbon-ink}"
    rounded: "{rounded.control}"
    padding: "{spacing.panel-padding}"
    width: "340px"
  page-counter:
    backgroundColor: "{colors.manuscript-paper}"
    textColor: "{colors.proof-vermilion}"
    rounded: "{rounded.pill}"
    padding: "0 10px"
    height: "24px"
---

# Design System: tokicv

## Overview

**Creative North Star: "The Editorial Proofing Desk (铅字校样台)"**

`tokicv` treats resume authoring as proofing a crisp printed sheet on a calm, daylight-balanced editorial desk. Built on `mygo/ui` native desktop controls wrapping an embedded `ui.WebView`, the application shell stays intentionally quiet—warm-ash structural surfaces and 1px hairline rules frame an uncluttered manuscript editor and a pure white A4 `Sheet` resting on the proofing-stage tray.

Expression relies on spatial hierarchy, surface contrast, and single-purpose icons rather than decorative labels or explanatory chrome. The desktop shell never competes with the white A4 `Sheet`, whose typography is pinned 1:1. Editorial cobalt and proof vermilion act strictly as disciplined proofing marks for active state, primary export, overflow, and syntax diagnostics.

**Key Characteristics:**
- Quiet warm-ash workbed that makes the pure white A4 `Sheet` the unmistakable optical anchor.
- Strict zero-redundancy expression: no explanatory subheadings, no duplicated status prose, no decorative dots or colored edge stripes.
- Clear separation between application shell styling (`mygo/ui`) and pinned resume rendering (`ui.WebView`).
- Shared HTML, `PrintToPDF`, and `pdfjs-dist` pipeline keeps DOM preview, PDF, and PNG page geometry aligned.

## Colors

A restrained warm-neutral editorial palette with one cobalt proofing accent and two semantic marks.

### Primary
- **Editorial Cobalt**: Primary export, engaged `智能一页`, editor cursor and selection, focus rings, and active `Split Block` highlighting.
- **Cobalt Hover / Pressed / Disabled**: The three interaction depths of the primary action.

### Semantic
- **Proof Vermilion**: Multi-page counters, readability-limit notices, export failures, save failures, and syntax errors.
- **Dialect Ochre**: Inline `icon:<name>` directive recognition in the editor.

### Neutral
- **Shell Ground, Toolbar Linen, Sidebar Tray, Proofing Stage**: Four adjacent warm-neutral planes establishing the application frame and the recessed stage behind the paper.
- **Manuscript Paper**: Editor writing surface, active document card, toolbar control face, and popover surface.
- **Pinned A4 Sheet**: Pure white paper reserved for the rendered resume.
- **Carbon Ink, Muted Graphite, Soft Graphite**: Primary, secondary, and disabled interface text.
- **Hairline Stone, Active Card Edge**: Structural rules and control borders.
- **Heading Rule**: The pure-black full-width H2 rule inside the resume document.
- **Pill Fill**: The borderless inline `Pill Badge` background inside the resume document.

### Named Rules
**The One-Signal-Per-State Rule.** Every state—active document, `智能一页` enabled, page count, overflow—is communicated once through a single style change, icon, or compact counter.

**The Quiet Desk Rule.** Editorial Cobalt and Proof Vermilion together occupy under 10% of the desktop chrome; inactive controls stay neutral on the toolbar and rail.

## Typography

**Display Font:** Static Nunito for Latin glyphs with PingFang SC priority and a Noto Sans CJK SC Medium fallback for Simplified Chinese.
**Body Font:** Nunito plus the same CJK fallback chain.
**Label / Mono Font:** PingFang SC for shell labels; JetBrains Mono for Markdown source, tabular page counts, and spatial metrics.

**Character:** Calm, high-legibility Simplified Chinese interface copy paired with crisp tabular numerals for page counts and `Layout Profile` metrics.

### Hierarchy
- **Display**: Resume name, 22.5pt at medium weight, tightly set.
- **Headline**: Section headings, 15pt at medium weight with a full-width black rule.
- **Title**: Project subheads, 11.5pt at medium weight.
- **Body**: Resume copy at 10.5pt with a user-adjustable line height never below 1.20.
- **Label**: Shell actions and field labels at 12px.
- **Mono**: Source editor, page counter, and layout metrics.

### Named Rules
**The Locked Sheet Scale Rule.** No font-size controls exist in the toolbar or `Layout Profile`. Font stack, spacing, margins, and readable-bound line height are the only adjustable typography inputs. Their allowed ranges are line height `1.20–2.20`, section spacing `5–40px`, item spacing `1.5–20px`, and top/bottom page margins `7.5–30mm`.

## Layout

A three-zone `mygo/ui` workspace governed by a 46px top bar:
- **Top Bar**: Sidebar toggle, `tokicv`, new, open, save, and import on the left; readable-limit or export status, `智能一页`, `排版`, and `导出` on the right.
- **Document Rail**: 224px wide, collapsed on launch and auto-collapsing at 1280px and below; it stays in flow down to 1171px and becomes an overlay drawer with a scrim below that. Active cards use a raised manuscript surface with a hairline edge.
- **Workbench Split**: A 1px hairline with a 7px drag target between the editor and preview, defaulting to 40% editor and 60% preview of the space after the rail. Arrow keys nudge it by 16px and a double click restores the default; the editor stays at least 420px wide and the preview at least 526px.
- **Manuscript Editor**: Resizable pane with 20px by 24px reading padding.
- **A4 Stage**: The `ui.WebView` keeps each 210mm by 297mm `Sheet` between 80% and 100% of its 1:1 width, stacking Sheets vertically for scrolling; the `1 / N` counter sits in the lower-right tray margin outside the paper.
- **Layout Profile Popover**: Anchored below `排版`, 340px wide, hidden by default; contains the four font stacks, paired section/item spacing fields, paired top/bottom margin fields, line height, save/reset actions, and no font-size control.
- **Export Menu**: Anchored below the cobalt `导出` button and aligned to its end; uses native `mygo/ui` menu semantics for hover, pressed, focus, keyboard navigation, and dismissal while presenting PDF, PNG, and Markdown as compact rows.

## Elevation & Depth

Depth comes primarily from tonal stepping—stage tray to toolbar and rail to manuscript editor to pure white paper—separated by crisp 1px hairlines. Soft shadows appear only on active document cards and the overlay rail; the A4 paper keeps an inset hairline edge.

### Shadow Vocabulary
- **Active Document**: `0 1px 3px rgba(24, 25, 27, 0.06)` with manuscript fill and Active Card Edge border.
- **Overlay Rail**: `0 6px 20px rgba(24, 25, 27, 0.16)` separating the drawer from the workbench and scrim.
- **A4 Sheet**: `outline: 1px solid rgba(24, 25, 27, 0.08)` inset with `outline-offset: -1px`, separating paper from the stage tray.
- **Popover**: Native `mygo/ui` popover elevation, with a 4px radius and hairline border.

## Motion

Motion treats the workspace as a physical proofing desk: the shell makes room for the document rail instead of reloading the page. Transitions are short, one-directional, and never loop; direct manipulation stays immediate.

### Motion Vocabulary
- **Document Rail Open**: 260ms `EaseOut`; the rail width and workbench split expand together, while rail contents keep a fixed 224px layout clipped to the moving tray.
- **Document Rail Close**: 190ms `EaseInOut`; the rail leaves the accessibility tree at once, then finishes its visual exit. The overlay drawer uses the same progress for its scrim and slide.
- **Popover Enter / Exit**: 150ms / 100ms, `EaseOut` / `EaseIn`; 6px travel and fade, mirrored when the panel flips above its anchor.
- **Dialog Scrim**: 180ms / 120ms, `EaseOut` / `EaseIn`; the panel fades with the scrim.
- **State Color**: 120ms `EaseOut`; page-count color and Smart One-Page fill and foreground crossfade without changing layout.

### Named Rules
**The Desk-Makes-Room Rule.** Toggling the document rail changes geometry continuously; the A4 preview updates in place and never reloads its page.

**The Immediate-Hand Rule.** Dragging the split, typing, scrolling, and pointer hover remain immediate; transitions explain state changes, never delay direct manipulation.

**The Reduced-Motion Rule.** Reduce Motion removes spatial travel but keeps opacity and color changes that carry state.

## Shapes

Crisp, understated stationery geometry. Shell controls and document cards use 4px corners with 1px borders. Editor syntax highlights use a 2px-radius pill background. The A4 `Sheet` keeps sharp 0–1px corners for print fidelity.

## Components

### Buttons
- **Shape:** 4px radius, 30px high for toolbar and export actions.
- **Primary Export:** Editorial Cobalt fill with white label and export icon; hover and pressed states deepen through Cobalt Hover and Cobalt Pressed.
- **Toolbar:** Transparent face with Carbon Ink label; hover uses Sidebar Tray, pressed uses Hairline Stone.
- **Focus:** A cobalt focus ring sits one pixel outside the control; disabled actions use Soft Graphite or Cobalt Disabled.

### Smart One-Page Toggle
- **Shape:** 4px radius, 30px high, icon plus `智能一页` label.
- **Off:** Manuscript Paper face with a Hairline Stone border on the toolbar.
- **On:** Full Editorial Cobalt fill with white icon and label.
- **Solving:** The control reads `正在压缩为一页` while its exact single-Sheet fit runs.
- **Behavior:** Turning the toggle off restores the document's base `Layout Profile`; text edits while enabled debounce into an automatic refit, and an overflowing result falls back to the baseline with the readability notice.

### Layout Profile Popover
- **Surface:** Manuscript Paper panel, 340px wide, 14px padding, 4px radius.
- **Controls:** Four-segment font stack selector; centered monospaced numeric fields with drawn minus/plus icon buttons for `分节间距`, `条目间距`, `上边距`, and `下边距`; a single `行高` field; a hairline rule separating the actions.
- **Actions:** `设为全局默认`, `设为当前简历`, `清除当前覆盖`, and `恢复默认`; paired in equal-width rows and kept readable at the minimum window size.
- **Behavior:** Coarse-step buttons move by `2px`, `1px`, `1mm`, and `0.05`; the text fields still accept fine values such as `1.25`, `7.5`, and `1.5`. Smart One-Page results populate the same fields, and a manual edit exits Smart One-Page with the edited spatial profile as the new baseline. Changes refresh the persistent DOM preview immediately. Persistence writes only the library's `layouts.json`; the Markdown source bytes remain untouched.

### Document Cards
- **Shape:** 4px radius, 38px high, 10px horizontal padding.
- **Active:** Manuscript Paper fill, Active Card Edge border, and a subtle 1px offset shadow.
- **Inactive:** Transparent face on Sidebar Tray with a faint manuscript hover.

### Page Counter
- **Shape:** 24px-high pill with 12px radius and a Hairline Stone border.
- **Type:** Monospaced, tabular numerals, 11px medium.
- **Color:** Carbon Ink for one Sheet; Proof Vermilion for multiple Sheets.

### A4 Sheet
- **Geometry:** 210mm by 297mm, printed with zero external PDF margins and internal sheet padding.
- **Typography:** Locked 1:1 scale with readable line-height bounds.
- **Pagination:** DOM preview slots and exported Sheets share the same A4 aspect ratio and page count for parity coverage.
- **List Markers:** Font-independent geometric marks, with a filled square for the first level and a hollow circle for nested levels.

### Shipping Assets
- `resources/icon.png`: 1024×1024 transparent-background application icon built with ImageGen (`image_gen`), depicting a folded warm-ivory A4 sheet stacked over a symmetrical dark-graphite backing sheet with a carbon-ink header block, an editorial-cobalt pill badge, and a two-column split block.

## Do's and Don'ts

### Do:
- **Do** keep control labels concise and express states through a single fill, color, or icon change.
- **Do** keep the page counter outside the A4 footprint in the lower-right stage margin.
- **Do** start with the document rail collapsed and keep the editor at 420px or wider and the preview at 526px or wider.
- **Do** preserve a readable 80–100% A4 `Sheet` and the toolbar at 1280×800 through 1920×1080 without window zoom; vertical scrolling is expected.
- **Do** keep layout and Smart One-Page changes non-destructive: only WebView CSS, print layout, and `layouts.json` may change.
- **Do** use bundled vector icons on a 16×16 viewBox with optically matched strokes and filled silhouettes.

### Don't:
- **Don't** add explanatory subheadings, kickers, or redundant status text such as `校样工作台`, `本地简历库`, `已保存`, or `行列号`.
- **Don't** place colored vertical stripes or unexplained status dots on sidebar cards.
- **Don't** expose font-size sliders, inputs, or menu items anywhere in the desktop shell or `Layout Profile`.
- **Don't** force-shrink below the 1.20 line-height floor; retain the multi-page baseline and show the single readability-limit notice.
- **Don't** introduce emoji or raster glyphs as UI icons.
