package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"microagent/internal/config"
)

// WhatsAppChannel implements the channel.Channel interface using the
// WhatsApp Cloud API (webhook-based, DMs only).
type WhatsAppChannel struct {
	phoneNumberID string
	accessToken   string
	verifyToken   string
	port          int
	webhookPath   string
	allowedPhones map[string]bool
	httpServer    *http.Server
	client        *http.Client
}

// NewWhatsAppChannel initializes the WhatsApp Cloud API channel.
func NewWhatsAppChannel(cfg config.ChannelConfig) (*WhatsAppChannel, error) {
	if cfg.PhoneNumberID == "" {
		return nil, fmt.Errorf("whatsapp: phone_number_id is required")
	}
	if cfg.AccessToken == "" {
		return nil, fmt.Errorf("whatsapp: access_token is required")
	}
	if cfg.VerifyToken == "" {
		return nil, fmt.Errorf("whatsapp: verify_token is required")
	}

	// For efficiency, parse allowlist into a constant-time lookup map.
	allowedPhones := make(map[string]bool)
	for _, phone := range cfg.AllowedPhones {
		allowedPhones[phone] = true
	}

	port := cfg.WebhookPort
	if port == 0 {
		port = 8080
	}
	webhookPath := cfg.WebhookPath
	if webhookPath == "" {
		webhookPath = "/webhook"
	}

	return &WhatsAppChannel{
		phoneNumberID: cfg.PhoneNumberID,
		accessToken:   cfg.AccessToken,
		verifyToken:   cfg.VerifyToken,
		port:          port,
		webhookPath:   webhookPath,
		allowedPhones: allowedPhones,
		client:        &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (w *WhatsAppChannel) Name() string {
	return "whatsapp"
}

// Start registers HTTP handlers and begins listening for webhook events.
// Non-blocking: returns immediately after the server goroutine is launched.
func (w *WhatsAppChannel) Start(ctx context.Context, inbox chan<- IncomingMessage) error {
	mux := http.NewServeMux()
	mux.HandleFunc(w.webhookPath, func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.handleVerification(rw, r)
		case http.MethodPost:
			w.handleIncoming(rw, r, inbox, ctx)
		default:
			http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	addr := fmt.Sprintf(":%d", w.port)
	w.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Bind the listener before returning so that the port is ready when Start returns.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("whatsapp: failed to bind port %d: %w", w.port, err)
	}

	go func() {
		slog.Info("whatsapp webhook server started", "addr", addr, "path", w.webhookPath)
		if serveErr := w.httpServer.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			slog.Error("whatsapp webhook server error", "error", serveErr)
		}
		slog.Info("whatsapp webhook server stopped")
	}()

	// Stop the HTTP server when ctx is cancelled.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("whatsapp: error during server shutdown", "error", err)
		}
	}()

	return nil
}

// handleVerification responds to the Meta webhook verification GET request.
func (w *WhatsAppChannel) handleVerification(rw http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == w.verifyToken {
		slog.Info("whatsapp webhook verified successfully")
		rw.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(rw, challenge)
		return
	}

	slog.Warn("whatsapp webhook verification failed", "mode", mode)
	http.Error(rw, "forbidden", http.StatusForbidden)
}

// whatsappPayload is the top-level structure of an inbound webhook POST body.
type whatsappPayload struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Messages []struct {
					ID   string `json:"id"`
					From string `json:"from"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Timestamp string `json:"timestamp"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// handleIncoming processes an inbound webhook POST from Meta.
func (w *WhatsAppChannel) handleIncoming(rw http.ResponseWriter, r *http.Request, inbox chan<- IncomingMessage, ctx context.Context) {
	// Meta requires a fast 200 OK; always acknowledge immediately.
	rw.WriteHeader(http.StatusOK)

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB cap
	if err != nil {
		slog.Error("whatsapp: failed to read request body", "error", err)
		return
	}

	var payload whatsappPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Error("whatsapp: failed to parse webhook payload", "error", err)
		return
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, message := range change.Value.Messages {
				// Only process text messages; ignore images, audio, etc.
				if message.Type != "text" {
					slog.Debug("whatsapp: ignoring non-text message", "type", message.Type, "from", message.From)
					continue
				}

				// Check allowlist if configured.
				if len(w.allowedPhones) > 0 && !w.allowedPhones[message.From] {
					slog.Warn("whatsapp: unauthorized sender", "from", message.From)
					continue
				}

				channelID := "whatsapp:" + message.From

				slog.Debug("whatsapp message received",
					"from", message.From,
					"channel_id", channelID,
					"text", message.Text.Body,
				)

				msg := IncomingMessage{
					ID:        message.ID,
					ChannelID: channelID,
					SenderID:  message.From,
					Text:      message.Text.Body,
					Timestamp: time.Now(),
				}

				select {
				case inbox <- msg:
				case <-ctx.Done():
					return
				default:
					slog.Warn("whatsapp: inbox full, dropping message", "channel_id", channelID)
				}
			}
		}
	}
}

// Send delivers a text reply to a WhatsApp phone number via the Cloud API.
// Long messages are chunked at 4000 characters to stay within limits.
func (w *WhatsAppChannel) Send(ctx context.Context, msg OutgoingMessage) error {
	// Strip "whatsapp:" prefix to obtain the phone number.
	phone := strings.TrimPrefix(msg.ChannelID, "whatsapp:")

	const maxChars = 4000
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

		if err := w.sendChunk(ctx, phone, chunk); err != nil {
			return err
		}
	}

	return nil
}

// sendChunk posts a single text chunk to the WhatsApp Cloud API.
func (w *WhatsAppChannel) sendChunk(ctx context.Context, phone, text string) error {
	url := fmt.Sprintf("https://graph.facebook.com/v20.0/%s/messages", w.phoneNumberID)

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                phone,
		"type":              "text",
		"text": map[string]string{
			"body": text,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("whatsapp: failed to marshal send payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("whatsapp: failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.accessToken)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp: send request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("whatsapp: send returned non-2xx status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Stop gracefully shuts down the HTTP server.
func (w *WhatsAppChannel) Stop() error {
	if w.httpServer == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.httpServer.Shutdown(shutdownCtx)
}
