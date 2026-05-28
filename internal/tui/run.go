package tui

// run.go — RunTUI entry point and TTY guard helper (AD-11).
//
// RunTUI is the exported entry point called by `daimon tui` (cmd/daimon/tui_cmd.go).
// requireTTY is extracted as a shared helper usable by both RunMCPManage and RunTUI
// (REFACTOR task 1.25).

import (
	"context"
	"fmt"
	"os"
	"runtime"

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
// ch MUST be the same *TUIChannel that was wired into the agent's mux by the
// caller (cmd/daimon/tui_cmd.go). Passing the same instance ensures that the
// Model reads from the exact channel the agent writes to, and that submit()
// delivers to the inbox the mux initialised via Start.
//
// The agent loop goroutine MUST be started by the caller before calling RunTUI.
//
// Returns an error if stdin is not a TTY or if the bubbletea program exits
// with an error.
func RunTUI(cfg *config.Config, ag *agent.Agent, bus notify.Bus, st store.Store, ch *TUIChannel) error {
	return runTUIWithStdin(cfg, ag, bus, st, ch, os.Stdin)
}

// runTUIWithStdin is the testable inner implementation of RunTUI.
// It accepts an explicit stdin file so tests can inject /dev/null.
// ch is the caller-constructed TUIChannel already wired into the mux.
func runTUIWithStdin(cfg *config.Config, ag *agent.Agent, bus notify.Bus, st store.Store, ch *TUIChannel, stdin *os.File) error {
	if err := requireTTY(stdin); err != nil {
		return err
	}

	// Wire the events channel BEFORE constructing the Model so the live model
	// holds the channel from the start. Init() will start pumpEvents on it.
	// FIX 1: pass context.Background() — goroutine exit is controlled by
	// ch.done (closed by TUIChannel.Stop()) as the primary signal.
	evCh := wireEvents(context.Background(), bus, ch)

	s := newTuiStyles()
	// Inject cfg.Models.Default into the model-picker panel so it renders the
	// active provider/model instead of the empty-string sentinel ("").
	// newTestModel uses newRail(s) with empty strings — renders "" in tests, which is fine.
	r := newRail(s)

	// Build real values for the environment panel.
	cwd, _ := os.Getwd()
	// Only render "provider/model" when BOTH are set, so a half-configured
	// config never shows a dangling "anthropic/" or "/model".
	modelStr := ""
	if cfg.Models.Default.Provider != "" && cfg.Models.Default.Model != "" {
		modelStr = cfg.Models.Default.Provider + "/" + cfg.Models.Default.Model
	}

	r = copyRailWith(r, func(panels map[panelID]Panel) {
		panels[panelModelPicker] = newModelPickerPanel(s, cfg.Models.Default.Provider, cfg.Models.Default.Model)
		// PR4b: environment panel (welcome screen).
		panels[panelEnvironment] = newEnvironmentPanel(
			s,
			cwd,
			modelStr,
			runtime.Version(),
			runtime.GOOS+"/"+runtime.GOARCH,
			cfg.Store.Type,
		)
		// PR4b: resume-list panel (welcome + sessions screens) — starts empty;
		// populated when sessionsLoadedMsg arrives via the global handler.
		panels[panelResumeList] = newResumeListPanel(s)
		// PR5: active-policy panel (error screen) — built once with the agent's
		// current mode at startup. The /mode command performs a live SetMode swap
		// on the existing session; on a later denial the panel is refreshed via
		// setMode(ag.CurrentMode()) at denial time so it always shows the mode
		// that actually triggered the denial, not a stale startup snapshot.
		panels[panelActivePolicy] = newActivePolicyPanel(s, ag.CurrentMode())
	})
	m := Model{
		styles:     s,
		ag:         ag,
		bus:        bus,
		store:      st,
		ch:         ch,
		cfg:        cfg,
		channelID:  "tui",
		senderID:   "local_user",
		screen:     screenWelcome,
		focus:      focusEditor,
		events:     evCh,
		topBar:     topBar{brand: "⫶"},
		footer:     footerHints{screen: screenWelcome},
		input:      newInputBar(),
		rail:       r,
		modeAgent:  newAgentModeAdapter(ag),
		breadcrumb: breadcrumb{styles: s},
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
