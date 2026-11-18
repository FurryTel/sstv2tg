package phone

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/zaf/g711"
)

func freePort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func startServer(t *testing.T, maxCalls int, h Handler) sip.Uri {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	port := freePort(t)
	cfg := Config{User: "sstv", BindHost: "127.0.0.1", Port: port, MaxCalls: maxCalls}
	errc := make(chan error, 1)
	go func() { errc <- Serve(ctx, cfg, h) }()
	t.Cleanup(func() {
		cancel()
		if err := <-errc; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	time.Sleep(100 * time.Millisecond)
	return sip.Uri{User: "sstv", Host: "127.0.0.1", Port: port}
}

func newCaller(t *testing.T, codecs ...media.Codec) *diago.Diago {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("caller"))
	if err != nil {
		t.Fatal(err)
	}
	dg := diago.NewDiago(ua,
		diago.WithTransport(diago.Transport{Transport: "udp", BindHost: "127.0.0.1", BindPort: freePort(t)}),
		diago.WithMediaConfig(diago.MediaConfig{Codecs: codecs}),
	)
	if err := dg.ServeBackground(ctx, func(d *diago.DialogServerSession) {}); err != nil {
		t.Fatal(err)
	}
	return dg
}

func dial(t *testing.T, dg *diago.Diago, uri sip.Uri) (*diago.DialogClientSession, *diago.DialogMedia, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, m, err := dg.Invite(ctx, uri, diago.InviteOptions{})
	if err == nil {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			d.Hangup(ctx)
			d.Close()
		})
	}
	return d, m, err
}

func TestCodecSelection(t *testing.T) {
	for _, want := range []struct {
		codec media.Codec
		name  string
	}{
		{media.CodecAudioUlaw, "pcmu"},
		{media.CodecAudioAlaw, "pcma"},
	} {
		got := make(chan string, 1)
		uri := startServer(t, 1, func(ctx context.Context, c *Call) { got <- c.codec })

		if _, _, err := dial(t, newCaller(t, want.codec), uri); err != nil {
			t.Fatalf("%s: %v", want.name, err)
		}
		if c := <-got; c != want.name {
			t.Errorf("offered %s, answered %s", want.name, c)
		}
	}
}

func TestNoCommonCodec(t *testing.T) {
	uri := startServer(t, 1, func(ctx context.Context, c *Call) {})

	_, _, err := dial(t, newCaller(t, media.CodecAudioOpus), uri)
	if err == nil || !strings.Contains(err.Error(), "488") {
		t.Errorf("opus-only offer: want 488, got %v", err)
	}
}

func TestCallerAudioReachesHandler(t *testing.T) {
	got := make(chan []int16, 1)
	uri := startServer(t, 1, func(ctx context.Context, c *Call) {
		select {
		case frame := <-c.Audio():
			got <- frame
		case <-ctx.Done():
		}
		<-ctx.Done()
	})

	_, m, err := dial(t, newCaller(t, media.CodecAudioUlaw), uri)
	if err != nil {
		t.Fatal(err)
	}
	w, err := m.AudioWriter()
	if err != nil {
		t.Fatal(err)
	}

	// 30 ms packets of a loud sample: larger than the 20 ms default, and
	// unlike silence once decoded.
	const loud = 0x1f
	packet := bytes.Repeat([]byte{loud}, 240)
	go func() {
		for range 10 {
			w.Write(packet)
		}
	}()

	select {
	case frame := <-got:
		if len(frame) != len(packet) {
			t.Fatalf("frame of %d samples, want %d", len(frame), len(packet))
		}
		if want := g711.DecodeUlawFrame(loud); frame[0] != want {
			t.Errorf("sample = %d, want %d", frame[0], want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no audio reached handler")
	}
}

func TestCallLimit(t *testing.T) {
	release := make(chan struct{})
	answered := make(chan struct{}, 2)
	uri := startServer(t, 2, func(ctx context.Context, c *Call) {
		answered <- struct{}{}
		<-release
	})
	defer close(release)

	for i := range 2 {
		if _, _, err := dial(t, newCaller(t, media.CodecAudioUlaw), uri); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		<-answered
	}

	_, _, err := dial(t, newCaller(t, media.CodecAudioUlaw), uri)
	if err == nil || !strings.Contains(err.Error(), "486") {
		t.Fatalf("third call: want 486, got %v", err)
	}
}

func TestRouteIP(t *testing.T) {
	for _, server := range []string{"127.0.0.1", "127.0.0.1:5080"} {
		ip, err := routeIP(server)
		if err != nil || ip != "127.0.0.1" {
			t.Errorf("routeIP(%q) = %q, %v; want 127.0.0.1", server, ip, err)
		}
	}
}

func TestKeepAliveLeavesFromSIPPort(t *testing.T) {
	saved := keepAliveInterval
	keepAliveInterval = 50 * time.Millisecond
	t.Cleanup(func() { keepAliveInterval = saved })

	pbx, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pbx.Close()

	ctx, cancel := context.WithCancel(context.Background())
	port := freePort(t)
	cfg := Config{Server: pbx.LocalAddr().String(), User: "sstv", BindHost: "127.0.0.1", Port: port, MaxCalls: 1}
	errc := make(chan error, 1)
	go func() { errc <- Serve(ctx, cfg, func(context.Context, *Call) {}) }()
	defer func() {
		cancel()
		if err := <-errc; err != nil {
			t.Errorf("serve: %v", err)
		}
	}()

	pbx.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 4096)
	for {
		n, from, err := pbx.ReadFrom(buf)
		if err != nil {
			t.Fatalf("no OPTIONS reached the registrar: %v", err)
		}
		if !strings.HasPrefix(string(buf[:n]), "OPTIONS ") {
			continue
		}
		if got := from.(*net.UDPAddr).Port; got != port {
			t.Fatalf("OPTIONS sent from port %d, want SIP port %d", got, port)
		}
		return
	}
}
