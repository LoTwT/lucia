package ui

import "github.com/egoist/mygo/ui"

// icon parses a 24×24 stroked icon drawn in the current text color, as
// common icon sets draw them.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	petIcon   = icon(`<circle cx="7" cy="7" r="1.4"/><circle cx="12" cy="5.5" r="1.4"/><circle cx="17" cy="7" r="1.4"/><path d="M8 15a4 4 0 0 1 8 0c0 2.2-1.8 4-4 4s-4-1.8-4-4Z"/>`)
	aboutIcon = icon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>`)
	checkIcon = icon(`<path d="M20 6 9 17l-5-5"/>`)
)
