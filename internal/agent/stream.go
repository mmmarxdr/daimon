package agent

import (
	"context"
	"log/slog"
	"time"

	"daimon/internal/channel"
	"daimon/internal/notify"
	"daimon/internal/provider"
)

// streamTelemetry is a convenience wrapper that emits a telemetry frame when
// te is non-nil. Errors are silently discarded — telemetry must never block
// or fail the agent loop.
func streamTelemetry(ctx context.Context, te channel.TelemetryEmitter, channelID string, frame map[string]any) {
	if te == nil {
		return
	}
	_ = te.EmitTelemetry(ctx, channelID, frame)
}

// processStreamingCall sends a streaming LLM request and progressively delivers
// text deltas to the channel's StreamWriter. Tool call events are buffered
// internally by the provider and returned in the assembled ChatResponse.
//
// subagentMeta carries the 4 attribution Meta keys for subagent conversations
// (REQ-10). It is nil for top-level agent turns. Every bus.Emit call merges
// these keys into the event Meta additively.
//
// Returns:
//   - resp: the fully assembled ChatResponse (text + tool calls + usage)
//   - textStreamed: true if text was already delivered to the user via StreamWriter;
//     the caller should skip channel.Send() for the text portion.
//   - err: non-nil on pre-stream or mid-stream fatal error
func (a *Agent) processStreamingCall(
	ctx context.Context,
	sp provider.StreamingProvider,
	ss channel.StreamSender, // may be nil if channel doesn't support streaming
	req provider.ChatRequest,
	channelID string,
	iteration int,
	llmStart time.Time,
	te channel.TelemetryEmitter, // may be nil if channel doesn't support telemetry
	subagentMeta map[string]string, // nil for top-level turns (REQ-10, D7)
) (resp *provider.ChatResponse, textStreamed bool, err error) {
	// 1. Initiate the streaming request.
	result, err := sp.ChatStream(ctx, req)
	if err != nil {
		return nil, false, err
	}

	// 2. Lazily initialise the stream writer on the first TextDelta.
	//    Tool-only responses never open a writer.
	var sw channel.StreamWriter

	// 3. Reasoning span state (D2). Local to this call; one span per contiguous
	//    block of ReasoningDelta events.
	var span struct {
		active    bool
		startedAt time.Time
	}

	// closeSpanIfOpen emits agent.reasoning.end and resets span state.
	// It is a no-op when no span is active or when bus is nil (REQ-14).
	closeSpanIfOpen := func() {
		if !span.active {
			return
		}
		if a.bus != nil {
			meta := make(map[string]string, len(subagentMeta)+1)
			for k, v := range subagentMeta {
				meta[k] = v
			}
			meta["conv_id"] = channelID // best proxy available in stream.go
			a.bus.Emit(notify.Event{
				Type:       notify.EventReasoningEnd,
				Origin:     notify.OriginAgent,
				ChannelID:  channelID,
				Timestamp:  time.Now(),
				DurationMs: time.Since(span.startedAt).Milliseconds(),
				Iteration:  iteration,
				Meta:       meta,
			})
		}
		span.active = false
	}

	// inputBytes tracks cumulative tool-call input bytes per tool ID (REQ-4, D6).
	inputBytes := map[string]int{}

	// 4. Consume events from the stream.
	for ev := range result.Events {
		switch ev.Type {
		case provider.StreamEventReasoningDelta:
			// Open a new reasoning span on the first delta of a contiguous block.
			if !span.active {
				span.active = true
				span.startedAt = time.Now()
				if a.bus != nil {
					meta := make(map[string]string, len(subagentMeta)+1)
					for k, v := range subagentMeta {
						meta[k] = v
					}
					meta["conv_id"] = channelID
					a.bus.Emit(notify.Event{
						Type:      notify.EventReasoningStart,
						Origin:    notify.OriginAgent,
						ChannelID: channelID,
						Timestamp: time.Now(),
						Iteration: iteration,
						Meta:      meta,
					})
				}
			}

			// Lazy init: open the stream writer on the first reasoning delta so
			// reasoning tokens that arrive before any text delta are still forwarded.
			if sw == nil && ss != nil {
				w, beginErr := ss.BeginStream(ctx, channelID)
				if beginErr != nil {
					slog.Warn("failed to begin stream for reasoning, falling back to buffered",
						"error", beginErr, "channel_id", channelID)
				} else {
					sw = w
				}
			}
			// Forward reasoning tokens to the stream writer if one is open.
			// Reasoning is supplementary — errors are non-fatal (slog.Debug only).
			// Do NOT accumulate into assembled content.
			if sw != nil {
				if writeErr := sw.WriteReasoning(ev.Text); writeErr != nil {
					slog.Debug("stream write reasoning failed (non-fatal)", "error", writeErr)
				}
			}

		case provider.StreamEventTextDelta:
			// Close any active reasoning span before switching to text (REQ-7.1).
			closeSpanIfOpen()

			// Lazy init: open the stream writer on the first text delta.
			if sw == nil && ss != nil {
				w, beginErr := ss.BeginStream(ctx, channelID)
				if beginErr != nil {
					slog.Warn("failed to begin stream, falling back to buffered send",
						"error", beginErr, "channel_id", channelID)
					// sw stays nil — text will be sent via channel.Send() after stream ends.
				} else {
					sw = w
				}
			}

			if sw != nil {
				if writeErr := sw.WriteChunk(ev.Text); writeErr != nil {
					slog.Warn("stream write chunk failed", "error", writeErr)
					// Continue consuming — the provider is still assembling the response.
				}
				textStreamed = true
			}

		case provider.StreamEventToolCallStart:
			// Close any active reasoning span (REQ-7.1 — non-reasoning event boundary).
			closeSpanIfOpen()

			// Forward to telemetry so the UI can show "tool in progress".
			streamTelemetry(ctx, te, channelID, map[string]any{
				"type":         "tool_start",
				"name":         ev.ToolName,
				"tool_call_id": ev.ToolCallID,
			})

		case provider.StreamEventToolCallDelta:
			// Accumulate cumulative input bytes and emit tool.delta telemetry (REQ-4, D6).
			// This is NOT a bus event — high-frequency, routes via TelemetryEmitter.
			inputBytes[ev.ToolCallID] += len(ev.ToolInput)
			streamTelemetry(ctx, te, channelID, map[string]any{
				"type":         notify.EventToolDelta,
				"tool_call_id": ev.ToolCallID,
				"token_count":  inputBytes[ev.ToolCallID],
			})

		case provider.StreamEventToolCallEnd:
			// Tool call assembly complete — provider will execute after stream ends.
			streamTelemetry(ctx, te, channelID, map[string]any{
				"type":         "tool_assembled",
				"name":         ev.ToolName,
				"tool_call_id": ev.ToolCallID,
			})

		case provider.StreamEventUsage:
			// Close any active reasoning span (REQ-7.1).
			closeSpanIfOpen()

			// Forward live token counts to the UI.
			if ev.Usage != nil {
				streamTelemetry(ctx, te, channelID, map[string]any{
					"type":          "stream_usage",
					"input_tokens":  ev.Usage.InputTokens,
					"output_tokens": ev.Usage.OutputTokens,
					"elapsed_ms":    time.Since(llmStart).Milliseconds(),
				})
			}

		case provider.StreamEventError:
			// Close any active reasoning span before aborting (D10).
			closeSpanIfOpen()

			if sw != nil {
				_ = sw.Abort(ev.Err)
				sw = nil // prevent double-finalize
			}
			// Don't return yet — let result.Response() provide the canonical error.

		case provider.StreamEventDone:
			// Close any active reasoning span before finalizing (REQ-7.2).
			closeSpanIfOpen()

			// Finalize whenever a writer was opened, regardless of whether text
			// was streamed. A reasoning-only response (no TextDelta) still needs
			// the writer closed to avoid a leaked stream on the channel side.
			if sw != nil {
				if finErr := sw.Finalize(); finErr != nil {
					slog.Warn("stream finalize failed", "error", finErr)
				}
				sw = nil
			}
		}
	}

	// 5. Get the fully assembled response.
	resp, err = result.Response()
	if err != nil {
		// Clean up writer if still open (e.g. error without explicit Error event).
		if sw != nil {
			_ = sw.Abort(err)
		}
		return nil, false, err
	}

	return resp, textStreamed, nil
}
