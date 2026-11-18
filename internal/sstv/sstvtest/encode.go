// Package sstvtest encodes test images in every supported SSTV mode. The
// encoders are tone sequences written from the mode specs, not from the
// decoders, so a round trip catches decoder timing mistakes.
package sstvtest

import (
	"image"
	"math"
)

const (
	rate      = 8000
	amplitude = 0.7 * math.MaxInt16
)

// Mode encodes images in one SSTV mode.
type Mode struct {
	Name          string
	VIS           byte
	Width, Height int
	lines         func(s *synth, img image.Image)
	grey          bool
}

var Robot36 = Mode{"Robot 36", 8, 320, 240, robot36, false}

var Modes = []Mode{
	Robot36,
	{"Robot 72", 12, 320, 240, robotFull(0.009, 0.003, 0.138, 0.0045, 0.0015, 0.069), false},
	{"Robot 24", 4, 160, 120, robotFull(0.006, 0.002, 0.092, 0.003, 0.001, 0.046), false},
	{"Robot 8 B/W", 2, 160, 120, robotBW(0.0669), true},
	{"Robot 12 B/W", 6, 160, 120, robotBW(0.100), true},
	{"Robot 24 B/W", 10, 320, 240, robotBW(0.100), true},
	{"Martin M1", 44, 320, 256, martin(0.146432), false},
	{"Martin M2", 40, 320, 256, martin(0.073216), false},
	{"Scottie S1", 60, 320, 256, scottie(0.138240), false},
	{"Scottie S2", 56, 320, 256, scottie(0.088064), false},
	{"Scottie DX", 76, 320, 256, scottie(0.3456), false},
	{"Wraase SC2-180", 55, 320, 256, wraase(0.235), false},
	{"Wraase SC2-120", 63, 320, 256, wraase(0.1565), false},
	{"PD 50", 93, 320, 256, pd(0.09152), false},
	{"PD 90", 99, 320, 256, pd(0.17024), false},
	{"PD 120", 95, 640, 496, pd(0.1216), false},
	{"PD 160", 98, 512, 400, pd(0.195584), false},
	{"PD 180", 96, 640, 496, pd(0.18304), false},
	{"PD 240", 97, 640, 496, pd(0.24448), false},
	{"PD 290", 94, 800, 616, pd(0.2288), false},
	{"Pasokon P3", 113, 640, 496, pasokon(1.0 / 4800), false},
	{"Pasokon P5", 114, 640, 496, pasokon(1.0 / 3200), false},
	{"Pasokon P7", 115, 640, 496, pasokon(1.0 / 2400), false},
}

// Card is the test card in this mode's size, in grey for black-and-white modes.
func (m Mode) Card() *image.RGBA {
	card := Card(m.Width, m.Height)
	if !m.grey {
		return card
	}
	for i := 0; i < len(card.Pix); i += 4 {
		l := uint8(0.299*float64(card.Pix[i]) + 0.587*float64(card.Pix[i+1]) + 0.114*float64(card.Pix[i+2]))
		card.Pix[i], card.Pix[i+1], card.Pix[i+2] = l, l, l
	}
	return card
}

// Encode returns img, which must be Width x Height, as 8 kHz PCM starting
// with the VIS header.
func (m Mode) Encode(img image.Image) []int16 {
	s := &synth{}
	s.vis(m.VIS)
	m.lines(s, img)
	return s.out
}

// Header returns a VIS header carrying code and nothing after it.
func Header(code byte) []int16 {
	s := &synth{}
	s.vis(code)
	return s.out
}

func robot36(s *synth, img image.Image) {
	y, cb, cr := toYCbCr(img)
	for row := range y {
		s.tone(1200, 0.009)
		s.tone(1500, 0.003)
		s.levels(y[row], 0.088)

		chroma, sep := cr, 1500.0
		if row%2 == 1 {
			chroma, sep = cb, 2300.0
		}
		s.tone(sep, 0.0045)
		s.tone(1900, 0.0015)
		s.levels(halve(mean(chroma[row&^1], chroma[row|1])), 0.044)
	}
}

// robotFull sends Y, R-Y and B-Y on every line, each chroma scan after a
// separator and porch.
func robotFull(sync, porch, y, sep, porch2, chroma float64) func(*synth, image.Image) {
	return func(s *synth, img image.Image) {
		yy, cb, cr := toYCbCr(img)
		for row := range yy {
			s.tone(1200, sync)
			s.tone(1500, porch)
			s.levels(yy[row], y)
			s.tone(1500, sep)
			s.tone(1900, porch2)
			s.levels(halve(cr[row]), chroma)
			s.tone(1500, sep)
			s.tone(1900, porch2)
			s.levels(halve(cb[row]), chroma)
		}
	}
}

// robotBW sends a 7 ms sync, then luminance for the rest of the line.
func robotBW(lineDur float64) func(*synth, image.Image) {
	return func(s *synth, img image.Image) {
		r, g, b := toRGB(img)
		for row := range r {
			s.tone(1200, 0.007)
			l := make([]float64, len(r[row]))
			for x := range l {
				l[x] = 0.299*r[row][x] + 0.587*g[row][x] + 0.114*b[row][x]
			}
			s.levels(l, lineDur-0.007)
		}
	}
}

func martin(scan float64) func(*synth, image.Image) {
	return func(s *synth, img image.Image) {
		r, g, b := toRGB(img)
		for row := range r {
			s.tone(1200, 0.004862)
			s.tone(1500, 0.000572)
			for _, ch := range [][]float64{g[row], b[row], r[row]} {
				s.levels(ch, scan)
				s.tone(1500, 0.000572)
			}
		}
	}
}

func scottie(scan float64) func(*synth, image.Image) {
	return func(s *synth, img image.Image) {
		r, g, b := toRGB(img)
		s.tone(1200, 0.009)
		for row := range r {
			s.tone(1500, 0.0015)
			s.levels(g[row], scan)
			s.tone(1500, 0.0015)
			s.levels(b[row], scan)
			s.tone(1200, 0.009)
			s.tone(1500, 0.0015)
			s.levels(r[row], scan)
		}
	}
}

func wraase(scan float64) func(*synth, image.Image) {
	return func(s *synth, img image.Image) {
		r, g, b := toRGB(img)
		for row := range r {
			s.tone(1200, 0.0055225)
			s.tone(1500, 0.0005)
			s.levels(r[row], scan)
			s.levels(g[row], scan)
			s.levels(b[row], scan)
		}
	}
}

func pd(scan float64) func(*synth, image.Image) {
	return func(s *synth, img image.Image) {
		y, cb, cr := toYCbCr(img)
		for row := 0; row < len(y); row += 2 {
			s.tone(1200, 0.020)
			s.tone(1500, 0.00208)
			s.levels(y[row], scan)
			s.levels(mean(cr[row], cr[row+1]), scan)
			s.levels(mean(cb[row], cb[row+1]), scan)
			s.levels(y[row+1], scan)
		}
	}
}

// pasokon timings are multiples of unit: 25 sync, 5 per gap, 640 per scan.
func pasokon(unit float64) func(*synth, image.Image) {
	return func(s *synth, img image.Image) {
		r, g, b := toRGB(img)
		for row := range r {
			s.tone(1200, 25*unit)
			for _, ch := range [][]float64{r[row], g[row], b[row]} {
				s.tone(1500, 5*unit)
				s.levels(ch, 640*unit)
			}
			s.tone(1500, 5*unit)
		}
	}
}

// toYCbCr converts to BT.601 studio-swing YCbCr planes.
func toYCbCr(img image.Image) (y, cb, cr [][]float64) {
	r, g, b := toRGB(img)
	y, cb, cr = like(r), like(r), like(r)
	for row := range r {
		for x := range r[row] {
			rr, gg, bb := r[row][x]/255, g[row][x]/255, b[row][x]/255
			y[row][x] = 16 + 65.481*rr + 128.553*gg + 24.966*bb
			cb[row][x] = 128 - 37.797*rr - 74.203*gg + 112.0*bb
			cr[row][x] = 128 + 112.0*rr - 93.786*gg - 18.214*bb
		}
	}
	return y, cb, cr
}

// toRGB splits img into 0-255 channel planes.
func toRGB(img image.Image) (r, g, b [][]float64) {
	bounds := img.Bounds()
	for row := bounds.Min.Y; row < bounds.Max.Y; row++ {
		var rr, gg, bb []float64
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, row).RGBA()
			rr, gg, bb = append(rr, float64(cr>>8)), append(gg, float64(cg>>8)), append(bb, float64(cb>>8))
		}
		r, g, b = append(r, rr), append(g, gg), append(b, bb)
	}
	return r, g, b
}

func like(p [][]float64) [][]float64 {
	out := make([][]float64, len(p))
	for i := range out {
		out[i] = make([]float64, len(p[i]))
	}
	return out
}

func mean(a, b []float64) []float64 {
	out := make([]float64, len(a))
	for i := range out {
		out[i] = (a[i] + b[i]) / 2
	}
	return out
}

func halve(v []float64) []float64 {
	out := make([]float64, len(v)/2)
	for i := range out {
		out[i] = (v[2*i] + v[2*i+1]) / 2
	}
	return out
}

type synth struct {
	out   []int16
	phase float64
	t     float64
}

// tone appends a phase-continuous sine. Durations accumulate as exact time so
// sub-sample pixel lengths do not drift the line timing.
func (s *synth) tone(freq, dur float64) {
	s.t += dur
	end := int(math.Round(s.t * rate))
	step := 2 * math.Pi * freq / rate
	for len(s.out) < end {
		s.out = append(s.out, int16(amplitude*math.Sin(s.phase)))
		s.phase += step
	}
	s.phase = math.Mod(s.phase, 2*math.Pi)
}

// levels sends 0-255 pixel levels spread evenly over dur seconds.
func (s *synth) levels(v []float64, dur float64) {
	for _, l := range v {
		s.tone(1500+800*l/255, dur/float64(len(v)))
	}
}

// vis sends the leader, break, start bit, seven data bits LSB first, even
// parity, and stop bit.
func (s *synth) vis(code byte) {
	s.tone(1900, 0.300)
	s.tone(1200, 0.010)
	s.tone(1900, 0.300)
	s.tone(1200, 0.030)

	var parity byte
	for i := range 7 {
		bit := code >> i & 1
		parity ^= bit
		s.bit(bit)
	}
	s.bit(parity)
	s.tone(1200, 0.030)
}

func (s *synth) bit(b byte) {
	f := 1300.0
	if b == 1 {
		f = 1100
	}
	s.tone(f, 0.030)
}
