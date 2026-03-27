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

// Compile-time assertion: MultiplexChannel implements StreamSender.
var _ StreamSender = (*MultiplexChannel)(nil)
