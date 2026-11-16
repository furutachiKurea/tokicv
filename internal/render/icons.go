package render

import (
	"fmt"
	"strings"
)

type iconGlyph struct {
	viewBox string
	body    string
}

var iconGlyphs = map[string]iconGlyph{
	"info": {
		viewBox: "0 0 16 16",
		body:    `<circle cx="8" cy="4.1" r="3.2"/><path d="M1.4 13.5c0-3.35 2.7-5.25 6.6-5.25s6.6 1.9 6.6 5.25v.5H1.4z"/>`,
	},
	"github": {
		viewBox: "0 0 16 16",
		body:    `<g transform="translate(1.2 1.2) scale(.85)"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"/></g>`,
	},
	"phone": {
		viewBox: "0 0 16 16",
		body:    `<rect x="3.6" y="0.5" width="8.8" height="15" rx="1.5" fill="currentColor"/><rect x="4.9" y="2.6" width="6.2" height="9.5" fill="#ffffff"/><circle cx="8" cy="13.8" r=".85" fill="#ffffff"/>`,
	},
	"email": {
		viewBox: "0 0 16 16",
		body:    `<path d="M1.6 4.1c0-.5.4-.9.9-.9h11c.5 0 .9.4.9.9v7.8c0 .5-.4.9-.9.9h-11a.9.9 0 0 1-.9-.9V4.1z"/><path d="m2.3 4.3 5.7 4.2 5.7-4.2" fill="none" stroke="#ffffff" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.15"/>`,
	},
	"email2": {
		viewBox: "0 0 16 16",
		body:    `<g transform="translate(-.48 -.48) scale(1.03)"><rect x="1.6" y="3.2" width="12.8" height="9.6" rx=".55" fill="none" stroke="currentColor" stroke-width="1.25"/><path d="m2.35 4.05 5.65 4.2 5.65-4.2M2.35 11.95 6.3 8.35M13.65 11.95 9.7 8.35" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.05"/></g>`,
	},
	"weixin": {
		viewBox: "0 0 16 16",
		body:    `<path d="M6.2 2.3c2.56 0 4.62 1.63 4.62 3.64 0 .24-.03.48-.09.71-.42-.14-.87-.22-1.33-.22-2.22 0-4.03 1.44-4.03 3.22 0 .2.02.4.07.59a5.6 5.6 0 0 1-.8-.13l-1.62.83.42-1.4c-1.14-.65-1.88-1.74-1.88-2.96 0-2.01 2.06-3.64 4.64-3.64zm-1.7 2.2a.58.58 0 1 0 0 1.16.58.58 0 0 0 0-1.16zm3.4 0a.58.58 0 1 0 0 1.16.58.58 0 0 0 0-1.16z"/><path d="M15 9.55c0-1.7-1.76-3.08-3.93-3.08s-3.93 1.38-3.93 3.08 1.76 3.08 3.93 3.08c.42 0 .82-.05 1.2-.15l1.43.73-.37-1.24c1.02-.58 1.67-1.5 1.67-2.42zm-5.2-.53a.5.5 0 1 1 0-1 0.5 0 0 1 0 1zm2.62 0a.5.5 0 1 1 0-1 0.5 0 0 1 0 1z"/>`,
	},
	"blog": {
		viewBox: "0 0 16 16",
		body:    `<circle cx="8" cy="8" r="5.7" fill="none" stroke="currentColor" stroke-width="1.25"/><path d="M2.5 8h11M8 2.3c1.45 1.55 2.18 3.45 2.18 5.7S9.45 12.15 8 13.7C6.55 12.15 5.82 10.25 5.82 8S6.55 3.85 8 2.3z" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.15"/>`,
	},
}

func renderIcon(out *strings.Builder, name string) {
	glyph, ok := iconGlyphs[name]
	if !ok {
		return
	}
	fmt.Fprintf(out, `<svg class="inline-icon icon-%s" viewBox="%s" aria-hidden="true" focusable="false">%s</svg>`,
		name, glyph.viewBox, glyph.body)
}
