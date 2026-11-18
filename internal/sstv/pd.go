package sstv

import "image"

func init() {
	pd(93, "PD 50", 320, 256, 0.09152)
	pd(99, "PD 90", 320, 256, 0.17024)
	pd(95, "PD 120", 640, 496, 0.1216)
	pd(98, "PD 160", 512, 400, 0.195584)
	pd(96, "PD 180", 640, 496, 0.18304)
	pd(97, "PD 240", 640, 496, 0.24448)
	pd(94, "PD 290", 800, 616, 0.2288)
}

// PD line: 20 ms sync, 2.08 ms porch, then Y of the first row, R-Y and B-Y
// shared by both rows, and Y of the second row.
func pd(vis byte, name string, w, h int, scanDur float64) {
	const (
		sync  = 0.020
		porch = 0.00208
	)
	lineDur := sync + porch + 4*scanDur
	register(vis, name, 0, lineDur, h/2, func(freq []float64, t0 float64) *image.RGBA {
		starts := lineStarts(freq, t0, lineDur, 0, sync, h/2)
		img := image.NewRGBA(image.Rect(0, 0, w, 2*len(starts)))
		for i, start := range starts {
			y0 := start + sync + porch
			ya := scanLine(freq, y0, scanDur, w)
			cr := scanLine(freq, y0+scanDur, scanDur, w)
			cb := scanLine(freq, y0+2*scanDur, scanDur, w)
			yb := scanLine(freq, y0+3*scanDur, scanDur, w)
			for x := range w {
				img.SetRGBA(x, 2*i, ycbcr(ya[x], cb[x], cr[x]))
				img.SetRGBA(x, 2*i+1, ycbcr(yb[x], cb[x], cr[x]))
			}
		}
		return img
	})
}
