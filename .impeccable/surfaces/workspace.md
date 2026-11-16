---
version: 1
slug: "workspace"
primary_target: "workspace"
related_targets: []
---

# Surface Brief: Desktop Workspace (`workspace`)

- **Mode**: Operate
- **Primary Surface**: Cross-platform `mygo/ui` native desktop shell (`sidebar` + `ui/editor` + embedded `ui.WebView` A4 `Sheet` stage), verified primarily on Linux desktop
- **Audience & Job**: Job seekers authoring and tailoring Markdown resumes per target role while keeping content cleanly within one A4 `Sheet` before exporting pixel-identical PDF / PNG `Artifact`s.

## Direction contract

THESIS: Treat resume authoring as proofing a print sheet on a calm editorial desk—a quiet warm-ash `mygo/ui` shell that disappears behind the pure white A4 `Sheet`, refusing dark IDE clutter, multi-template sprawl, and explanatory UI prose.
OWN-WORLD: Warm-ash workbed (`#EDECE8` shell, `#E6E4DF` bar, `#E9E7E2` rail, `#E0DFDB` tray), distraction-free `#FBFBF9` manuscript editor, pure `#FFFFFF` A4 `Sheet`, deep carbon ink (`#18191B`), editorial cobalt (`#1E4D8C`) solely for active toggles/primary export, and proof vermilion (`#C43B2B`) strictly for overflow and syntax errors.
STORY: Users immediately see their resume list, clean `Resume Dialect` source, and live 1:1 A4 `Sheet` fit (`1 / 1`) without zooming, trusting that non-destructive `Layout Profile` tuning and `Smart One-Page` keep `.md` text untouched and exports pixel-identical.
FIRST VIEWPORT: Compact `46px` top bar (sidebar toggle, `tokicv`, new, open on left; `智能一页` toggle, `排版` popover trigger, `导出` primary action on right), collapsible `224px` document rail with elevated active card (no colored left stripe, no mystery dots), `flex: 1` `ui/editor`, and proportional `ui.WebView` A4 `Sheet` with `1 / N` counter parked cleanly below the bottom-right corner of the paper.
FORM: Candidate 7 (`铅字校样台 · 修订版` / Editorial Proofing Desk), seed:d179a645490ceb94.
FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Interaction & Component Boundaries

- **Expression Discipline (Confirmed)**:
  - Never render explanatory subheadings, kickers, or descriptive banners (e.g., no「校样工作台」「本地简历库」「已保存」「行列号」chrome clutter).
  - Express each concept once via style or icon: active list item uses raised `#FBFBF9` surface + `1px` border + subtle shadow (no colored left stripe, no status dots).
- **Viewport Responsiveness**:
  - Scale the right `ui.WebView` A4 `Sheet` container proportionally within available viewport height (`1280×800` through `1920×1080`+) and auto-collapse or narrow the sidebar when width is constrained so the full A4 `Sheet` and toolbar remain operable at 100% scale without browser/window zooming.
- **Layout Profile Popover (`排版`, Default Collapsed)**:
  - **Zero font-size controls**: A4 `Sheet` font sizes are locked to the reference (`H1` `22.5pt`, `H2` `15pt`, body `10.5pt`, subtitle/date/`Pill Badge` `10pt`, `**bold**` inherits `10.5pt`).
  - Exposes only: font stack switcher (`Nunito + PingFang SC` priority + fallbacks), section spacing `5–40px`, item spacing `1.5–20px`, top/bottom page margins `7.5–30mm`, line height `1.20–2.20`, and `全局默认` / `单篇覆盖` reset control.
- **Smart One-Page (`智能一页`) & Page Fit**:
  - Tightens strictly in order: gentle `section/list spacing` → `1.40 → 1.30 → 1.20` line height → minimum section/list spacing → `10mm → 7.5mm` vertical margins; never shrinks font size. Result values populate the same fields, and manual edits exit Smart mode with the edited spatial profile as the new baseline.
  - If still exceeding 1 `Sheet` at the lower bound, preserves the multi-page layout (no forced compression) and displays a clear non-blocking notice that single-page fit cannot be achieved without harming readability.
- **Preview vs Export Pipeline**:
  - Live preview uses pure HTML + CSS + DOM A4 pagination inside `ui.WebView`.
  - `PrintToPDF` runs only on PDF export, PNG export (via bundled `pdfjs-dist`), and page-count verification.
- **Key States**:
  - **Empty Library**: Clean onboarding state in the workspace offering「新建空白简历」「从示例模板创建」「打开本地 `.md` 文件」.
  - **External `.md` Path**: Opens directly in editor + live preview with path context and non-destructive per-file override or one-click copy into library.
  - **Syntax Diagnostics**: Unclosed `:::` `Split Block` or unknown `icon:<name>` shows subtle inline editor gutter marker and concise status indicator without silently corrupting layout.
