# Offscreen WebView Execution for Rendering CLI Commands

Status: accepted

Non-rendering CLI commands (`--version`, `--help`, `list`, `create`, `show`, `delete`, `import`) parse flags and execute in pure Go before `mygo.App.Run()` without touching GUI loops or creating library directories prematurely. Rendering CLI commands (`export`, `check`) start `mygo.App.Run()` with an unshown (`Hidden: true`, macOS `ActivationPolicyAccessory`, `BackgroundThrottling: mygo.ThrottlingOff`) WebView window, wait deterministically for page load + `document.fonts.ready` + layout, invoke `PrintToPDF`, and use bundled `pdfjs-dist` (`intent: 'print'` to avoid hidden `requestAnimationFrame` stalls) for exact `numPages` and PNG rasterization under a hard timeout before exiting cleanly.

Using a separate pure-Go PDF generator for CLI would cause line-wrap and font-metric drift against the desktop WebView. Reusing the same `mygo` WebView engine guarantees identical pagination and visual layout between GUI and CLI on the same OS/font environment (and strict PDF↔PNG parity within each export), at the trade-off that rendering CLI commands require a graphical session on Linux.
