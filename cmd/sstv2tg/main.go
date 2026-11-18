package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"sstv2tg/internal/callflow"
	"sstv2tg/internal/config"
	"sstv2tg/internal/phone"
	"sstv2tg/internal/telegram"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tg := telegram.New(cfg.TGBotToken, cfg.TGChatID, cfg.TGThreadID)
	if err := tg.GetMe(ctx); err != nil {
		slog.Warn("telegram getMe failed; will retry on first post", "err", err)
	} else {
		slog.Info("telegram ready", "chat", cfg.TGChatID, "thread", cfg.TGThreadID)
	}

	opts := callflow.Options{
		VISTimeout:     cfg.VISTimeout,
		SilenceTimeout: cfg.SilenceTimeout,
		Caption:        cfg.TGCaption,
		Sink:           tg,
	}

	phoneCfg := phone.Config{
		Server:       cfg.SIPServer,
		User:         cfg.SIPUser,
		Pass:         cfg.SIPPass,
		Port:         cfg.SIPPort,
		AdvertisedIP: cfg.AdvertisedIP,
		RTPStart:     cfg.RTPPorts.Lo,
		RTPEnd:       cfg.RTPPorts.Hi,
		MaxCalls:     cfg.MaxCalls,
	}
	if err := phone.Serve(ctx, phoneCfg, func(ctx context.Context, c *phone.Call) {
		callflow.Handle(ctx, c, opts)
	}); err != nil {
		slog.Error("phone serve", "err", err)
		os.Exit(1)
	}
}
