package sstv_test

import (
	"errors"
	"image"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/zaf/g711"

	"sstv2tg/internal/sstv"
	"sstv2tg/internal/sstv/sstvtest"
)

func TestRoundTrip(t *testing.T) {
	for _, m := range sstvtest.Modes {
		t.Run(m.Name, func(t *testing.T) {
			t.Parallel()
			card := m.Card()
			pcm := m.Encode(card)

			res, err := sstv.Decode(pcm)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if res.Mode.Name != m.Name {
				t.Fatalf("decoded as %s", res.Mode.Name)
			}
			if res.Img.Bounds() != card.Bounds() {
				t.Fatalf("bounds %v, want %v", res.Img.Bounds(), card.Bounds())
			}
			if got := sstvtest.MeanError(card, res.Img); got > 8 {
				t.Errorf("mean per-channel error %.2f, want <= 8", got)
			}
			got := time.Duration(len(pcm)) * time.Second / sstv.SampleRate
			if d := res.Mode.Duration - got; d < -time.Millisecond || d > time.Millisecond {
				t.Errorf("Duration %v, encoded audio is %v", res.Mode.Duration, got)
			}
		})
	}
}

// TestImpairedRoundTrip decodes each mode through what an acoustic call does
// to it: room echo, low level, the sender's clock running 200 ppm slow, two
// lost 20 ms RTP packets, noise at 20 dB SNR, spikes inside the VIS leader,
// and mu-law.
func TestImpairedRoundTrip(t *testing.T) {
	for _, m := range sstvtest.Modes {
		t.Run(m.Name, func(t *testing.T) {
			t.Parallel()
			card := m.Card()
			res, err := sstv.Decode(impair(m.Encode(card)))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if res.Img.Bounds() != card.Bounds() {
				t.Fatalf("bounds %v, want %v", res.Img.Bounds(), card.Bounds())
			}
			// Noise and echo ghosting cost up to about 25; losing line alignment costs 50+.
			if got := sstvtest.MeanError(card, res.Img); got > 30 {
				t.Errorf("mean per-channel error %.2f, want <= 30", got)
			}
		})
	}
}

func impair(clean []int16) []int16 {
	rng := rand.New(rand.NewPCG(1, 2))
	const (
		echoDelay = 80 // 10 ms
		echoGain  = 0.3
		gain      = 1000 / (0.7 * math.MaxInt16) // tone peak 1000, about -33 dBFS RMS
		drift     = 200e-6
		noise     = 70.0
	)

	room := make([]float64, len(clean))
	for i, s := range clean {
		room[i] = float64(s)
		if i >= echoDelay {
			room[i] += echoGain * float64(clean[i-echoDelay])
		}
	}

	var out []int16
	lost := map[int]bool{len(clean) * 2 / 5 / 160: true, len(clean) * 7 / 10 / 160: true}
	for i := 0; ; i++ {
		p := float64(i) * (1 - drift)
		j := int(p)
		if j+1 >= len(room) {
			break
		}
		if lost[j/160] {
			continue
		}
		v := (room[j]*(float64(j+1)-p) + room[j+1]*(p-float64(j))) * gain
		out = append(out, int16(v+rng.NormFloat64()*noise))
	}
	for range sstv.SampleRate / 2 {
		out = append(out, int16(rng.NormFloat64()*noise))
	}
	for _, k := range []int{400, 1100, 3300} { // inside the two leaders
		out[k] = math.MaxInt16
	}
	for i, s := range out {
		out[i] = g711.DecodeUlawFrame(g711.EncodeUlawFrame(s))
	}
	return out
}

func TestPartialDecodesWholeLines(t *testing.T) {
	card := sstvtest.Card(320, 240)
	pcm := sstvtest.Robot36.Encode(card)
	cut := int((0.910 + 100.5*0.150) * sstv.SampleRate)

	res, err := sstv.Decode(pcm[:cut])
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if h := res.Img.Bounds().Dy(); h != 100 {
		t.Fatalf("height %d, want 100", h)
	}
	if got := sstvtest.MeanError(card.SubImage(image.Rect(0, 0, 320, 100)), res.Img); got > 8 {
		t.Errorf("mean per-channel error %.2f, want <= 8", got)
	}
}

func TestHeaderErrors(t *testing.T) {
	if _, err := sstv.Decode(sstvtest.Header(8)); err == nil {
		t.Error("header with no lines: want error")
	}
	if _, err := sstv.Decode(sstvtest.Header(100)); err == nil || !strings.Contains(err.Error(), "unsupported VIS code 100") {
		t.Errorf("VIS 100: got %v", err)
	}
	if _, err := sstv.Decode(make([]int16, sstv.SampleRate)); !errors.Is(err, sstv.ErrNoHeader) {
		t.Errorf("silence: got %v, want ErrNoHeader", err)
	}
}

func TestFindHeader(t *testing.T) {
	hdr := sstvtest.Header(8)
	if _, _, err := sstv.FindHeader(hdr[:len(hdr)/2]); !errors.Is(err, sstv.ErrNoHeader) {
		t.Errorf("half a header: got %v, want ErrNoHeader", err)
	}

	pad := 4000
	m, start, err := sstv.FindHeader(append(make([]int16, pad), hdr...))
	if err != nil {
		t.Fatalf("FindHeader: %v", err)
	}
	if m.Name != "Robot 36" {
		t.Errorf("mode %s, want Robot 36", m.Name)
	}
	if d := start - pad; d < -8 || d > 8 {
		t.Errorf("leader at sample %d, want %d", start, pad)
	}
}

// TestHeaderThroughPhoneProcessing covers two things a speakerphone did to
// live headers: garbling the first milliseconds after each tone change,
// which swallows the 10 ms break, and a noise gate clipping the first leader.
func TestHeaderThroughPhoneProcessing(t *testing.T) {
	ms := func(t float64) int { return int(t * sstv.SampleRate / 1000) }
	for _, tc := range []struct {
		name   string
		damage func(pcm []int16)
	}{
		{"garbled break", func(pcm []int16) {
			tone(pcm[ms(300):ms(310)], 1450) // the break
			tone(pcm[ms(610):ms(618)], 1450) // the start of the start bit
		}},
		{"clipped first leader", func(pcm []int16) {
			clear(pcm[:ms(120)])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pcm := sstvtest.Robot36.Encode(sstvtest.Robot36.Card())
			tc.damage(pcm)
			res, err := sstv.Decode(pcm)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if res.Mode.Name != "Robot 36" || res.Img.Bounds().Dy() != 240 {
				t.Errorf("decoded %s, %d lines", res.Mode.Name, res.Img.Bounds().Dy())
			}
		})
	}
}

func tone(pcm []int16, freq float64) {
	for i := range pcm {
		pcm[i] = int16(0.7 * math.MaxInt16 * math.Sin(2*math.Pi*freq*float64(i)/sstv.SampleRate))
	}
}
