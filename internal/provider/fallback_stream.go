package provider

import (
	"context"
	"fmt"
)

// --------------------------------------------------------------------------
// StreamingProvider implementation for FallbackProvider
// --------------------------------------------------------------------------

// ChatStream implements StreamingProvider for FallbackProvider.
// It tries the primary provider first; on eligible pre-stream errors it falls
// back to the secondary. Mid-stream failures cannot fall back (the user has
// already seen partial text).
//
// If the primary does not implement StreamingProvider, the request is served
// via syncToStream (wrapping the synchronous Chat call).
func (f *FallbackProvider) ChatStream(ctx context.Context, req ChatRequest) (*StreamResult, error) {
	// Try primary first if it supports streaming.
	if sp, ok := f.primary.(StreamingProvider); ok {
		result, err := sp.ChatStream(ctx, req)
		if err == nil {
			return result, nil
		}
		// Pre-stream error — eligible for fallback?
		if isFallbackEligible(err) {
			f.logger.Warn("primary provider stream failed, activating fallback",
				"primary", f.primary.Name(),
				"fallback", f.fallback.Name(),
				"error", err,
			)
			return f.streamFallback(ctx, req, err)
		}
		return nil, err
	}

	// Primary doesn't support streaming — wrap synchronous Chat.
	return syncToStream(ctx, f.primary, req)
}

// streamFallback tries the fallback provider for streaming. If the fallback
// implements StreamingProvider, it delegates directly; otherwise it uses
// syncToStream. primaryErr is the original error from the primary, used for
// the combined error when both providers fail.
func (f *FallbackProvider) streamFallback(ctx context.Context, req ChatRequest, primaryErr error) (*StreamResult, error) {
	if sp, ok := f.fallback.(StreamingProvider); ok {
		result, err := sp.ChatStream(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("primary: %w; fallback: %v", primaryErr, err)
		}
		return result, nil
	}

	// Fallback doesn't support streaming — wrap synchronous Chat.
	result, err := syncToStream(ctx, f.fallback, req)
	if err != nil {
		return nil, fmt.Errorf("primary: %w; fallback: %v", primaryErr, err)
	}
	return result, nil
}

// Compile-time assertion: FallbackProvider implements StreamingProvider.
var _ StreamingProvider = (*FallbackProvider)(nil)
