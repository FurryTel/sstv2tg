package sstv

import (
	"errors"
	"image"
)

// Result is a decoded image and the mode it was sent in.
type Result struct {
	Mode *Mode
	Img  *image.RGBA
}

// FindHeader looks for a complete VIS header in pcm and returns its mode and
// the sample where its leader starts. It returns ErrNoHeader until a whole
// header is present.
func FindHeader(pcm []int16) (*Mode, int, error) {
	m, t0, err := header(demod(pcm))
	if err != nil {
		return nil, 0, err
	}
	return m, max(0, int(t0*SampleRate)), nil
}

// Decode decodes the image after the first VIS header in pcm. A recording cut
// short decodes to the whole lines it holds.
func Decode(pcm []int16) (*Result, error) {
	freq, mag := demod(pcm)
	m, t0, err := header(freq, mag)
	if err != nil {
		return nil, err
	}
	img := m.decode(freq, t0+visDur+m.lead)
	if img.Bounds().Empty() {
		return nil, errors.New("no complete image lines")
	}
	return &Result{Mode: m, Img: img}, nil
}
