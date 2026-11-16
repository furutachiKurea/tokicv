# tokicv

tokicv is a standalone cross-platform resume authoring, A4 pagination preview, and export tool.

## Language

**Resume Document**:
A Markdown source unit written in the supported resume dialect and stored either inside the application's local library or at an arbitrary user-specified filesystem path.
_Avoid_: Note, template, CV record

**Resume Dialect**:
The Goldmark-backed structured Markdown syntax subset supporting headings (`#`, `##`, `###`), emphasis, inline pill badges, links, paired split blocks, nested unordered lists (font-independent filled square at level 1 and hollow circle at level 2+), and inline icon directives (`info`, `github`, `phone`, `email`, `email2`, `weixin`, `blog`).
_Avoid_: Custom markup, template language

**Split Block**:
Two adjacent, individually closed containers (`::: left ... :::` followed by `::: right ... :::`, with optional whitespace after `:::`) paired into a single horizontal two-column band; an unpaired container renders gracefully as a single column.
_Avoid_: Flex row, two-column div, grid block

**Pill Badge**:
An inline code span rendered as a compact, borderless gray tag beside headings or inline technical terms.
_Avoid_: Chip, label, inline code block

**Icon Directive**:
A prefixed token (`icon:<name>`, normalizing whitespace variants such as `icon:email 2`) rendered as an inline vector glyph aligned to the baseline of its line item, falling back gracefully to plain text when unrecognized.
_Avoid_: Emoji, image tag, icon shortcut

**Layout Profile**:
The non-destructive spatial and font-stack configuration—font family selection, line height (bounded above the reference compact floor), top/bottom page margins, and section/list spacing—applied under locked 1:1 reference font sizes without mutating the Resume Document.
_Avoid_: Theme, stylesheet, frontmatter config

**Sheet**:
A single A4 page (`210mm × 297mm`) produced by laying out a Resume Document under a Layout Profile.
_Avoid_: Canvas, viewport, slide

**Page Fit**:
The pagination measurement of a rendered Resume Document (`1 / N` Sheets and whether it fits on one Sheet).
_Avoid_: Page count estimate, viewport fit

**Smart One-Page**:
The deterministic spatial tightening pass that preserves locked font sizes and steps through gentle section/list spacing, readable-floor line height, extreme section/list spacing, and top/bottom margins to fit onto one Sheet, reverting to the unforced multi-page Layout Profile with a readability notice when single-page fit cannot be achieved cleanly.
_Avoid_: Auto-shrink, zoom-to-fit, forced compression

**Artifact**:
An exported PDF document or per-page PNG image generated from the exact rendered layout of a Resume Document.
_Avoid_: Printout, screenshot, snapshot
