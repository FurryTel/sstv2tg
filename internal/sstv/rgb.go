package sstv

import (
	"image"
	"strings"
)

func init() {
	// Martin: green, blue, red, with a separator after each.
	martin := rgbLine{sync: 0.004862, porch: 0.000572, gap: 0.000572, tail: 0.000572, order: "gbr"}
	martin.register(44, "Martin M1", 320, 256, 0.146432)
	martin.register(40, "Martin M2", 320, 256, 0.073216)

	// Pasokon: timings are whole multiples of a per-mode unit.
	for _, p := range []struct {
		vis  byte
		name string
		unit float64
	}{
		{113, "Pasokon P3", 1.0 / 4800},
		{114, "Pasokon P5", 1.0 / 3200},
		{115, "Pasokon P7", 1.0 / 2400},
	} {
		u := p.unit
		line := rgbLine{sync: 25 * u, porch: 5 * u, gap: 5 * u, tail: 5 * u, order: "rgb"}
		line.register(p.vis, p.name, 640, 496, 640*u)
	}

	// Wraase SC2: red, green, blue back to back.
	wraase := rgbLine{sync: 0.0055225, porch: 0.0005, order: "rgb"}
	wraase.register(55, "Wraase SC2-180", 320, 256, 0.235)
	wraase.register(63, "Wraase SC2-120", 320, 256, 0.1565)
}

// rgbLine is a line of sync, porch, then three colour scans in order with
// gap between them and tail after the last.
type rgbLine struct {
	sync, porch, gap, tail float64
	order                  string
}

func (l rgbLine) register(vis byte, name string, w, h int, scanDur float64) {
	lineDur := l.sync + l.porch + 3*scanDur + 2*l.gap + l.tail
	register(vis, name, 0, lineDur, h, func(freq []float64, t0 float64) *image.RGBA {
		starts := lineStarts(freq, t0, lineDur, 0, l.sync, h)
		img := image.NewRGBA(image.Rect(0, 0, w, len(starts)))
		for row, start := range starts {
			var ch [3][]float64 // red, green, blue
			t := start + l.sync + l.porch
			for _, c := range l.order {
				ch[strings.IndexRune("rgb", c)] = scanLine(freq, t, scanDur, w)
				t += scanDur + l.gap
			}
			for x := range w {
				img.SetRGBA(x, row, rgb(ch[0][x], ch[1][x], ch[2][x]))
			}
		}
		return img
	})
}
