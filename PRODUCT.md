# Product

<!-- impeccable:product-schema 1 -->

## Platform

desktop

## Stack

Go + `github.com/egoist/mygo v0.9.0` hybrid desktop application (`mygo/ui` native shell, `ui/editor` source editor, embedded `ui.WebView` for pure HTML+CSS+DOM A4 `Sheet` pagination preview, `PrintToPDF` export/page-count verification, and `pdfjs-dist` PNG rasterization) plus equivalent CLI. `Resume Dialect` standard Markdown parsing is implemented with `github.com/yuin/goldmark`. The codebase remains cross-platform (macOS, Windows, Linux) with zero Linux-only external binary dependencies (such as `pdftoppm`), while verification and runtime guarantees focus on Linux desktop.

## Users

Job seekers drafting resumes who frequently tailor versions per target role and strictly manage content to fit on a single A4 `Sheet`, operating in a Simplified Chinese (`zh-CN`) interface.

## Product Purpose

Provide a standalone, zero-dependency, out-of-the-box Markdown resume authoring, live A4 pagination preview, and pixel-identical PDF/PNG export desktop tool with an equivalent scriptable CLI.

## Positioning

Unlike web resume builders with multi-template clutter or note-vault plugins coupled to external editors, `tokicv` pairs a focused `Resume Dialect` with one precision-calibrated classic layout, non-destructive `Layout Profile` overrides, `Smart One-Page` pagination control, and a single `ui.WebView` rendering engine shared by GUI preview and offscreen CLI execution so screen preview, PDF, and PNG match 1:1.

## Operating Context

Maintains a self-contained resume library and `Layout Profile` store inside `mygo.PathUserData`, completely decoupled from external note vaults, while allowing desktop users to open/import/export arbitrary `.md` files and allowing CLI `export`/`check` commands to run directly on any relative or absolute `.md` file path.

## Capabilities and Constraints

- **Interface language**: Simplified Chinese (`zh-CN`) only; no multi-language UI switching.
- **Single-layout focus**: One classic high-precision A4 resume layout with `Layout Profile` tuning and `Smart One-Page`; no multi-template marketplace.
- **Resume Dialect**: Goldmark-backed standard Markdown for headings, emphasis, links, code spans, lists, and escaped raw HTML, plus two-column `Split Block` (`::: left` / `::: right` / `:::`), inline `Pill Badge`, and inline `Icon Directive` (`icon:<name>`).
- **Locked 1:1 Sheet Typography**: All A4 `Sheet` font sizes are strictly locked to reference samples (`H1` `22.5pt`, `H2` `15pt`, body `10.5pt`, subtitle/date/`Pill Badge` `10pt`, inline `**bold**` inherits `10.5pt`). `H2` uses a pure-black full-width rule; Split Block body rows use the body size, and dates use `#010101`. No font-size controls exist in desktop UI or `Layout Profile`.
- **Non-destructive Layout Profile**: Built-in global default + per-Resume Document overrides stored outside `.md` source files, limited strictly to font stack switching and spatial parameters. Readable bounds are line height `1.20–2.20`, section spacing `5–40px`, item spacing `1.5–20px`, and top/bottom page margins `7.5–30mm`.
- **Smart One-Page (`Page Fit`)**: Tightens strictly in order (gentle `8px/3px` section/item spacing → readable `1.40 → 1.30 → 1.20` line height → extreme `5px/1.5px` spacing → top/bottom margins `10mm → 7.5mm`) without ever reducing font size; candidates are DOM-prefiltered in one WebView session and confirmed with `PrintToPDF`. If still exceeding 1 `Sheet`, preserves the original multi-page layout and surfaces a clear readability-limit notice.
- **Font strategy**: Bundled static `Nunito` `400/500/600/700` WOFF2 files, with Simplified-Chinese fallback resolved toward Medium for display and emphasis text, plus cross-platform fallbacks and manual switching.
- **Resizable Workbench**: The document rail starts collapsed; the editor and preview share a draggable 40/60 split with the editor at least 420px and the preview at least 526px, and the rail becomes an overlay drawer below 1171px.
- **Live DOM Preview vs Export**: The right-hand preview keeps a persistent shell, renders each Sheet at a readable 80–100% width with vertical scrolling, and updates its source HTML, margin attributes, and layout CSS variables in place before repaginating; `PrintToPDF` runs only on PDF export, PNG export (`pdfjs-dist`), and page-count verification.
- **CLI parity**: Pure-Go informational commands (`--version`, `--help`, `list`, `create`, `show`, `delete`) execute before `mygo.App.Run()`; rendering commands (`export`, `check`) run via offscreen `ui.WebView` (`Hidden: true`, macOS `ActivationPolicyAccessory`).
- **Application & CLI Name**: `tokicv` (unified desktop application title and lowercase CLI binary name).

## Brand Commitments

- The right-hand A4 `Sheet` layout and `Resume Dialect` rendering are strictly pinned 1:1 to the reference resume format.
- The surrounding desktop application shell, editor, and controls use native-feeling `mygo/ui` components wrapping the embedded `ui.WebView`.

## Evidence on Hand

- Reference Markdown resume sources and PDF outputs from the target classic resume format (`GLOSSARY.md`, `docs/agents/domain.md`, and `docs/adr/0001`–`0003`).

## Product Principles

- **What You Preview Is What You Export**: GUI preview, PDF `Artifact`, and PNG `Artifact` share one WebView layout and print pipeline across platforms and CLI.
- **Non-Destructive Formatting**: Markdown source content stays clean; `Layout Profile` adjustments and `Smart One-Page` tightening never mutate `.md` text.
- **Precision Over Variety**: One rigorously calibrated A4 layout with deterministic `Page Fit` feedback rather than distracting template sprawl.
- **Self-Contained and Scriptable**: Local library storage in `mygo.PathUserData` with path-independent file workflows in both desktop and CLI.

## Accessibility & Inclusion

Keyboard-accessible `mygo/ui` desktop controls and `ui/editor` shortcuts, clear focus indicators, and platform accessibility semantics exposed via `mygo/ui`.
