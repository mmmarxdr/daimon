package main

// tui_cmd.go — `daimon tui` positional subcommand handler (AD-11, REQ-1).
//
// Wiring:
//   1. Load config (same path as main).
//   2. Build agent + bus + store (minimal — no web, no cron, no health-check).
//   3. Create TUIChannel, wire it into a MultiplexChannel.
//   4. Start agent loop goroutine.
//   5. Call tui.RunTUI — blocks until the user quits.
//
// The agent loop goroutine exits when ctx is cancelled (on RunTUI return).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"daimon/internal/agent"
	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
	"daimon/internal/tui"
)

// runTUICommand is the entry function for `daimon tui`.
// args are the remaining os.Args after "tui"; cfgPath is the pre-extracted
// --config value (may be empty → default discovery).
func runTUICommand(args []string, cfgPath string) error {
	// 1. Config.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		if errors.Is(err, config.ErrNoConfig) {
			return fmt.Errorf("tui: no config file found — run `daimon setup` first")
		}
		return fmt.Errorf("tui: failed to load config: %w", err)
	}

	// 2. Store.
	st, err := store.New(cfg.Store)
	if err != nil {
		return fmt.Errorf("tui: failed to initialize store: %w", err)
	}
	defer st.Close()

	// 3. Tool registry (built-in tools only; no MCP for TUI minimal start).
	toolsRegistry := tool.BuildRegistrySimple(cfg.Tools)

	// 4. Skills (required by agent.New).
	skillContents, _, _, _ := skill.LoadSkillsUnified(
		context.Background(),
		cfg.Skills,
		nil, // no UserSkillStore in minimal path
		skill.CuratedFS,
		cfg.Tools.Shell,
		cfg.Limits,
	)
	_, skillIndex := agent.InitSkillInjection(skillContents, cfg.Agent.MaxContextTokens)

	// 5. Provider.
	activeProv := config.ResolveActiveProvider(*cfg)
	prov, err := buildProvider(activeProv)
	if err != nil {
		return fmt.Errorf("tui: failed to initialize provider: %w", err)
	}

	// 6. TUIChannel — wired as the agent's user-facing channel.
	// defer Stop() is defense-in-depth: the goroutine already exits via
	// ag.Shutdown() → mux.Stop() → tuiCh.Stop() (sync.Once-guarded), so this
	// defer is a harmless belt-and-suspenders call, not the primary exit path.
	tuiCh := tui.NewTUIChannel()
	defer tuiCh.Stop() //nolint:errcheck // Stop() always returns nil

	// 7. Single mux shared by the notification sender and the agent so both
	// routes reach the same TUIChannel instance (C4 fix: was two separate mux
	// instances wrapping tuiCh, meaning agent and notification sender used
	// different wrappers around the same channel).
	mux := channel.NewMultiplexChannel([]channel.Channel{tuiCh})

	// 8. Build the notification bus (nil if notifications disabled).
	var notifyBus notify.Bus
	if cfg.Notifications.Enabled && len(cfg.Notifications.Rules) > 0 {
		bus := notify.NewEventBus(
			cfg.Notifications.BusBufferSize,
			cfg.Notifications.MaxPerMinute,
			0, // zero → use default timeout
		)
		sender := notify.NewNotificationSender(mux, audit.NoopAuditor{}, bus)
		engine, engineErr := notify.NewRulesEngine(cfg.Notifications.Rules, sender)
		if engineErr == nil {
			bus.Subscribe(engine.Handle)
		}
		notifyBus = bus
	}

	// 9. Agent (reuses the mux constructed above).
	ag := agent.New(
		cfg.Agent, cfg.Limits, cfg.Filter,
		mux, prov, st,
		audit.NoopAuditor{},
		toolsRegistry,
		skillContents, skillIndex,
		cfg.Cron.MaxConcurrent,
		false, // no streaming in TUI minimal path
	).WithBus(notifyBus)

	// wireTodo is a package-private function in main; call via the exported path.
	// The TUI uses the agent accessor TodoListForConv, not the tool directly.

	// 10. Shutdown agent when RunTUI returns (S-b fix: prevents EventBus goroutine leak).
	defer func() {
		if err := ag.Shutdown(); err != nil {
			slog.Error("tui: agent shutdown error", "error", err)
		}
	}()

	// 11. Start agent loop in background — exits when ctx is cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agErrCh := make(chan error, 1)
	go func() {
		if err := ag.Run(ctx); err != nil && err != context.Canceled {
			slog.Error("tui: agent loop error", "error", err)
			agErrCh <- err
		}
		close(agErrCh)
	}()

	// 12. RunTUI blocks until the user quits (Ctrl+C / q).
	// Pass the same tuiCh that was wired into the mux so the Model reads/writes
	// the exact channel instance the agent's mux.Start() initialised.
	return tui.RunTUI(cfg, ag, notifyBus, st, tuiCh)
}
