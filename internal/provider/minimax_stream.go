package provider

import (
	"context"
	"strings"
)

// --------------------------------------------------------------------------
// Think-tag filter constants
// --------------------------------------------------------------------------

const (
	thinkOpen  = "<think>"  // len 7
	thinkClose = "</think>" // len 8
)

// --------------------------------------------------------------------------
// longestSuffixThatIsPrefixOf — buffer-bound helper
// --------------------------------------------------------------------------

// longestSuffixThatIsPrefixOf returns the length of the longest suffix of work
// that is also a prefix of marker. The maximum returned value is
// len(marker)-1 (a full-length match would have been caught by strings.Index).
//
// This is the kernel of the retain-tail rule in thinkTagFilter.feed: we keep
// exactly this many bytes in buf so a split marker is detected on the next feed.
func longestSuffixThatIsPrefixOf(work, marker string) int {
	maxK := len(marker) - 1
	if maxK > len(work) {
		maxK = len(work)
	}
	for k := maxK; k >= 1; k-- {
		if strings.HasPrefix(marker, work[len(work)-k:]) {
			return k
		}
	}
	return 0
}

// --------------------------------------------------------------------------
// thinkTagFilter — 2-state streaming state machine
// --------------------------------------------------------------------------

// thinkTagFilter strips <think>...</think> spans from a streaming text,
// routing in-think bytes to reasonOut and out-of-think bytes to textOut.
// It is safe to use once (not pooled); create a fresh instance per stream.
//
// buf holds at most len(thinkClose)-1 == 7 bytes: the longest suffix of the
// most-recently-processed work that might be the start of the current marker.
type thinkTagFilter struct {
	inThink bool   // true after <think>, false after </think>
	buf     string // retained partial-marker prefix, ≤7 bytes
}

// feed processes a single delta (SSE chunk content) and returns the bytes
// that belong to the text channel and the reasoning channel respectively.
// Multiple calls accumulate state across chunks.
func (f *thinkTagFilter) feed(delta string) (textOut, reasonOut string) {
	work := f.buf + delta
	f.buf = ""

	var text, reason strings.Builder

	for {
		marker := thinkOpen
		if f.inThink {
			marker = thinkClose
		}

		idx := strings.Index(work, marker)
		if idx >= 0 {
			// Bytes before the marker go to the current state's channel.
			before := work[:idx]
			if before != "" {
				if f.inThink {
					reason.WriteString(before)
				} else {
					text.WriteString(before)
				}
			}
			work = work[idx+len(marker):]
			f.inThink = !f.inThink
			// Continue scanning the remainder for more markers.
			continue
		}

		// No complete marker found. Emit all but a possible partial-marker tail.
		keep := longestSuffixThatIsPrefixOf(work, marker)
		flushable := work[:len(work)-keep]
		if flushable != "" {
			if f.inThink {
				reason.WriteString(flushable)
			} else {
				text.WriteString(flushable)
			}
		}
		f.buf = work[len(work)-keep:]
		break
	}

	return text.String(), reason.String()
}

// flush emits any bytes held in buf at stream end. If the stream ended inside
// a <think> block (unclosed), residual goes to reasonOut; otherwise to textOut.
// After flush the filter is in a clean state.
func (f *thinkTagFilter) flush() (textOut, reasonOut string) {
	if f.buf == "" {
		return "", ""
	}
	out := f.buf
	f.buf = ""
	if f.inThink {
		return "", out // unclosed <think> at EOF → reasoning channel
	}
	return out, "" // dangling partial-<think> prefix → literal text
}

// --------------------------------------------------------------------------
// stripThinkContent — sync wrapper (ADR-2.5)
// --------------------------------------------------------------------------

// stripThinkContent removes all <think>...</think> spans from s and returns
// only the text channel output. Reasoning content is discarded (no channel
// available in the sync path). Unclosed <think> at EOF routes residual to
// reasoning (discarded), so Content is never polluted with CoT bytes.
func stripThinkContent(s string) string {
	var f thinkTagFilter
	text, _ := f.feed(s)
	textTail, _ := f.flush()
	return text + textTail
}

// --------------------------------------------------------------------------
// filterStreamResult — streaming rewire goroutine (ADR-3)
// --------------------------------------------------------------------------

// filterStreamResult wraps an upstream *StreamResult from OpenAIProvider.ChatStream,
// intercepting TextDelta events and routing think-tag content to ReasoningDelta.
// All other events (tool calls, usage, error) pass through unchanged.
//
// The goroutine lifetime is bound to the upstream Events channel; cancellation
// propagates naturally without needing a separate ctx select.
func filterStreamResult(upstream *StreamResult) *StreamResult {
	sr, events := NewStreamResult(32)

	go func() {
		defer close(events)

		var f thinkTagFilter

		for ev := range upstream.Events {
			switch ev.Type {
			case StreamEventTextDelta:
				text, reason := f.feed(ev.Text)
				if reason != "" {
					events <- StreamEvent{Type: StreamEventReasoningDelta, Text: reason}
				}
				if text != "" {
					events <- StreamEvent{Type: StreamEventTextDelta, Text: text}
				}

			case StreamEventDone:
				// Flush any retained partial-marker bytes before forwarding Done.
				text, reason := f.flush()
				if reason != "" {
					events <- StreamEvent{Type: StreamEventReasoningDelta, Text: reason}
				}
				if text != "" {
					events <- StreamEvent{Type: StreamEventTextDelta, Text: text}
				}
				events <- ev // forward Done

			default:
				// ReasoningDelta, ToolCallStart/Delta/End, Usage, Error → pass through.
				events <- ev
			}
		}

		// Upstream channel closed. Assemble the final response with stripped Content.
		resp, rerr := upstream.Response()
		if rerr != nil {
			sr.SetResponse(nil, rerr)
			return
		}
		if resp != nil {
			stripped := *resp // shallow copy
			stripped.Content = stripThinkContent(resp.Content)
			sr.SetResponse(&stripped, nil)
		} else {
			sr.SetResponse(resp, nil)
		}
	}()

	return sr
}

// --------------------------------------------------------------------------
// MiniMaxProvider.ChatStream — streaming entry point
// --------------------------------------------------------------------------

// ChatStream overrides the embedded OpenAIProvider.ChatStream to route
// <think>...</think> tokens to ReasoningDelta events.
// It delegates transport entirely to the inner OpenAIProvider (MiniMax uses
// an OpenAI-compatible SSE endpoint) and wraps the stream with filterStreamResult.
func (p *MiniMaxProvider) ChatStream(ctx context.Context, req ChatRequest) (*StreamResult, error) {
	upstream, err := p.OpenAIProvider.ChatStream(ctx, req)
	if err != nil {
		return nil, err
	}
	return filterStreamResult(upstream), nil
}
