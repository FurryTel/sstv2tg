package sstvtest

import (
	"image"
	"image/color"
	"math"
)

// Card draws colour bars fading from black at the top, enough to catch a
// channel swap, a chroma-line offset, or timing drift down the image.
func Card(w, h int) *image.RGBA {
	bars := []color.RGBA{
		{255, 255, 255, 255},
		{255, 255, 0, 255},
		{0, 255, 255, 255},
		{0, 255, 0, 255},
		{255, 0, 255, 255},
		{255, 0, 0, 255},
		{0, 0, 255, 255},
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		shade := float64(y) / float64(h)
		for x := range w {
			c := bars[x*len(bars)/w]
			img.SetRGBA(x, y, color.RGBA{
				uint8(float64(c.R) * shade),
				uint8(float64(c.G) * shade),
				uint8(float64(c.B) * shade),
				255,
			})
		}
	}
	return img
}

// MeanError is the mean absolute per-channel difference over a's bounds, 0-255.
func MeanError(a, b image.Image) float64 {
	bounds := a.Bounds()
	var sum float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			sum += (math.Abs(float64(ar>>8)-float64(br>>8)) +
				math.Abs(float64(ag>>8)-float64(bg>>8)) +
				math.Abs(float64(ab>>8)-float64(bb>>8))) / 3
		}
	}
	return sum / float64(bounds.Dx()*bounds.Dy())
}
