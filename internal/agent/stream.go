package agent

import (
	"context"
	"log/slog"
	"time"

	"microagent/internal/channel"
	"microagent/internal/provider"
)

// processStreamingCall sends a streaming LLM request and progressively delivers
// text deltas to the channel's StreamWriter. Tool call events are buffered
// internally by the provider and returned in the assembled ChatResponse.
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
) (resp *provider.ChatResponse, textStreamed bool, err error) {

	// 1. Initiate the streaming request.
	result, err := sp.ChatStream(ctx, req)
	if err != nil {
		return nil, false, err
	}

	// 2. Lazily initialise the stream writer on the first TextDelta.
	//    Tool-only responses never open a writer.
	var sw channel.StreamWriter

	// 3. Consume events from the stream.
	for ev := range result.Events {
		switch ev.Type {
		case provider.StreamEventTextDelta:
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

		case provider.StreamEventToolCallStart,
			provider.StreamEventToolCallDelta,
			provider.StreamEventToolCallEnd:
			// Tool call events are handled internally by the provider's
			// response assembly. Nothing to forward to the channel.

		case provider.StreamEventUsage:
			// Usage metadata — no display action needed.

		case provider.StreamEventError:
			if sw != nil {
				_ = sw.Abort(ev.Err)
				sw = nil // prevent double-finalize
			}
			// Don't return yet — let result.Response() provide the canonical error.

		case provider.StreamEventDone:
			if sw != nil && textStreamed {
				if finErr := sw.Finalize(); finErr != nil {
					slog.Warn("stream finalize failed", "error", finErr)
				}
				sw = nil
			}
		}
	}

	// 4. Get the fully assembled response.
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
