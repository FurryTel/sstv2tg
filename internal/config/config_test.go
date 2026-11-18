package config

import (
	"testing"

	"github.com/caarlos0/env/v11"
)

func TestParseDefaults(t *testing.T) {
	c, err := parse(env.Options{Environment: map[string]string{
		"SSTV_SIP_SERVER":   "pbx.example.com",
		"SSTV_SIP_USER":     "sstv",
		"SSTV_SIP_PASS":     "pw",
		"SSTV_TG_BOT_TOKEN": "tok",
		"SSTV_TG_CHAT_ID":   "-1001",
	}})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.MaxCalls != 4 || c.RTPPorts.Lo != 16384 || c.RTPPorts.Hi != 16484 {
		t.Errorf("defaults wrong: %+v", c)
	}
	if c.TGChatID != -1001 {
		t.Errorf("chat ID = %d, want -1001", c.TGChatID)
	}
}

func TestParseRejectsBadRange(t *testing.T) {
	_, err := parse(env.Options{Environment: map[string]string{
		"SSTV_SIP_SERVER":     "x",
		"SSTV_SIP_USER":       "x",
		"SSTV_SIP_PASS":       "x",
		"SSTV_TG_BOT_TOKEN":   "x",
		"SSTV_TG_CHAT_ID":     "1",
		"SSTV_RTP_PORT_RANGE": "100",
	}})
	if err == nil {
		t.Fatal("want error on bad port range")
	}
}
