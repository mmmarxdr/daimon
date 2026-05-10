package agent

import (
	"strings"
	"testing"

	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/store"
)

// TestShouldGenerateTitle_SkipsChildConversations verifies that
// shouldGenerateTitle returns false when conv.ParentConvID is non-empty.
// This prevents the title generator from firing for ephemeral subagent convs.
// Satisfies design §6 risk 5.
func TestShouldGenerateTitle_SkipsChildConversations(t *testing.T) {
	mkMsg := func(role, text string) provider.ChatMessage {
		return provider.ChatMessage{
			Role:    role,
			Content: content.Blocks{{Type: content.BlockText, Text: text}},
		}
	}

	longEnough := strings.Repeat("a", 25) // 25 runes > minFirstUserRunesForTitle (20)
	msgs := []provider.ChatMessage{
		mkMsg("user", longEnough),
		mkMsg("assistant", "x"),
		mkMsg("user", "y"),
		mkMsg("assistant", "z"),
		mkMsg("user", "w"),
		mkMsg("assistant", "v"),
	}

	t.Run("child conv is skipped", func(t *testing.T) {
		conv := &store.Conversation{
			Messages:     msgs,
			ParentConvID: "parent-123",
		}
		if shouldGenerateTitle(conv) {
			t.Error("shouldGenerateTitle must return false for child conversations (ParentConvID != \"\")")
		}
	})

	t.Run("root conv still eligible", func(t *testing.T) {
		conv := &store.Conversation{
			Messages:     msgs,
			ParentConvID: "", // root conv
		}
		if !shouldGenerateTitle(conv) {
			t.Error("shouldGenerateTitle must return true for root conversations with sufficient content")
		}
	})
}
