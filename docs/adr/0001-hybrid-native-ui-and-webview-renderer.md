# Hybrid Native UI Shell and Embedded WebView Renderer

Status: accepted

tokicv builds the desktop shell, resume sidebar, toolbar, and Markdown source editor using `mygo/ui` (including `ui/editor`), while embedding a `ui.WebView` for live pure-HTML/CSS/DOM A4 Sheet pagination preview, `embeddedWebView.Page().PrintToPDF` export, and bundled `pdfjs-dist` PDF-to-PNG rasterization.

Native UI cannot call `Page.PrintToPDF` (calling `win.Page().PrintToPDF` on the outer `Content` window returns `errNoPage`) and lacks an HTML/CSS pagination engine, whereas a full-WebView UI would discard native desktop controls (`ui/editor`, sidebar, popovers) that `mygo/ui` provides natively. Live preview uses fast DOM A4 pagination for responsive editing and crisp vector text, while PDF export, PNG export, and page-count verification invoke `embeddedWebView.Page().PrintToPDF` with explicit A4 dimensions, margins, and `Background: true` so preview, PDF, and PNG share one HTML/CSS layout without external OS binaries like `pdftoppm`. Cross-platform builds remain clean without OS-specific tools, with primary runtime and visual verification focused on Linux desktop.
