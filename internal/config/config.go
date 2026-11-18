// Package config reads the SSTV_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	SIPServer    string    `env:"SSTV_SIP_SERVER,required,notEmpty"`
	SIPUser      string    `env:"SSTV_SIP_USER,required,notEmpty"`
	SIPPass      string    `env:"SSTV_SIP_PASS,required,notEmpty"`
	SIPPort      int       `env:"SSTV_SIP_LOCAL_PORT" envDefault:"5060"`
	MaxCalls     int       `env:"SSTV_MAX_CALLS" envDefault:"4"`
	RTPPorts     PortRange `env:"SSTV_RTP_PORT_RANGE" envDefault:"16384-16484"`
	AdvertisedIP net.IP    `env:"SSTV_ADVERTISED_IP"`

	TGBotToken string `env:"SSTV_TG_BOT_TOKEN,required,notEmpty"`
	TGChatID   int64  `env:"SSTV_TG_CHAT_ID,required"`
	TGThreadID int    `env:"SSTV_TG_THREAD_ID"`
	TGCaption  string `env:"SSTV_TG_CAPTION"`

	VISTimeout     time.Duration `env:"SSTV_VIS_TIMEOUT" envDefault:"60s"`
	SilenceTimeout time.Duration `env:"SSTV_SILENCE_TIMEOUT" envDefault:"2s"`
	LogLevel       slog.Level    `env:"SSTV_LOG_LEVEL" envDefault:"info"`
}

func Load() (Config, error) {
	return parse(env.Options{})
}

func parse(opts env.Options) (Config, error) {
	c, err := env.ParseAsWithOptions[Config](opts)
	if err != nil {
		return c, err
	}
	if c.MaxCalls < 1 {
		return c, fmt.Errorf("SSTV_MAX_CALLS: %d is less than 1", c.MaxCalls)
	}
	return c, nil
}

type PortRange struct{ Lo, Hi int }

func (r *PortRange) UnmarshalText(b []byte) error {
	lo, hi, ok := strings.Cut(string(b), "-")
	r.Lo, _ = strconv.Atoi(lo)
	r.Hi, _ = strconv.Atoi(hi)
	if !ok || r.Lo <= 0 || r.Hi <= r.Lo {
		return fmt.Errorf("%q: want low-high, e.g. 16384-16484", b)
	}
	return nil
}
