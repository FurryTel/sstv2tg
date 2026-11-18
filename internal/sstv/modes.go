// Package sstv decodes slow-scan television audio into images.
package sstv

import (
	"image"
	"time"
)

const SampleRate = 8000

// Mode is one supported SSTV mode.
type Mode struct {
	VIS  byte
	Name string
	// Duration runs from the start of the VIS leader to the end of the last line.
	Duration time.Duration
	lead     float64
	decode   func(freq []float64, t0 float64) *image.RGBA
}

var modes = map[byte]*Mode{}

// register adds a mode whose first line starts lead seconds after the VIS
// header. decode gets the time that first line starts.
func register(vis byte, name string, lead, lineDur float64, lines int, decode func(freq []float64, t0 float64) *image.RGBA) {
	d := visDur + lead + lineDur*float64(lines)
	modes[vis] = &Mode{
		VIS:      vis,
		Name:     name,
		Duration: time.Duration(d * float64(time.Second)),
		lead:     lead,
		decode:   decode,
	}
}
