package sstv

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

const (
	visLeader = 1900.0
	visBreak  = 1200.0
	visBit    = 0.030
	visBits   = 0.640 // leader, break, leader, start bit
	visDur    = 0.910 // through the stop bit
)

// ErrNoHeader means the audio holds no complete VIS header.
var ErrNoHeader = errors.New("no VIS header")

// header finds the first complete VIS header and returns its mode and the
// time its leader starts.
func header(freq, mag []float64) (*Mode, float64, error) {
	t0, ok := findStart(freq, mag)
	if !ok {
		return nil, 0, ErrNoHeader
	}

	// Seven data bits LSB first, then even parity: all eight XOR to zero.
	var code, parity byte
	for i := range 8 {
		f, _ := bitSlot(freq, t0, i)
		var bit byte
		if f < 1200 {
			bit = 1
		}
		parity ^= bit
		code |= bit << i
	}
	code &= 0x7f
	if parity != 0 {
		return nil, 0, errors.New("VIS parity mismatch")
	}

	m, ok := modes[code]
	if !ok {
		return nil, 0, fmt.Errorf("unsupported VIS code %d", code)
	}
	return m, t0, nil
}

// findStart returns the time a complete VIS header's first leader starts.
// Candidates are 1900 Hz runs of 250 ms or more, forgiving up to 5 ms of stray
// samples, since acoustic coupling and G.711 put noise spikes in the
// frequency track. Echo and a phone's processing garble the first few
// milliseconds after every tone change, which can swallow the 10 ms break,
// so a candidate is judged instead on its start and stop bits sitting at
// 1200 Hz. A phone's noise gate can clip the first leader, so a run is tried
// as either leader.
func findStart(freq, mag []float64) (float64, bool) {
	const (
		minMag = 100 // about -44 dBFS; silence demodulates to exactly 1900 Hz
		maxGap = 40
	)
	leader := int(0.250 * SampleRate)
	run, gap := 0, 0
	for i, f := range freq {
		if math.Abs(f-visLeader) < 50 && mag[i] > minMag {
			run += 1 + gap
			gap = 0
			continue
		}
		if gap++; gap > maxGap {
			run, gap = 0, 0
			continue
		}
		if gap > 1 || run < leader {
			continue
		}

		end, ok := leaderEnd(freq, i)
		if !ok {
			continue
		}
		for _, t0 := range []float64{end - 0.300, end - 0.610} {
			start, okStart := bitSlot(freq, t0, -1)
			stop, okStop := bitSlot(freq, t0, 8)
			if okStart && okStop && math.Abs(start-visBreak) < 50 && math.Abs(stop-visBreak) < 50 {
				return t0, true
			}
		}
	}
	return 0, false
}

// leaderEnd returns when the leader ending around sample i drops to the
// break. The filter smears tone edges, so it is the point where the track
// crosses midway between the two tones.
func leaderEnd(freq []float64, i int) (float64, bool) {
	const mid = (visLeader + visBreak) / 2
	j := max(i, 1)
	for j < len(freq) && freq[j] > mid {
		j++
	}
	if j >= len(freq) {
		return 0, false
	}
	// Interpolate the crossing; the phase-difference estimate lags half a sample.
	edge := float64(j-1) + (freq[j-1]-mid)/(freq[j-1]-freq[j]) - 0.5
	return edge / SampleRate, true
}

// bitSlot returns the mean frequency over the middle 20 ms of VIS bit k of a
// header starting at t0: -1 is the start bit, 0-6 data, 7 parity, 8 stop. It
// fails when the slot is not wholly inside the recording.
func bitSlot(freq []float64, t0 float64, k int) (float64, bool) {
	s := t0 + visBits + float64(k)*visBit
	if s < 0 || (s+0.025)*SampleRate > float64(len(freq)) {
		return 0, false
	}
	return mean(freq, s+0.005, s+0.025), true
}

func median(fs []float64) float64 {
	s := slices.Clone(fs)
	slices.Sort(s)
	return s[len(s)/2]
}

// mean averages freq over [from, to) seconds.
func mean(freq []float64, from, to float64) float64 {
	a, b := int(from*SampleRate), int(to*SampleRate)
	var sum float64
	for _, f := range freq[a:b] {
		sum += f
	}
	return sum / float64(b-a)
}
