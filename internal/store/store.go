package store

import (
	"context"
	"time"

	"microagent/internal/provider"
)

type Conversation struct {
	ID        string                 `json:"id"`
	ChannelID string                 `json:"channel_id"`
	Messages  []provider.ChatMessage `json:"messages"`
	Metadata  map[string]string      `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type MemoryEntry struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	Source    string    `json:"source"` // conversation ID
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	SaveConversation(ctx context.Context, conv Conversation) error
	LoadConversation(ctx context.Context, id string) (*Conversation, error)
	ListConversations(ctx context.Context, channelID string, limit int) ([]Conversation, error)
	AppendMemory(ctx context.Context, entry MemoryEntry) error
	SearchMemory(ctx context.Context, query string, limit int) ([]MemoryEntry, error)
	Close() error
}
