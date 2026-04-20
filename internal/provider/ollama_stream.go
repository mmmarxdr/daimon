package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// --------------------------------------------------------------------------
// Ollama /api/chat NDJSON wire types
// --------------------------------------------------------------------------

// ollamaChatRequest is the request body for POST /api/chat.
// When think is true the model emits thinking tokens in message.thinking.
type ollamaChatRequest struct {
	Model    string               `json:"model"`
	Messages []ollamaChatMessage  `json:"messages"`
	Stream   bool                 `json:"stream"`
	Think    bool                 `json:"think,omitempty"`
}

// ollamaChatMessage is a single chat turn in the Ollama /api/chat request.
type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatToolCall represents a single tool call entry in message.tool_calls.
// Ollama does not supply IDs; they are generated deterministically at parse time.
type ollamaChatToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

// ollamaChatChunk is a single NDJSON line from POST /api/chat streaming response.
type ollamaChatChunk struct {
	Model   string `json:"model"`
	Message struct {
		Role      string               `json:"role"`
		Content   string               `json:"content"`
		Thinking  string               `json:"thinking"`
		ToolCalls []ollamaChatToolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// --------------------------------------------------------------------------
// ChatStream — Ollama native /api/chat override
// --------------------------------------------------------------------------

// ChatStream overrides the embedded OpenAIProvider.ChatStream.
// It always uses Ollama's native POST /api/chat endpoint (NDJSON streaming),
// and sets think:true for models identified as reasoning models by isOllamaReasoningModel.
//
// This avoids the OpenAI-compat /v1/chat/completions path which does not carry
// thinking tokens.
func (o *OllamaProvider) ChatStream(ctx context.Context, req ChatRequest) (*StreamResult, error) {
	model := req.Model
	if model == "" {
		model = o.OpenAIProvider.model
	}

	// Build the Ollama /api/chat request body.
	msgs := make([]ollamaChatMessage, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		msgs = append(msgs, ollamaChatMessage{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, ollamaChatMessage{
			Role:    m.Role,
			Content: m.Content.TextOnly(),
		})
	}

	apiReq := ollamaChatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
		Think:    isOllamaReasoningModel(model),
	}

	bodyBytes, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream: marshaling request: %w", err)
	}

	// Derive the Ollama server root from the embedded OpenAI provider's baseURL.
	// The OpenAI-compat baseURL is "http://host:port/v1"; we need "http://host:port".
	apiBase := o.OpenAIProvider.baseURL
	if apiBase == "" {
		apiBase = "http://localhost:11434/v1"
	}
	root := strings.TrimSuffix(apiBase, "/v1")
	if root == "" {
		root = "http://localhost:11434"
	}
	endpoint := root + "/api/chat"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama stream: creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.OpenAIProvider.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.OpenAIProvider.apiKey)
	}

	streamClient := &http.Client{} // no Timeout — context provides cancellation
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream: %w", wrapNetworkError(err))
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama stream: /api/chat returned %d: %s", resp.StatusCode, string(body))
	}

	sr, events := NewStreamResult(32)

	go func() {
		defer close(events)
		defer resp.Body.Close()

		var textContent strings.Builder
		var inputTokens, outputTokens int
		var stopReason string
		var chunkIndex int

		// Use a large scanner buffer to handle long thinking lines (Req: bufio gotcha).
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk ollamaChatChunk
			if err := json.Unmarshal(line, &chunk); err != nil {
				events <- StreamEvent{
					Type: StreamEventError,
					Err:  fmt.Errorf("ollama stream: parsing chunk: %w", err),
				}
				sr.SetResponse(nil, err)
				return
			}

			// Emit thinking (reasoning) delta — do NOT accumulate into textContent.
			if chunk.Message.Thinking != "" {
				events <- StreamEvent{
					Type: StreamEventReasoningDelta,
					Text: chunk.Message.Thinking,
				}
			}

			// Emit content (text) delta.
			if chunk.Message.Content != "" {
				textContent.WriteString(chunk.Message.Content)
				events <- StreamEvent{
					Type: StreamEventTextDelta,
					Text: chunk.Message.Content,
				}
			}

			// Emit tool call events. Ollama delivers complete tool calls in a single
			// chunk (no streaming delta accumulation needed). Each call produces
			// Start → Delta (marshaled args JSON) → End.
			for callIndex, tc := range chunk.Message.ToolCalls {
				toolCallID := fmt.Sprintf("ollama-tc-%d-%d", chunkIndex, callIndex)

				events <- StreamEvent{
					Type:       StreamEventToolCallStart,
					ToolCallID: toolCallID,
					ToolName:   tc.Function.Name,
				}

				argsJSON, marshalErr := json.Marshal(tc.Function.Arguments)
				if marshalErr != nil {
					argsJSON = []byte("{}")
				}
				events <- StreamEvent{
					Type:      StreamEventToolCallDelta,
					ToolInput: string(argsJSON),
				}

				events <- StreamEvent{Type: StreamEventToolCallEnd}
			}

			chunkIndex++

			// Handle done line.
			if chunk.Done {
				inputTokens = chunk.PromptEvalCount
				outputTokens = chunk.EvalCount
				stopReason = normalizeOllamaFinishReason(chunk.DoneReason)

				events <- StreamEvent{
					Type: StreamEventUsage,
					Usage: &UsageStats{
						InputTokens:  inputTokens,
						OutputTokens: outputTokens,
					},
					StopReason: stopReason,
				}
				events <- StreamEvent{Type: StreamEventDone}

				sr.SetResponse(&ChatResponse{
					Content:    textContent.String(),
					Usage:      UsageStats{InputTokens: inputTokens, OutputTokens: outputTokens},
					StopReason: stopReason,
				}, nil)
				return
			}
		}

		if err := scanner.Err(); err != nil {
			scanErr := fmt.Errorf("ollama stream: reading response: %w", wrapNetworkError(err))
			events <- StreamEvent{Type: StreamEventError, Err: scanErr}
			sr.SetResponse(nil, scanErr)
			return
		}

		// Stream ended without a done:true line — treat as EOF.
		sr.SetResponse(&ChatResponse{
			Content:    textContent.String(),
			StopReason: stopReason,
		}, nil)
	}()

	return sr, nil
}

// normalizeOllamaFinishReason maps Ollama done_reason strings to internal stop reason strings.
func normalizeOllamaFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	default:
		if reason == "" {
			return "end_turn"
		}
		return reason
	}
}
