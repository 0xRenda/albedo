package bot

import (
	"albedo-checker/internal/config"
	"albedo-checker/internal/database"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

type Bot struct {
	api      *tgbotapi.BotAPI
	handlers *Handlers
	logger   *zap.Logger
}

func New(cfg *config.Config, db *database.DB, logger *zap.Logger) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}
	api.Debug = false
	handlers := NewHandlers(api, cfg, db, logger)
	return &Bot{api: api, handlers: handlers, logger: logger}, nil
}

func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return
		case update := <-updates:
			go b.handlers.Handle(update)
		}
	}
}
