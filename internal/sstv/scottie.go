package sstv

import "image"

func init() {
	scottie(60, "Scottie S1", 0.138240)
	scottie(56, "Scottie S2", 0.088064)
	scottie(76, "Scottie DX", 0.3456)
}

// Scottie sends one sync pulse before the first line. Each line is then:
// separator, green, separator, blue, sync, porch, red.
func scottie(vis byte, name string, scanDur float64) {
	const (
		w, h = 320, 256
		sync = 0.009
		gap  = 0.0015
	)
	lineDur := 3*scanDur + 3*gap + sync
	syncOff := 2 * (gap + scanDur)
	register(vis, name, sync, lineDur, h, func(freq []float64, t0 float64) *image.RGBA {
		starts := lineStarts(freq, t0, lineDur, syncOff, sync, h)
		img := image.NewRGBA(image.Rect(0, 0, w, len(starts)))
		for row, start := range starts {
			g := scanLine(freq, start+gap, scanDur, w)
			b := scanLine(freq, start+2*gap+scanDur, scanDur, w)
			r := scanLine(freq, start+syncOff+sync+gap, scanDur, w)
			for x := range w {
				img.SetRGBA(x, row, rgb(r[x], g[x], b[x]))
			}
		}
		return img
	})
}
