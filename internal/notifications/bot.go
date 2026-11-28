package notifications

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
)

type Bot struct {
	client  *Client
	allowed map[int64]struct{}
	logger  *log.Logger
	offset  int64
}

func newBot(client *Client, recipients []int64, logger *log.Logger) *Bot {
	allowed := make(map[int64]struct{}, len(recipients))
	for _, recipientID := range recipients {
		allowed[recipientID] = struct{}{}
	}

	return &Bot{
		client:  client,
		allowed: allowed,
		logger:  logger,
	}
}

// Run long-polls getUpdates. It does not start if Telegram rejects the token
// or if a webhook is configured, because repeating that request forever would
// only hide a configuration error.
func (b *Bot) Run(ctx context.Context) {
	b.logger.Printf("telegram bot: long polling started")

	for {
		if ctx.Err() != nil {
			return
		}

		updates, err := b.client.GetUpdates(ctx, b.offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			var apiErr *APIError
			if errors.As(err, &apiErr) && (apiErr.Code == 401 || apiErr.Code == 409) {
				b.logger.Printf("telegram bot: polling stopped: %v", err)
				return
			}

			b.logger.Printf("telegram bot: getUpdates failed: %v", err)

			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		b.handleUpdates(ctx, updates)
	}
}

func (b *Bot) handleUpdates(ctx context.Context, updates []telegramUpdate) {
	for _, update := range updates {
		if update.UpdateID < b.offset {
			continue
		}
		b.offset = update.UpdateID + 1

		message := update.Message
		if message == nil || message.From == nil || message.Chat == nil {
			continue
		}
		if message.Chat.Type != "private" {
			continue
		}
		if !isStartCommand(message.Text) {
			continue
		}

		if _, allowed := b.allowed[message.From.ID]; !allowed {
			b.logger.Printf("telegram bot: access denied for user %d", message.From.ID)
			_ = b.client.SendMessage(ctx, message.Chat.ID, "⛔ Доступ запрещён.")
			continue
		}

		_ = b.client.SendMessage(ctx, message.Chat.ID, "✅ Вы подключены к уведомлениям о новых заказах.")
	}
}

func isStartCommand(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}

	command := fields[0]
	return command == "/start" || strings.HasPrefix(command, "/start@")
}
