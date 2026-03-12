package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"microagent/internal/channel"
	"microagent/internal/provider"
	"microagent/internal/store"
	"microagent/internal/tool"
)

func (a *Agent) processMessage(ctx context.Context, msg channel.IncomingMessage) {
	convID := "conv_" + msg.ChannelID
	conv, err := a.store.LoadConversation(ctx, convID)
	if err != nil {
		conv = &store.Conversation{
			ID:        convID,
			ChannelID: msg.ChannelID,
			CreatedAt: time.Now(),
		}
	}

	conv.Messages = append(conv.Messages, provider.ChatMessage{
		Role:    "user",
		Content: msg.Text,
	})

	if a.config.HistoryLength > 0 && len(conv.Messages) > a.config.HistoryLength {
		trim := len(conv.Messages) - a.config.HistoryLength
		conv.Messages = conv.Messages[trim:]
	}

	memories, _ := a.store.SearchMemory(ctx, msg.Text, a.config.MemoryResults)

	maxIters := a.config.MaxIterations
	if maxIters <= 0 {
		maxIters = 10
	}

	totalTimeout := a.limits.TotalTimeout
	if totalTimeout == 0 {
		totalTimeout = 120 * time.Second
	}
	loopCtx, cancelLoop := context.WithTimeout(ctx, totalTimeout)
	defer cancelLoop()

	for i := 0; i < maxIters; i++ {
		req := a.buildContext(conv, memories)

		resp, err := a.provider.Chat(loopCtx, req)
		if err != nil {
			slog.Error("provider chat failed", "error", err)
			_ = a.channel.Send(ctx, channel.OutgoingMessage{
				ChannelID: msg.ChannelID,
				Text:      fmt.Sprintf("Provider error: %v", err),
			})
			return
		}

		if resp.Content != "" {
			_ = a.channel.Send(ctx, channel.OutgoingMessage{
				ChannelID: msg.ChannelID,
				Text:      resp.Content,
			})
		}

		if len(resp.ToolCalls) == 0 {
			conv.Messages = append(conv.Messages, provider.ChatMessage{
				Role:    "assistant",
				Content: resp.Content,
			})
			if resp.Content != "" {
				entry := store.MemoryEntry{
					ID:        uuid.New().String(),
					Content:   resp.Content,
					Source:    convID,
					CreatedAt: time.Now(),
				}
				if err := a.store.AppendMemory(ctx, entry); err != nil {
					slog.Warn("failed to append memory", "error", err)
				}
			}
			break
		}

		conv.Messages = append(conv.Messages, provider.ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			slog.Info("executing tool", "name", tc.Name, "id", tc.ID)
			t, ok := a.tools[tc.Name]

			var result tool.ToolResult
			if !ok {
				result = tool.ToolResult{IsError: true, Content: fmt.Sprintf("Tool %s not found", tc.Name)}
			} else {
				toolTimeout := a.limits.ToolTimeout
				if toolTimeout == 0 {
					toolTimeout = 30 * time.Second
				}
				toolCtx, tCancel := context.WithTimeout(loopCtx, toolTimeout)
				result, err = executeWithRecover(toolCtx, t, tc.Input)
				tCancel()
				if err != nil {
					result = tool.ToolResult{IsError: true, Content: err.Error()}
				}
			}

			conv.Messages = append(conv.Messages, provider.ChatMessage{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: tc.ID,
			})
		}

		if i == maxIters-1 {
			_ = a.channel.Send(ctx, channel.OutgoingMessage{
				ChannelID: msg.ChannelID,
				Text:      "(iteration limit reached)",
			})
		}
	}

	conv.UpdatedAt = time.Now()
	_ = a.store.SaveConversation(ctx, *conv)
}

func executeWithRecover(ctx context.Context, t tool.Tool, params json.RawMessage) (result tool.ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = tool.ToolResult{IsError: true, Content: fmt.Sprintf("Tool crashed: %v", r)}
			err = nil
		}
	}()
	return t.Execute(ctx, params)
}
