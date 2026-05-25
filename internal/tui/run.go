package tui

// run.go — RunTUI entry point and TTY guard helper (AD-11).
//
// RunTUI is the exported entry point called by `daimon tui` (cmd/daimon/tui_cmd.go).
// requireTTY is extracted as a shared helper usable by both RunMCPManage and RunTUI
// (REFACTOR task 1.25).

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"daimon/internal/agent"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/store"
)

// requireTTY returns an error if f is not an interactive terminal.
// Shared by RunTUI and RunMCPManage (refactor task 1.25).
func requireTTY(f *os.File) error {
	if isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()) {
		return nil
	}
	return fmt.Errorf("daimon tui requires a TTY (stdin is not a terminal)")
}

// RunTUI starts the full-screen Bubble Tea TUI.
//
// It builds the root Model with the injected ag/bus/store + a fresh TUIChannel,
// then calls tea.NewProgram(m, tea.WithAltScreen()).Run(). The agent loop goroutine
// MUST be started by the caller (cmd/daimon/tui_cmd.go) before calling RunTUI.
//
// Returns an error if stdin is not a TTY or if the bubbletea program exits
// with an error.
func RunTUI(cfg *config.Config, ag *agent.Agent, bus notify.Bus, st store.Store) error {
	return runTUIWithStdin(cfg, ag, bus, st, os.Stdin)
}

// runTUIWithStdin is the testable inner implementation of RunTUI.
// It accepts an explicit stdin file so tests can inject /dev/null.
func runTUIWithStdin(cfg *config.Config, ag *agent.Agent, bus notify.Bus, st store.Store, stdin *os.File) error {
	if err := requireTTY(stdin); err != nil {
		return err
	}

	ch := newTUIChannel()
	m := Model{
		styles:    newTuiStyles(),
		ag:        ag,
		bus:       bus,
		store:     st,
		ch:        ch,
		cfg:       cfg,
		channelID: "tui",
		senderID:  "local_user",
		screen:    screenWelcome,
		topBar:    topBar{brand: "⫶"},
		footer:    footerHints{screen: screenWelcome},
		input:     newInputBar(),
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
