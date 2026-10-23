package ui

import "github.com/egoist/mygo/ui"

// icon parses a 24×24 stroked icon drawn in the current text color, as
// common icon sets draw them.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	overviewIcon = icon(`<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>`)
	updatesIcon  = icon(`<path d="M12 3v12M7 10l5 5 5-5"/><path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2"/>`)
	aboutIcon    = icon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>`)
	refreshIcon  = icon(`<path d="M20 11a8 8 0 1 0-2.3 5.7M20 5v6h-6"/>`)
	downloadIcon = icon(`<path d="M12 4v10M8 11l4 4 4-4"/><path d="M5 19h14"/>`)
	checkIcon    = icon(`<path d="M20 6 9 17l-5-5"/>`)
	linkIcon     = icon(`<path d="M14 4h6v6M20 4l-9 9"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>`)
)
