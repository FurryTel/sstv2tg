package sstv

import "math"

const (
	syncFreq = 1200.0
	// lineSlack lets the last line count as whole when the recording ends a
	// few samples short of it.
	lineSlack = 0.005
)

// lineStarts returns the start time of each whole line in the recording, at
// most limit. Each line's sync pulse, syncOff seconds into the line and
// syncDur long, is located near where the previous lines predict it, so the
// sender's clock drift and the jumps lost RTP packets cause are followed
// rather than smeared across the rest of the image. A pulse missing from
// there is looked for up to half a line earlier, since a PSTN leg can drop
// runs of packets longer than the window. Room echo makes the
// track swing sample to sample, so pulses are found on a 1 ms running median.
func lineStarts(freq []float64, t0, lineDur, syncOff, syncDur float64, limit int) []float64 {
	hits := make([]int, len(freq)+1)
	for i := range freq {
		hits[i+1] = hits[i]
		if math.Abs(median(freq[max(0, i-4):min(len(freq), i+5)])-syncFreq) < 150 {
			hits[i+1]++
		}
	}
	window := int(min(0.050, 0.4*lineDur) * SampleRate)
	width := int(syncDur * SampleRate)
	back := int(0.5 * lineDur * SampleRate)

	var starts []float64
	var shift float64
	for n := range limit {
		nominal := t0 + float64(n)*lineDur
		center := int((nominal + shift + syncOff) * SampleRate)
		p, ok := findSync(hits, center, window, width)
		if !ok {
			p, ok = findSync(hits, center-back/2, back/2, width)
		}
		if ok {
			shift = float64(p)/SampleRate - syncOff - nominal
		}
		start := nominal + shift
		if (start+lineDur-lineSlack)*SampleRate > float64(len(freq)) {
			break
		}
		starts = append(starts, start)
	}
	return starts
}

// findSync returns the sample within window of center where a sync pulse
// width samples long fits best, preferring the one nearest center. It fails
// when no position is at least 70% sync tone.
func findSync(hits []int, center, window, width int) (int, bool) {
	best, bestScore := 0, -1
	for d := 0; d <= window; d++ {
		for _, p := range []int{center - d, center + d} {
			if p < 0 || p+width >= len(hits) {
				continue
			}
			if score := hits[p+width] - hits[p]; score > bestScore {
				best, bestScore = p, score
			}
		}
	}
	return best, bestScore*10 >= width*7
}
