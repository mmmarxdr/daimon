package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"microagent/internal/agent"
	"microagent/internal/channel"
	"microagent/internal/config"
	"microagent/internal/provider"
	"microagent/internal/store"
	"microagent/internal/tool"
)

func main() {
	slog.Info("MicroAgent starting up")

	cfgPath := "configs/default.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("config loaded", "agent_name", cfg.Agent.Name)

	toolsRegistry := tool.BuildRegistry(cfg.Tools)

	prov := provider.NewAnthropicProvider(cfg.Provider)
	ch := channel.NewCLIChannelDefault(cfg.Channel)
	st := store.NewFileStore(cfg.Store)

	ag := agent.New(cfg.Agent, cfg.Limits, ch, prov, st, toolsRegistry)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		slog.Info("shutting down")
		ag.Shutdown()
		cancel()
	}()

	if err := ag.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("agent loop exited with error", "error", err)
	}

	slog.Info("MicroAgent exited")
}
