package bot

import (
	"albedo-checker/internal/config"
	"albedo-checker/internal/user"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Middleware struct {
	cfg     *config.Config
	userMgr *user.Manager
}

func NewMiddleware(cfg *config.Config, userMgr *user.Manager) *Middleware {
	return &Middleware{cfg: cfg, userMgr: userMgr}
}

func (m *Middleware) EnsureUser(update tgbotapi.Update, next func(userID int64, username string)) {
	var userID int64
	var username string
	if update.Message != nil {
		userID = update.Message.From.ID
		username = update.Message.From.UserName
	} else if update.CallbackQuery != nil {
		userID = update.CallbackQuery.From.ID
		username = update.CallbackQuery.From.UserName
	} else {
		return
	}
	m.userMgr.Register(userID, username)
	next(userID, username)
}

func (m *Middleware) RequireAdmin(userID int64) bool {
	return userID == m.cfg.AdminID
}

func (m *Middleware) RequirePremium(userID int64) bool {
	can, _ := m.userMgr.CanCheck(userID, 1)
	return can
}
