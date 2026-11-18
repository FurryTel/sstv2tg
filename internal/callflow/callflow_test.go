package callflow

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/zaf/g711"

	"sstv2tg/internal/phone"
	"sstv2tg/internal/sstv"
	"sstv2tg/internal/sstv/sstvtest"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

type post struct {
	png     []byte
	caption string
}

type sink struct {
	mu     sync.Mutex
	posts  []post
	posted chan struct{}
}

func newSink() *sink { return &sink{posted: make(chan struct{}, 1)} }

func (s *sink) SendPhoto(_ context.Context, png []byte, caption string) error {
	s.mu.Lock()
	s.posts = append(s.posts, post{png, caption})
	s.mu.Unlock()
	s.posted <- struct{}{}
	return nil
}

func (s *sink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts)
}

func freePort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func startServer(t *testing.T, s Sink) sip.Uri {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	port := freePort(t)
	cfg := phone.Config{User: "sstv", BindHost: "127.0.0.1", Port: port, MaxCalls: 1}
	opt := Options{VISTimeout: 2 * time.Second, SilenceTimeout: 2 * time.Second, Caption: "test caption", Sink: s}
	errc := make(chan error, 1)
	go func() {
		errc <- phone.Serve(ctx, cfg, func(ctx context.Context, c *phone.Call) { Handle(ctx, c, opt) })
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-errc; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	time.Sleep(100 * time.Millisecond)
	return sip.Uri{User: "sstv", Host: "127.0.0.1", Port: port}
}

func dial(t *testing.T, uri sip.Uri) (*diago.DialogClientSession, io.Writer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("caller"))
	if err != nil {
		t.Fatal(err)
	}
	dg := diago.NewDiago(ua,
		diago.WithTransport(diago.Transport{Transport: "udp", BindHost: "127.0.0.1", BindPort: freePort(t)}),
		diago.WithMediaConfig(diago.MediaConfig{Codecs: []media.Codec{media.CodecAudioUlaw}}),
	)
	if err := dg.ServeBackground(ctx, func(d *diago.DialogServerSession) {}); err != nil {
		t.Fatal(err)
	}

	inviteCtx, inviteCancel := context.WithTimeout(ctx, 5*time.Second)
	defer inviteCancel()
	d, m, err := dg.Invite(inviteCtx, uri, diago.InviteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		d.Hangup(ctx)
		d.Close()
	})
	w, err := m.AudioWriter()
	if err != nil {
		t.Fatal(err)
	}
	return d, w
}

// play sends pcm as mu-law frames; diago paces each Write at 20 ms.
func play(w io.Writer, pcm []int16) error {
	frame := make([]byte, 160)
	for i := 0; i < len(pcm); i += len(frame) {
		for j := range frame {
			var s int16
			if i+j < len(pcm) {
				s = pcm[i+j]
			}
			frame[j] = g711.EncodeUlawFrame(s)
		}
		if _, err := w.Write(frame); err != nil {
			return err
		}
	}
	return nil
}

// hold sends silence until the server hangs up.
func hold(d *diago.DialogClientSession, w io.Writer) {
	silence := make([]int16, 160)
	for d.Context().Err() == nil {
		if play(w, silence) != nil {
			return
		}
	}
}

func waitHangup(t *testing.T, d *diago.DialogClientSession, within time.Duration) {
	t.Helper()
	select {
	case <-d.Context().Done():
	case <-time.After(within):
		t.Fatalf("server did not hang up within %v", within)
	}
}

func decoded(t *testing.T, s *sink) image.Image {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.posts) != 1 {
		t.Fatalf("%d posts, want 1", len(s.posts))
	}
	if c := s.posts[0].caption; c != "test caption" {
		t.Errorf("caption %q, want test caption", c)
	}
	img, err := png.Decode(bytes.NewReader(s.posts[0].png))
	if err != nil {
		t.Fatalf("posted PNG: %v", err)
	}
	return img
}

func TestPostsWholeImage(t *testing.T) {
	t.Parallel()
	s := newSink()
	d, w := dial(t, startServer(t, s))

	card := sstvtest.Card(320, 240)
	go func() {
		if play(w, append(make([]int16, sstv.SampleRate/2), sstvtest.Robot36.Encode(card)...)) == nil {
			hold(d, w)
		}
	}()

	select {
	case <-s.posted:
	case <-time.After(60 * time.Second):
		t.Fatal("nothing posted")
	}
	waitHangup(t, d, 5*time.Second)

	img := decoded(t, s)
	if img.Bounds() != card.Bounds() {
		t.Fatalf("bounds %v, want %v", img.Bounds(), card.Bounds())
	}
	if got := sstvtest.MeanError(card, img); got > 8 {
		t.Errorf("mean per-channel error %.2f, want <= 8", got)
	}
}

func TestPostsPartialOnHangup(t *testing.T) {
	t.Parallel()
	s := newSink()
	d, w := dial(t, startServer(t, s))

	card := sstvtest.Card(320, 240)
	pcm := sstvtest.Robot36.Encode(card)
	if err := play(w, pcm[:int((0.910+60.5*0.150)*sstv.SampleRate)]); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := d.Hangup(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case <-s.posted:
	case <-time.After(10 * time.Second):
		t.Fatal("nothing posted")
	}
	img := decoded(t, s)
	h := img.Bounds().Dy()
	if h < 50 || h > 60 {
		t.Fatalf("height %d, want the 50-60 lines sent", h)
	}
	if got := sstvtest.MeanError(img, card); got > 8 {
		t.Errorf("mean per-channel error %.2f, want <= 8", got)
	}
}

func TestHangsUpWithoutPosting(t *testing.T) {
	for _, tc := range []struct {
		name string
		pcm  []int16
	}{
		{"unsupported VIS", sstvtest.Header(100)},
		{"no VIS", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newSink()
			d, w := dial(t, startServer(t, s))
			go func() {
				if play(w, tc.pcm) == nil {
					hold(d, w)
				}
			}()

			waitHangup(t, d, 5*time.Second)
			if n := s.count(); n != 0 {
				t.Errorf("%d posts, want 0", n)
			}
		})
	}
}
