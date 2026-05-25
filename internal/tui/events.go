package tui

// events.go — notify.Bus subscription bridge (AD-5).
//
// Pattern: Bus.Subscribe registers a thin NON-BLOCKING handler that drops
// events onto a Go channel. A blocking tea.Cmd (pumpEvents) drains that
// channel and returns each event as a tea.Msg. The Cmd re-issues itself so
// the program keeps draining indefinitely.
//
// Agent replies (agentReplyMsg from TUIChannel.out) are multiplexed onto the
// same events channel by a goroutine started in RunTUI.
//
// RULE: NO IO in Update. All state mutations happen in Update on the Msg.
// RULE: busEventMsg handler in Subscribe MUST be non-blocking (bus watchdog: 5s).

import (
	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/notify"
)

// busEventMsg wraps a notify.Event for delivery into the bubbletea event loop.
// It is emitted by pumpEvents and consumed in updateChat (screen_chat.go).
type busEventMsg struct {
	event notify.Event
}

// pumpEvents returns a tea.Cmd that blocks until a message is available on ch
// and then returns it as a tea.Msg. The caller MUST re-issue pumpEvents(ch)
// after consuming each message so the loop continues.
//
// Pattern (AD-5):
//
//	case busEventMsg, agentReplyMsg:
//	    // handle event
//	    return m, pumpEvents(m.events)
func pumpEvents(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// wireEvents sets up the bus subscription and agent reply multiplexer.
// It returns the events channel to be stored on the Model before the program
// starts. Must be called from RunTUI (before tea.NewProgram), NOT from Init
// (Init runs on a value-receiver copy and cannot reliably mutate the live model).
//
// Bus handler: non-blocking select with default (drops events when channel is
// full — telemetry is best-effort). Channel capacity 256 matches AD-5.
//
// Agent replies: a goroutine reads from ch.out (cap 64, agent-side buffer) and
// forwards each agentReplyMsg to evCh. This goroutine exits when ch.out is
// closed (via Stop or GC).
func wireEvents(bus notify.Bus, ch *TUIChannel) <-chan tea.Msg {
	evCh := make(chan tea.Msg, 256)

	// Thin bus handler — must not block (bus enforces 5s watchdog).
	bus.Subscribe(func(e notify.Event) {
		select {
		case evCh <- busEventMsg{event: e}:
		default:
			// Drop on overflow — telemetry is best-effort.
		}
	})

	// Forward agent replies from TUIChannel.out onto the same channel.
	go func() {
		for raw := range ch.out {
			evCh <- raw.(tea.Msg) //nolint:forcetypeassert // out only carries tea.Msg values
		}
	}()

	return evCh
}
