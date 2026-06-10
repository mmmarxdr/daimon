package provider

import (
	"context"
	"fmt"

	"daimon/internal/config"
)

// miniMaxDefaultBaseURL is the global OpenAI-compatible MiniMax endpoint. Used
// when cfg.BaseURL is empty so a `provider: minimax` config never silently
// falls through to OpenAI's default base URL.
const miniMaxDefaultBaseURL = "https://api.minimax.io/v1"

// Compile-time interface assertions.
var _ Provider = (*MiniMaxProvider)(nil)
var _ StreamingProvider = (*MiniMaxProvider)(nil)

// MiniMaxProvider wraps *OpenAIProvider to add <think>...</think> stripping
// in both the sync Chat() and streaming ChatStream() paths. All other methods
// (HealthCheck, ListModels, Embed, etc.) are promoted from the embedded provider.
//
// MiniMax uses an OpenAI-compatible SSE endpoint, so no wire-level changes are
// needed — only post-processing of the response content.
type MiniMaxProvider struct {
	*OpenAIProvider
}

// NewMiniMaxProvider constructs a MiniMaxProvider from cfg.
// It delegates construction entirely to NewOpenAIProvider, which validates
// the api_key and wires base_url, model, timeout, etc.
//
// IMPORTANT: cfg.Model must be set explicitly. NewOpenAIProvider defaults to
// "gpt-4o" when Model is empty, which is wrong for MiniMax. Callers should
// set cfg.Model (e.g. "MiniMax-M2"). Config validation enforces the api_key +
// type checks; see ADR-5.1.
//
// When cfg.BaseURL is empty it defaults to miniMaxDefaultBaseURL so a minimax
// config never silently targets OpenAI's endpoint. Because that default also
// bypasses NewOpenAIProvider's openai-only api_key guard, api_key is enforced
// here explicitly (MM-1b).
func NewMiniMaxProvider(cfg config.ProviderConfig) (*MiniMaxProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("minimax: api_key is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = miniMaxDefaultBaseURL
	}
	inner, err := NewOpenAIProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &MiniMaxProvider{OpenAIProvider: inner}, nil
}

// Name returns "minimax" so logs, token-estimation maps, and the registry use
// the correct provider key.
func (p *MiniMaxProvider) Name() string { return "minimax" }

// Chat delegates to the inner OpenAIProvider.Chat and strips any
// <think>...</think> spans from the response Content. ToolCalls, Usage, and
// StopReason are passed through unchanged (ADR-4).
func (p *MiniMaxProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	resp, err := p.OpenAIProvider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	resp.Content = stripThinkContent(resp.Content)
	return resp, nil
}
