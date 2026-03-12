package channel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"microagent/internal/config"

	"github.com/google/uuid"
)

type CLIChannel struct {
	config config.ChannelConfig
	in     io.Reader
	out    io.Writer
}

// NewCLIChannel creates a CLIChannel with injectable I/O for testing.
func NewCLIChannel(cfg config.ChannelConfig, in io.Reader, out io.Writer) *CLIChannel {
	return &CLIChannel{config: cfg, in: in, out: out}
}

// NewCLIChannelDefault creates a CLIChannel using os.Stdin and os.Stdout.
func NewCLIChannelDefault(cfg config.ChannelConfig) *CLIChannel {
	return NewCLIChannel(cfg, os.Stdin, os.Stdout)
}

func (c *CLIChannel) Name() string { return "cli" }

func (c *CLIChannel) Start(ctx context.Context, inbox chan<- IncomingMessage) error {
	go func() {
		scanner := bufio.NewScanner(c.in)
		fmt.Fprintln(c.out, "MicroAgent CLI started. Type your message and press ENTER (Ctrl+C to exit):")
		fmt.Fprint(c.out, "> ")

		lineCh := make(chan string)
		errCh := make(chan error, 1)

		go func() {
			for scanner.Scan() {
				lineCh <- scanner.Text()
			}
			errCh <- scanner.Err()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errCh:
				if err != nil {
					fmt.Fprintf(c.out, "Error reading stdin: %v\n", err)
				}
				return
			case text := <-lineCh:
				if text != "" {
					inbox <- IncomingMessage{
						ID:        uuid.New().String(),
						ChannelID: "cli",
						SenderID:  "local_user",
						Text:      text,
						Timestamp: time.Now(),
					}
				}
			}
		}
	}()
	return nil
}

func (c *CLIChannel) Send(ctx context.Context, msg OutgoingMessage) error {
	fmt.Fprintf(c.out, "\nAgent: %s\n> ", msg.Text)
	return nil
}

func (c *CLIChannel) Stop() error {
	return nil
}
