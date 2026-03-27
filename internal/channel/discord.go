package channel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"microagent/internal/config"
)

// DiscordChannel implements the channel.Channel interface using Discord's Gateway (WebSocket).
type DiscordChannel struct {
	session         *discordgo.Session
	allowedGuilds   map[string]bool
	allowedChannels map[string]bool
	cancel          context.CancelFunc
}

// NewDiscordChannel initializes the Discord session and sets up guild/channel allowlists.
func NewDiscordChannel(cfg config.ChannelConfig) (*DiscordChannel, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("discord token is required")
	}

	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize discord session: %w", err)
	}

	// Require message content intent to receive message bodies.
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	// For efficiency, parse allowlist slices into constant-time lookup maps.
	allowedGuilds := make(map[string]bool)
	for _, id := range cfg.AllowedGuilds {
		allowedGuilds[id] = true
	}

	allowedChannels := make(map[string]bool)
	for _, id := range cfg.AllowedChannels {
		allowedChannels[id] = true
	}

	return &DiscordChannel{
		session:         session,
		allowedGuilds:   allowedGuilds,
		allowedChannels: allowedChannels,
	}, nil
}

func (d *DiscordChannel) Name() string {
	return "discord"
}

// Start registers the MessageCreate handler, opens the WebSocket connection,
// and launches a goroutine to watch for context cancellation.
// MUST be non-blocking — the handler runs asynchronously via discordgo's dispatch loop.
func (d *DiscordChannel) Start(ctx context.Context, inbox chan<- IncomingMessage) error {
	stopCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	d.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore messages from bots (including ourselves).
		if m.Author == nil || m.Author.Bot {
			return
		}

		// Empty content (e.g. embeds-only) is not actionable.
		if strings.TrimSpace(m.Content) == "" {
			return
		}

		// Enforce guild allowlist when configured.
		if len(d.allowedGuilds) > 0 && !d.allowedGuilds[m.GuildID] {
			slog.Warn("discord message from unauthorized guild",
				"guild_id", m.GuildID,
				"author", m.Author.Username,
			)
			return
		}

		// Enforce channel allowlist when configured.
		if len(d.allowedChannels) > 0 && !d.allowedChannels[m.ChannelID] {
			slog.Warn("discord message from unauthorized channel",
				"channel_id", m.ChannelID,
				"author", m.Author.Username,
			)
			return
		}

		slog.Debug("discord message received",
			"channel_id", m.ChannelID,
			"guild_id", m.GuildID,
			"author", m.Author.Username,
			"content", m.Content,
		)

		// Attribute the sender so the LLM knows who is speaking in a team channel.
		text := fmt.Sprintf("[%s]: %s", m.Author.Username, m.Content)

		msg := IncomingMessage{
			ID:        m.ID,
			ChannelID: "discord:" + m.ChannelID,
			SenderID:  m.Author.ID,
			Text:      text,
			Timestamp: time.Now(),
		}

		// Non-blocking push: drop if inbox is full rather than blocking the dispatch loop.
		select {
		case inbox <- msg:
		case <-stopCtx.Done():
			return
		default:
			slog.Warn("inbox is currently full, dropping discord message", "channel_id", msg.ChannelID)
		}
	})

	if err := d.session.Open(); err != nil {
		cancel()
		return fmt.Errorf("failed to open discord session: %w", err)
	}

	slog.Info("discord gateway connected")

	// Watch for context cancellation and stop the session cleanly.
	go func() {
		<-stopCtx.Done()
		if err := d.session.Close(); err != nil {
			slog.Warn("discord session close error", "error", err)
		}
		slog.Info("discord gateway disconnected")
	}()

	return nil
}

// Stop cancels the internal context, which triggers the watcher goroutine to
// close the Discord session.
func (d *DiscordChannel) Stop() error {
	if d.cancel != nil {
		d.cancel()
	}
	return nil
}

// Send delivers a message to a Discord channel, chunking at 1900 characters
// to stay safely under Discord's 2000-character limit.
func (d *DiscordChannel) Send(ctx context.Context, msg OutgoingMessage) error {
	channelID := strings.TrimPrefix(msg.ChannelID, "discord:")

	const maxChars = 1900
	runes := []rune(msg.Text)
	length := len(runes)

	for i := 0; i < length; i += maxChars {
		end := i + maxChars
		if end > length {
			end = length
		}

		chunk := string(runes[i:end])
		if chunk == "" {
			continue
		}

		if _, err := d.session.ChannelMessageSend(channelID, chunk); err != nil {
			return fmt.Errorf("failed to send discord message chunk: %w", err)
		}
	}

	return nil
}
