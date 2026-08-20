package theme

import (
	"image/color"
	"math"
)

// Alphas the design applies to a tone when filling a chip or an active tab.
// The design writes these as an eight-bit suffix on a hex colour — #4a9eea18
// is the agent tone at nine percent — which a terminal cell cannot express, so
// the composite is computed against the surface behind it instead.
const (
	ChipAlpha = 0x18 / 255.0 // state chip background
	TabAlpha  = 0x14 / 255.0 // active mode tab background
	FillAlpha = 0.75         // the context meter's fill
)

// Blend mixes fg over bg at the given alpha, where 0 is fully transparent and
// 1 fully opaque. Values outside that range are clamped.
func Blend(fg, bg color.Color, alpha float64) color.Color {
	alpha = min(max(alpha, 0), 1)

	fr, fgr, fb, _ := fg.RGBA()
	br, bgr, bb, _ := bg.RGBA()

	mix := func(f, b uint32) uint8 {
		// RGBA returns 16-bit channels; shift down before mixing.
		v := float64(f>>8)*alpha + float64(b>>8)*(1-alpha)
		return uint8(math.Round(min(max(v, 0), 255)))
	}
	return color.RGBA{R: mix(fr, br), G: mix(fgr, bgr), B: mix(fb, bb), A: 0xff}
}
