package channel

import (
	"context"
	"fmt"
	"strings"
)

// --------------------------------------------------------------------------
// StreamSender implementation for MultiplexChannel
// --------------------------------------------------------------------------

// BeginStream routes to the sub-channel that owns the given channelID.
// If the sub-channel implements StreamSender, it delegates directly;
// otherwise it returns ErrStreamNotSupported.
func (m *MultiplexChannel) BeginStream(ctx context.Context, channelID string) (StreamWriter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, child := range m.children {
		name := child.Name()
		if strings.HasPrefix(channelID, name+":") || channelID == name {
			if ss, ok := child.(StreamSender); ok {
				return ss.BeginStream(ctx, channelID)
			}
			return nil, ErrStreamNotSupported
		}
	}

	return nil, fmt.Errorf("mux: no channel found for channelID: %s", channelID)
}

// EmitTelemetry routes to the sub-channel that owns the given channelID.
// If the sub-channel implements TelemetryEmitter, it delegates; otherwise no-op.
func (m *MultiplexChannel) EmitTelemetry(ctx context.Context, channelID string, frame map[string]any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, child := range m.children {
		name := child.Name()
		if strings.HasPrefix(channelID, name+":") || channelID == name {
			if te, ok := child.(TelemetryEmitter); ok {
				return te.EmitTelemetry(ctx, channelID, frame)
			}
			return nil
		}
	}
	return nil
}

// Compile-time assertions.
var (
	_ StreamSender     = (*MultiplexChannel)(nil)
	_ TelemetryEmitter = (*MultiplexChannel)(nil)
)
