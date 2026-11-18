package sstv

import (
	"image/color"
	"math"
	"math/cmplx"
)

const (
	center  = 1900.0
	firTaps = 31
)

// demod returns the instantaneous frequency and envelope at each sample.
func demod(pcm []int16) (freq, mag []float64) {
	n := len(pcm)
	mixed := make([]complex128, n)
	for i, s := range pcm {
		mixed[i] = complex(float64(s), 0) * cmplx.Rect(1, -2*math.Pi*center*float64(i)/SampleRate)
	}

	taps := lowpass(firTaps, 1100)
	half := firTaps / 2
	filtered := make([]complex128, n)
	for i := range filtered {
		var acc complex128
		for k, h := range taps {
			j := i + k - half
			if j < 0 || j >= n {
				continue
			}
			acc += mixed[j] * complex(h, 0)
		}
		filtered[i] = acc
	}

	freq, mag = make([]float64, n), make([]float64, n)
	for i := 1; i < n; i++ {
		d := cmplx.Phase(filtered[i] * cmplx.Conj(filtered[i-1]))
		freq[i] = center + d*SampleRate/(2*math.Pi)
		mag[i] = cmplx.Abs(filtered[i])
	}
	if n > 1 {
		freq[0] = freq[1]
	}
	return freq, mag
}

func lowpass(n int, cutoff float64) []float64 {
	h := make([]float64, n)
	fc := cutoff / SampleRate
	m := float64(n - 1)
	var sum float64
	for i := range h {
		x := float64(i) - m/2
		v := 2 * fc
		if x != 0 {
			v = math.Sin(2*math.Pi*fc*x) / (math.Pi * x)
		}
		h[i] = v * (0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/m))
		sum += h[i]
	}
	for i := range h {
		h[i] /= sum
	}
	return h
}

// at interpolates freq at t seconds.
func at(freq []float64, t float64) float64 {
	p := t * SampleRate
	i := int(p)
	if i+1 >= len(freq) {
		return freq[len(freq)-1]
	}
	f := p - float64(i)
	return freq[i]*(1-f) + freq[i+1]*f
}

// scanLine samples n pixel levels spread evenly over dur seconds from t.
func scanLine(freq []float64, t, dur float64, n int) []float64 {
	px := make([]float64, n)
	for x := range px {
		px[x] = (at(freq, t+(float64(x)+0.5)*dur/float64(n)) - 1500) * 255 / 800
	}
	return px
}

func rgb(r, g, b float64) color.RGBA {
	return color.RGBA{clamp(r), clamp(g), clamp(b), 255}
}

// ycbcr converts BT.601 studio-swing YCbCr, which SSTV uses, to RGB.
func ycbcr(y, cb, cr float64) color.RGBA {
	y, cb, cr = 1.164*(y-16), cb-128, cr-128
	return rgb(y+1.596*cr, y-0.392*cb-0.813*cr, y+2.017*cb)
}

func clamp(v float64) uint8 {
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}
