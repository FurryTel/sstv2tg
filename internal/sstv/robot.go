package sstv

import "image"

// Robot 36 line: 9 ms sync, 3 ms porch, 88 ms Y, 4.5 ms separator, 1.5 ms
// porch, 44 ms chroma. Even lines carry R-Y and odd lines B-Y for the pair.
func init() {
	const (
		w, h     = 320, 240
		lineDur  = 0.150
		sync     = 0.009
		yStart   = 0.012
		yDur     = 0.088
		chrStart = 0.106
		chrDur   = 0.044
	)
	register(8, "Robot 36", 0, lineDur, h, func(freq []float64, t0 float64) *image.RGBA {
		starts := lineStarts(freq, t0, lineDur, 0, sync, h)
		n := len(starts) &^ 1
		img := image.NewRGBA(image.Rect(0, 0, w, n))
		for top := 0; top < n; top += 2 {
			cr := scanLine(freq, starts[top]+chrStart, chrDur, w/2)
			cb := scanLine(freq, starts[top+1]+chrStart, chrDur, w/2)
			for row := top; row < top+2; row++ {
				y := scanLine(freq, starts[row]+yStart, yDur, w)
				for x := range w {
					img.SetRGBA(x, row, ycbcr(y[x], cb[x/2], cr[x/2]))
				}
			}
		}
		return img
	})
}

func init() {
	robotFullChroma(12, "Robot 72", 320, 240, 1)
	robotFullChroma(4, "Robot 24", 160, 120, 2.0/3)

	robotBW(2, "Robot 8 B/W", 160, 120, 0.0669)
	robotBW(6, "Robot 12 B/W", 160, 120, 0.100)
	robotBW(10, "Robot 24 B/W", 320, 240, 0.100)
}

// robotFullChroma registers a mode whose every line carries both chroma
// channels. Robot 72's line, which the other is scale times: 9 ms sync, 3 ms
// porch, 138 ms Y, then R-Y and B-Y of 69 ms, each after a 4.5 ms separator
// and 1.5 ms porch.
func robotFullChroma(vis byte, name string, w, h int, scale float64) {
	lineDur := 0.300 * scale
	sync := 0.009 * scale
	yStart, yDur := 0.012*scale, 0.138*scale
	crStart, cbStart, chrDur := 0.156*scale, 0.231*scale, 0.069*scale
	register(vis, name, 0, lineDur, h, func(freq []float64, t0 float64) *image.RGBA {
		starts := lineStarts(freq, t0, lineDur, 0, sync, h)
		img := image.NewRGBA(image.Rect(0, 0, w, len(starts)))
		for row, start := range starts {
			y := scanLine(freq, start+yStart, yDur, w)
			cr := scanLine(freq, start+crStart, chrDur, w/2)
			cb := scanLine(freq, start+cbStart, chrDur, w/2)
			for x := range w {
				img.SetRGBA(x, row, ycbcr(y[x], cb[x/2], cr[x/2]))
			}
		}
		return img
	})
}

// robotBW registers a black-and-white mode: a 7 ms sync, then luminance for
// the rest of the line, one line per image row.
func robotBW(vis byte, name string, w, h int, lineDur float64) {
	const sync = 0.007
	register(vis, name, 0, lineDur, h, func(freq []float64, t0 float64) *image.RGBA {
		starts := lineStarts(freq, t0, lineDur, 0, sync, h)
		img := image.NewRGBA(image.Rect(0, 0, w, len(starts)))
		for row, start := range starts {
			y := scanLine(freq, start+sync, lineDur-sync, w)
			for x := range w {
				img.SetRGBA(x, row, rgb(y[x], y[x], y[x]))
			}
		}
		return img
	})
}
