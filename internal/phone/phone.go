// Package phone answers SIP calls and plumbs their audio in and out.
package phone

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/zaf/g711"
)

const (
	frameSamples  = 160 // 20 ms at 8 kHz
	hangupTimeout = 5 * time.Second

	registerExpiry = 120 * time.Second
)

var keepAliveInterval = 20 * time.Second

var codecs = []media.Codec{media.CodecAudioUlaw, media.CodecAudioAlaw}

type Config struct {
	// Server is the registrar host[:port]. Empty skips registration.
	Server string
	User   string
	Pass   string

	// BindHost is the local IP for SIP and RTP. Empty means the one that
	// routes to Server.
	BindHost string
	Port     int

	// AdvertisedIP overrides the address written into SIP and SDP.
	AdvertisedIP     net.IP
	RTPStart, RTPEnd int

	// MaxCalls caps concurrent calls; callers beyond it get 486 Busy Here.
	MaxCalls int
}

// Handler runs one answered call. ctx is cancelled when the caller hangs up.
type Handler func(ctx context.Context, c *Call)

// Serve answers calls and keeps the registration alive until ctx ends.
func Serve(ctx context.Context, cfg Config, handle Handler) error {
	bindHost, err := bindAddr(cfg)
	if err != nil {
		return err
	}

	ua, err := sipgo.NewUA(sipgo.WithUserAgent(cfg.User))
	if err != nil {
		return err
	}

	dg := newDiago(ua, bindHost, cfg)
	if err := dg.ServeBackground(ctx, limit(cfg.MaxCalls, handle)); err != nil {
		return err
	}

	if cfg.Server == "" {
		<-ctx.Done()
		return nil
	}

	var registrar sip.Uri
	if err := sip.ParseUri("sip:"+cfg.User+"@"+cfg.Server, &registrar); err != nil {
		return fmt.Errorf("SIP server %q: %w", cfg.Server, err)
	}
	go keepAlive(ctx, ua, net.JoinHostPort(bindHost, strconv.Itoa(cfg.Port)), registrar)
	return register(ctx, dg, registrar, cfg)
}

func bindAddr(cfg Config) (string, error) {
	if cfg.BindHost != "" {
		return cfg.BindHost, nil
	}
	return routeIP(cfg.Server)
}

func newDiago(ua *sipgo.UserAgent, bindHost string, cfg Config) *diago.Diago {
	media.RTPPortStart, media.RTPPortEnd = cfg.RTPStart, cfg.RTPEnd

	tr := diago.Transport{Transport: "udp", BindHost: bindHost, BindPort: cfg.Port}
	if cfg.AdvertisedIP != nil {
		tr.ExternalHost = cfg.AdvertisedIP.String()
		tr.MediaExternalIP = cfg.AdvertisedIP
	}

	return diago.NewDiago(ua, diago.WithTransport(tr), diago.WithMediaConfig(diago.MediaConfig{Codecs: codecs}))
}

// routeIP returns the local IP the kernel uses to reach server; dialing UDP
// sends nothing. Left alone, diago takes the first non-127 address on any
// interface, which can be one that never leaves the machine.
func routeIP(server string) (string, error) {
	host, port := server, "5060"
	if h, p, err := net.SplitHostPort(server); err == nil {
		host, port = h, p
	}

	c, err := net.Dial("udp4", net.JoinHostPort(host, port))
	if err != nil {
		return "", fmt.Errorf("route to SIP server %q: %w", server, err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

func limit(n int, handle Handler) diago.ServeDialogFunc {
	slots := make(chan struct{}, n)
	return func(d *diago.DialogServerSession) {
		select {
		case slots <- struct{}{}:
		default:
			d.Respond(sip.StatusBusyHere, "Busy Here", nil)
			return
		}
		defer func() { <-slots }()

		answer(d, handle)
	}
}

func register(ctx context.Context, dg *diago.Diago, registrar sip.Uri, cfg Config) error {
	err := dg.Register(ctx, registrar, diago.RegisterOptions{
		Username:     cfg.User,
		Password:     cfg.Pass,
		Expiry:       registerExpiry,
		OnRegistered: func() { slog.Info("registered", "server", cfg.Server, "user", cfg.User) },
	})
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// keepAlive pings the registrar from the SIP socket. NAT on the way back to
// us forgets an idle mapping well before the next re-register, so without
// this new INVITEs never arrive.
func keepAlive(ctx context.Context, ua *sipgo.UserAgent, from string, registrar sip.Uri) {
	client, err := sipgo.NewClient(ua, sipgo.WithClientNAT(), sipgo.WithClientConnectionAddr(from))
	if err != nil {
		slog.Error("keep-alive disabled", "err", err)
		return
	}

	t := time.NewTicker(keepAliveInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		reqCtx, cancel := context.WithTimeout(ctx, keepAliveInterval)
		_, err := client.Do(reqCtx, sip.NewRequest(sip.OPTIONS, registrar))
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("keep-alive failed", "err", err)
		}
	}
}

func answer(d *diago.DialogServerSession, handle Handler) {
	log := slog.With("call", d.InviteRequest.CallID().Value(), "from", d.FromUser())

	m, err := d.Answer(diago.AnswerOptions{})
	if err != nil {
		log.Warn("answer failed", "err", err)
		d.Respond(sip.StatusNotAcceptableHere, "Not Acceptable Here", nil)
		return
	}

	ctx := d.Context()
	c, err := newCall(ctx, m, log)
	if err != nil {
		log.Error("media setup failed", "err", err)
		hangup(d, log)
		return
	}

	log.Info("answered", "codec", c.codec)
	handle(ctx, c)
	hangup(d, log)
}

func hangup(d *diago.DialogServerSession, log *slog.Logger) {
	if d.Context().Err() != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), hangupTimeout)
	defer cancel()
	if err := d.Hangup(ctx); err != nil {
		log.Warn("hangup failed", "err", err)
	}
}

// Call is an answered call. RTP flows send-and-receive from answer until
// hangup: queued audio when there is some, silence otherwise; incoming
// frames are decoded to 8 kHz linear PCM and delivered on Audio.
type Call struct {
	log    *slog.Logger
	codec  string
	encode func(int16) byte
	decode func(byte) int16
	frames chan []byte
	audio  chan []int16
}

func newCall(ctx context.Context, m *diago.DialogMedia, log *slog.Logger) (*Call, error) {
	c := &Call{log: log, frames: make(chan []byte), audio: make(chan []int16, 64)}

	switch codec := media.CodecAudioFromSession(m.MediaSession()); codec.PayloadType {
	case media.CodecAudioUlaw.PayloadType:
		c.codec = "pcmu"
		c.encode = g711.EncodeUlawFrame
		c.decode = g711.DecodeUlawFrame
	case media.CodecAudioAlaw.PayloadType:
		c.codec = "pcma"
		c.encode = g711.EncodeAlawFrame
		c.decode = g711.DecodeAlawFrame
	default:
		return nil, fmt.Errorf("unsupported codec %s", codec.Name)
	}

	w, err := m.AudioWriter()
	if err != nil {
		return nil, err
	}
	r, err := m.AudioReader()
	if err != nil {
		return nil, err
	}
	go c.send(ctx, w)
	go c.receive(ctx, r)

	return c, nil
}

// send writes one frame per Write, which diago paces at 20 ms.
func (c *Call) send(ctx context.Context, w io.Writer) {
	silence := c.frame(nil)
	for {
		f := silence
		select {
		case <-ctx.Done():
			return
		case f = <-c.frames:
		default:
		}

		if _, err := w.Write(f); err != nil {
			return
		}
	}
}

// receive decodes each RTP payload to 8 kHz linear PCM. The buffer takes a
// whole packet: diago fails a read into anything smaller.
func (c *Call) receive(ctx context.Context, r io.Reader) {
	defer close(c.audio)
	buf := make([]byte, media.RTPBufSize)
	for {
		n, err := r.Read(buf)
		if err != nil {
			return
		}
		if n == 0 {
			continue
		}
		pcm := make([]int16, n)
		for i, b := range buf[:n] {
			pcm[i] = c.decode(b)
		}
		select {
		case c.audio <- pcm:
		case <-ctx.Done():
			return
		}
	}
}

// Log is tagged with the call ID and caller, so concurrent calls stay apart.
func (c *Call) Log() *slog.Logger { return c.log }

// Audio delivers the caller's RTP payloads as 8 kHz linear PCM until the call
// ends.
func (c *Call) Audio() <-chan []int16 { return c.audio }

// Play sends 8 kHz PCM and returns once the last frame is on the wire or ctx ends.
func (c *Call) Play(ctx context.Context, pcm []int16) error {
	for len(pcm) > 0 {
		n := min(frameSamples, len(pcm))
		select {
		case c.frames <- c.frame(pcm[:n]):
		case <-ctx.Done():
			return ctx.Err()
		}
		pcm = pcm[n:]
	}
	return nil
}

func (c *Call) frame(pcm []int16) []byte {
	f := make([]byte, frameSamples)
	for i := range f {
		var s int16
		if i < len(pcm) {
			s = pcm[i]
		}
		f[i] = c.encode(s)
	}
	return f
}
