// Package callflow runs one answered call from beep to Telegram post.
package callflow

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"math"
	"time"

	"sstv2tg/internal/phone"
	"sstv2tg/internal/sstv"
)

const (
	scanWindow = 3 * sstv.SampleRate
	scanEvery  = sstv.SampleRate / 2
	// deadlineSlack is how long past the mode's length a transmission may run.
	deadlineSlack = 5 * time.Second
	postTimeout   = 30 * time.Second
)

// Sink posts the decoded PNG. telegram.Client satisfies it.
type Sink interface {
	SendPhoto(ctx context.Context, png []byte, caption string) error
}

type Options struct {
	// VISTimeout is how long to wait after the beep for a VIS header.
	VISTimeout time.Duration
	// SilenceTimeout ends a transmission that goes quiet partway.
	SilenceTimeout time.Duration
	// Caption goes on every posted picture.
	Caption string
	Sink    Sink
}

type summary struct {
	result, end, mode string
	lines             int
	heard             heard
}

// heard tracks how much caller audio arrived and how loud it got.
type heard struct {
	samples int
	peak    float64 // RMS of the loudest frame
}

func (h *heard) add(frame []int16) float64 {
	level := rms(frame)
	h.samples += len(frame)
	h.peak = max(h.peak, level)
	return level
}

// Handle plays the ready beep, records the caller's SSTV transmission,
// decodes it and posts the picture. A transmission cut short posts the lines
// that arrived.
func Handle(ctx context.Context, c *phone.Call, opt Options) {
	start := time.Now()
	s := run(ctx, c, opt)
	c.Log().Info("call finished",
		"result", s.result,
		"end", s.end,
		"mode", s.mode,
		"lines", s.lines,
		"heard", (time.Duration(s.heard.samples) * time.Second / sstv.SampleRate).Round(100*time.Millisecond),
		"peak_dbfs", math.Round(20*math.Log10(s.heard.peak/math.MaxInt16)*10)/10,
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

func run(ctx context.Context, c *phone.Call, opt Options) summary {
	if err := c.Play(ctx, beep()); err != nil {
		return summary{result: "hangup during beep"}
	}

	s := summary{result: "no image"}
	pcm, m, end := collect(ctx, c, opt, &s.heard)
	s.end = end
	if m == nil {
		return s
	}
	s.mode = m.Name

	res, err := sstv.Decode(pcm)
	if err != nil {
		s.result = "decode failed: " + err.Error()
		return s
	}
	s.lines = res.Img.Bounds().Dy()

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, res.Img); err != nil {
		s.result = "png failed: " + err.Error()
		return s
	}

	postCtx, cancel := context.WithTimeout(context.Background(), postTimeout)
	defer cancel()
	if err := opt.Sink.SendPhoto(postCtx, buf.Bytes(), opt.Caption); err != nil {
		s.result = "post failed: " + err.Error()
		return s
	}
	s.result = "posted"
	return s
}

// collect scans the caller's audio for a VIS header, then records from its
// leader until the mode's length is in hand. It returns the recording, the
// mode (nil when no transmission started), and why recording stopped.
func collect(ctx context.Context, c *phone.Call, opt Options, h *heard) ([]int16, *sstv.Mode, string) {
	deadline := time.NewTimer(opt.VISTimeout)
	defer deadline.Stop()

	var scan, pcm []int16
	var m *sstv.Mode
	var pending int
	var quiet time.Duration
	for {
		var frame []int16
		select {
		case <-ctx.Done():
			return pcm, m, "hangup"
		case <-deadline.C:
			if m == nil {
				return nil, nil, "no VIS header"
			}
			return pcm, m, "deadline"
		case f, ok := <-c.Audio():
			if !ok {
				return pcm, m, "hangup"
			}
			frame = f
		}
		level := h.add(frame)

		if m == nil {
			scan = append(scan, frame...)
			if len(scan) > scanWindow {
				scan = scan[len(scan)-scanWindow:]
			}
			if pending += len(frame); pending < scanEvery {
				continue
			}
			pending = 0

			mode, start, err := sstv.FindHeader(scan)
			if errors.Is(err, sstv.ErrNoHeader) {
				continue
			}
			if err != nil {
				return nil, nil, err.Error()
			}
			m, pcm, scan = mode, scan[start:], nil
			deadline.Reset(m.Duration + deadlineSlack)
			c.Log().Info("VIS header", "mode", m.Name)
			continue
		}

		pcm = append(pcm, frame...)
		if time.Duration(len(pcm))*time.Second/sstv.SampleRate >= m.Duration {
			return pcm, m, "complete"
		}
		if level >= silenceRMS {
			quiet = 0
			continue
		}
		if quiet += time.Duration(len(frame)) * time.Second / sstv.SampleRate; quiet >= opt.SilenceTimeout {
			return pcm, m, "silence"
		}
	}
}

// silenceRMS is -44 dBFS, far under any SSTV tone.
const silenceRMS = 200

func rms(frame []int16) float64 {
	var sum float64
	for _, s := range frame {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(frame)))
}

// beep is 1 s of silence, since audio sent while the media path through the
// PBX is still coming up after answer is lost, then 250 ms of 1 kHz.
func beep() []int16 {
	const settle = sstv.SampleRate
	out := make([]int16, settle+sstv.SampleRate/4)
	for i := range out[settle:] {
		out[settle+i] = int16(0.3 * math.MaxInt16 * math.Sin(2*math.Pi*1000*float64(i)/sstv.SampleRate))
	}
	return out
}
