package bot

import (
	"albedo-checker/internal/admin"
	"albedo-checker/internal/bin"
	"albedo-checker/internal/checker"
	"albedo-checker/internal/config"
	"albedo-checker/internal/database"
	"albedo-checker/internal/proxy"
	"albedo-checker/internal/store"
	"albedo-checker/internal/user"
	"albedo-checker/internal/utils"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

type Handlers struct {
	bot       *tgbotapi.BotAPI
	cfg       *config.Config
	db        *database.DB
	userMgr   *user.Manager
	proxyMgr  *proxy.Manager
	storeMgr  *store.Manager
	checker   *checker.Shopify
	binLookup *bin.Lookup
	admin     *admin.Admin
	logger    *zap.Logger
}

func NewHandlers(bot *tgbotapi.BotAPI, cfg *config.Config, db *database.DB, logger *zap.Logger) *Handlers {
	um := user.NewManager(db, cfg.DefaultFreeLimit, cfg.PremiumKeyPrefix)
	pm := proxy.NewManager(db, cfg.ProxyEncryptionKey)
	sm := store.NewManager(db)
	ch := checker.NewShopify(cfg, db, pm, sm)
	bl := bin.NewLookup(db)
	ad := admin.NewAdmin(um, db, cfg.AdminID)

	pm.Load()
	sm.Load()

	return &Handlers{
		bot: bot, cfg: cfg, db: db, userMgr: um,
		proxyMgr: pm, storeMgr: sm, checker: ch,
		binLookup: bl, admin: ad, logger: logger,
	}
}

func (h *Handlers) Handle(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}
	msg := update.Message
	userID := msg.From.ID
	username := msg.From.UserName
	text := msg.Text

	h.userMgr.Register(userID, username)

	if text == "" && msg.Document != nil {
		h.handleDocument(userID, msg)
		return
	}

	parts := strings.SplitN(text, " ", 2)
	cmd := parts[0]
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	switch cmd {
	case "/start":
		h.cmdStart(userID, username)
	case "/sc":
		h.cmdSC(userID, args)
	case "/msc":
		h.cmdMSC(userID, msg)
	case "/msctxt":
		h.cmdMSCTxt(userID, msg)
	case "/bin":
		h.cmdBIN(userID, args)
	case "/addstore":
		h.cmdAddStore(userID, args)
	case "/rmstore":
		h.cmdRmStore(userID, args)
	case "/stores":
		h.cmdStores(userID)
	case "/clearstores":
		h.cmdClearStores(userID)
	case "/addproxy":
		h.cmdAddProxy(userID, msg)
	case "/proxy":
		h.cmdProxy(userID)
	case "/rmproxy":
		h.cmdRmProxy(userID)
	case "/redeem":
		h.cmdRedeem(userID, args)
	case "/me":
		h.cmdMe(userID)
	case "/genkey":
		h.cmdGenKey(userID, args)
	case "/revokekey":
		h.cmdRevokeKey(userID, args)
	case "/users":
		h.cmdUsers(userID)
	case "/setrole":
		h.cmdSetRole(userID, args)
	case "/broadcast":
		h.cmdBroadcast(userID, args)
	case "/stats":
		h.cmdStats(userID)
	default:
		if msg.ReplyToMessage != nil && strings.Contains(msg.ReplyToMessage.Text, "Send proxy list") {
			h.cmdAddProxy(userID, msg)
		}
	}
}

func (h *Handlers) send(userID int64, text string) {
	msg := tgbotapi.NewMessage(userID, text)
	msg.ParseMode = "HTML"
	h.bot.Send(msg)
}

func (h *Handlers) cmdStart(userID int64, username string) {
	role := "FREE"
	u, _ := h.userMgr.Get(userID)
	if u != nil {
		role = u.Role
	}
	if userID == h.cfg.AdminID {
		h.db.UpdateUserRole(userID, "OWNER", nil)
		role = "OWNER"
	}
	h.send(userID, utils.WelcomeMessage(username, role, userID))
}

func (h *Handlers) cmdSC(userID int64, args string) {
	can, _ := h.userMgr.CanCheck(userID, 1)
	if !can {
		h.send(userID, "❌ Daily limit reached. Use /redeem to upgrade.")
		return
	}

	number, month, year, cvv, ok := utils.ParseCard(args)
	if !ok {
		h.send(userID, "❌ Invalid card format. Use: /sc cc|mm|yy|cvv")
		return
	}

	h.send(userID, "⏳ Checking card...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := h.checker.CheckCard(ctx, number, month, year, cvv)
	if err != nil {
		h.logger.Error("check failed", zap.Error(err))
	}

	brand, cardType, level, bank, country, emoji := "Unknown", "Unknown", "Unknown", "Unknown", "Unknown", ""
	if h.cfg.EnableBINLookup {
		b, t, l, ba, c, e, err := h.binLookup.Lookup(number)
		if err == nil {
			brand, cardType, level, bank, country, emoji = b, t, l, ba, c, e
		}
	}

	price := "$1.0"
	status := "DEAD"
	reason := "UNKNOWN_ERROR"
	proxyUsed := "direct"
	storeUsed := "none"
	latency := time.Second

	if result != nil {
		status = result.Status
		reason = result.Reason
		proxyUsed = result.ProxyUsed
		storeUsed = result.StoreUsed
		latency = time.Duration(result.LatencyMs) * time.Millisecond
	}

	h.send(userID, utils.SingleResult(status, utils.MaskCard(number), "Shopify Payments", reason, price, proxyUsed, storeUsed, brand, cardType, level, bank, country, emoji, latency))

	liveCount := 0
	chargedCount := 0
	if status == "LIVE" {
		liveCount = 1
	}
	if status == "CHARGED" {
		chargedCount = 1
		liveCount = 1
	}
	h.userMgr.IncrementChecks(userID, 1, liveCount, chargedCount)
}

func (h *Handlers) cmdMSC(userID int64, msg *tgbotapi.Message) {
	if msg.ReplyToMessage == nil || msg.ReplyToMessage.Text == "" {
		h.send(userID, "❌ Reply to a message containing card list.")
		return
	}
	h.runMassCheck(userID, msg.ReplyToMessage.Text)
}

func (h *Handlers) cmdMSCTxt(userID int64, msg *tgbotapi.Message) {
	if msg.Document == nil {
		h.send(userID, "❌ Reply to a .txt file containing card list.")
		return
	}
	h.handleDocument(userID, msg)
}

func (h *Handlers) handleDocument(userID int64, msg *tgbotapi.Message) {
	fileURL, err := h.bot.GetFileDirectURL(msg.Document.FileID)
	if err != nil {
		h.send(userID, "❌ Failed to download file.")
		return
	}
	resp, err := http.Get(fileURL)
	if err != nil {
		h.send(userID, "❌ Failed to download file.")
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	h.runMassCheck(userID, string(body))
}

func (h *Handlers) runMassCheck(userID int64, content string) {
	lines := strings.Split(content, "\n")
	var cards []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, _, _, _, ok := utils.ParseCard(line); ok {
			cards = append(cards, line)
		}
	}

	if len(cards) == 0 {
		h.send(userID, "❌ No valid cards found.")
		return
	}

	can, _ := h.userMgr.CanCheck(userID, len(cards))
	if !can {
		h.send(userID, "❌ Daily limit exceeded for this batch.")
		return
	}

	sessionID, _ := h.db.CreateSession(userID, "Shopify", len(cards))
	progressMsg, _ := h.bot.Send(tgbotapi.NewMessage(userID, utils.MassProgress(len(cards), 0, 0, 0, 0, 0, 0)))

	start := time.Now()
	var mu sync.Mutex
	var checked, charged, live, dead, otp int
	var hits []string

	semaphore := make(chan struct{}, h.cfg.MassCheckConcurrency)
	var wg sync.WaitGroup

	for i, cardLine := range cards {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(idx int, cl string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			number, month, year, cvv, _ := utils.ParseCard(cl)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			result, err := h.checker.CheckCard(ctx, number, month, year, cvv)
			cancel()

			mu.Lock()
			checked++
			status := "DEAD"
			if result != nil {
				status = result.Status
			}
			if err != nil {
				status = "DEAD"
			}

			switch status {
			case "CHARGED":
				charged++
				live++
				hits = append(hits, "💎 "+utils.MaskCard(number))
			case "LIVE":
				live++
				hits = append(hits, "🟢 "+utils.MaskCard(number))
			case "OTP":
				otp++
			default:
				dead++
			}

			h.db.UpdateSession(sessionID, checked, charged, live, dead, otp)
			h.db.AddCheckResult(sessionID, utils.MaskCard(number), status, "", "", "", "", 0)

			if idx%10 == 0 || idx == len(cards)-1 {
				edit := tgbotapi.NewEditMessageText(userID, progressMsg.MessageID, utils.MassProgress(len(cards), checked, charged, live, dead, otp, time.Since(start)))
				h.bot.Send(edit)
			}
			mu.Unlock()

			time.Sleep(utils.JitteredDelay(h.cfg.RequestDelayMinMs, h.cfg.RequestDelayMaxMs))
		}(i, cardLine)
	}

	wg.Wait()
	duration := time.Since(start)
	h.db.CompleteSession(sessionID)

	h.send(userID, utils.MassSummary(len(cards), charged, live, dead, otp, duration, hits))
	h.userMgr.IncrementChecks(userID, checked, live, charged)
}

func (h *Handlers) cmdBIN(userID int64, args string) {
	if len(args) < 6 {
		h.send(userID, "❌ Usage: /bin 462845")
		return
	}
	bin := args[:6]
	brand, cardType, level, bank, country, emoji, err := h.binLookup.Lookup(bin)
	if err != nil {
		h.send(userID, "❌ BIN lookup failed: "+err.Error())
		return
	}
	h.send(userID, utils.BINLookup(bin, brand, cardType, level, bank, country, emoji))
}

func (h *Handlers) cmdAddStore(userID int64, args string) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Admin only.")
		return
	}
	if args == "" {
		h.send(userID, "❌ Usage: /addstore store.myshopify.com")
		return
	}
	name, hasShopify, latency, err := store.ValidateStore(args)
	if err != nil {
		h.send(userID, "❌ Store validation failed: "+err.Error())
		return
	}
	if !hasShopify {
		h.send(userID, "⚠️ Shopify Payments not detected.")
	}
	h.storeMgr.Add(args, name, userID)
	h.send(userID, utils.StoreAdded(args, "Active", "Shopify Payments", latency, h.storeMgr.Count()))
}

func (h *Handlers) cmdRmStore(userID int64, args string) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Admin only.")
		return
	}
	h.storeMgr.Remove(args)
	h.send(userID, utils.StoreRemoved(args, h.storeMgr.Count()))
}

func (h *Handlers) cmdStores(userID int64) {
	stores := h.storeMgr.List()
	var lines []string
	for i, s := range stores {
		emoji := "🟢"
		if s.HealthScore < 50 {
			emoji = "🔴"
		} else if s.HealthScore < 80 {
			emoji = "🟡"
		}
		lines = append(lines, fmt.Sprintf("%d. %s %s — Health: %d — Checks: %d", i+1, emoji, s.StoreURL, s.HealthScore, s.TotalChecks))
	}
	h.send(userID, utils.StoreList(lines))
}

func (h *Handlers) cmdClearStores(userID int64) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Admin only.")
		return
	}
	h.storeMgr.Clear()
	h.send(userID, "✅ All stores cleared.")
}

func (h *Handlers) cmdAddProxy(userID int64, msg *tgbotapi.Message) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Admin only.")
		return
	}
	var lines []string
	if msg.Document != nil {
		fileURL, _ := h.bot.GetFileDirectURL(msg.Document.FileID)
		resp, _ := http.Get(fileURL)
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lines = strings.Split(string(body), "\n")
		}
	} else if msg.Text != "" {
		lines = strings.Split(msg.Text, "\n")
	}
	if len(lines) == 0 {
		h.send(userID, "❌ Send proxy list as text or reply with file.")
		return
	}
	added, failed := h.proxyMgr.AddProxies(lines)
	h.send(userID, fmt.Sprintf("📲 Proxy Ingestion\n\n📥 Received: %d\n✅ Added: %d\n❌ Failed: %d\n\n💾 Total proxies: %d", len(lines), added, failed, h.proxyMgr.Count()))
}

func (h *Handlers) cmdProxy(userID int64) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Admin only.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	alive, dead := h.proxyMgr.HealthCheck(ctx, h.cfg.ProxyHealthCheckURL, h.cfg.ProxyTimeout())
	h.send(userID, fmt.Sprintf("🔍 Proxy Health Audit\n\n🟢 Alive: %d\n🔴 Dead: %d\n\n💾 Total: %d", alive, dead, h.proxyMgr.Count()))
}

func (h *Handlers) cmdRmProxy(userID int64) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Admin only.")
		return
	}
	h.proxyMgr.RemoveAll()
	h.send(userID, "✅ All proxies removed.")
}

func (h *Handlers) cmdRedeem(userID int64, args string) {
	if args == "" {
		h.send(userID, "❌ Usage: /redeem ALBEDO-XXXX-XXXX-XXXX")
		return
	}
	err := h.userMgr.RedeemKey(userID, args)
	if err != nil {
		h.send(userID, "❌ "+err.Error())
		return
	}
	h.send(userID, "✅ Premium activated!")
}

func (h *Handlers) cmdMe(userID int64) {
	u, err := h.userMgr.Get(userID)
	if err != nil || u == nil {
		h.send(userID, "❌ Profile not found.")
		return
	}
	limit := "Unlimited"
	if u.Role == "FREE" {
		limit = fmt.Sprintf("%d/%d", u.CardsCheckedToday, u.DailyLimit)
	}
	h.send(userID, fmt.Sprintf(`👤 𝑷𝑹𝑶𝑭𝑰𝑳𝑬 👤

👑 Role: %s
📊 Limit: %s
💳 Checked Today: %d
📈 Total Checks: %d
💎 Total Charged: %d
🟢 Total Live: %d

📅 Joined: %s
⏳ Last Active: Just now

━━━━━━━━━━━━━━━━━━━━━━
🛡️ Made by Xi`, u.Role, limit, u.CardsCheckedToday, u.TotalChecks, u.TotalCharged, u.TotalLive, u.JoinedAt.Format("2006-01-02")))
}

func (h *Handlers) cmdGenKey(userID int64, args string) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Owner only.")
		return
	}
	key, days, uses, err := h.admin.GenKey(args)
	if err != nil {
		h.send(userID, "❌ "+err.Error())
		return
	}
	h.send(userID, utils.PremiumKeyGenerated(key, days, uses))
	h.bot.Send(tgbotapi.NewMessage(userID, fmt.Sprintf("🔑 Key: `%s`", key)))
}

func (h *Handlers) cmdRevokeKey(userID int64, args string) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Owner only.")
		return
	}
	if err := h.admin.RevokeKey(args); err != nil {
		h.send(userID, "❌ "+err.Error())
		return
	}
	h.send(userID, "✅ Key revoked.")
}

func (h *Handlers) cmdUsers(userID int64) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Owner only.")
		return
	}
	users, err := h.admin.ListUsers()
	if err != nil {
		h.send(userID, "❌ "+err.Error())
		return
	}
	var sb strings.Builder
	sb.WriteString("👥 𝑼𝑺𝑬𝑹𝑺 👥\n\n")
	for _, u := range users {
		sb.WriteString(fmt.Sprintf("• %d | @%s | %s | Checks: %d\n", u.UserID, u.Username, u.Role, u.TotalChecks))
	}
	h.send(userID, sb.String())
}

func (h *Handlers) cmdSetRole(userID int64, args string) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Owner only.")
		return
	}
	parts := strings.Fields(args)
	if len(parts) < 2 {
		h.send(userID, "❌ Usage: /setrole <user_id> <role> [days]")
		return
	}
	days := 0
	if len(parts) > 2 {
		days, _ = strconv.Atoi(parts[2])
	}
	if err := h.admin.SetRole(parts[0], parts[1], days); err != nil {
		h.send(userID, "❌ "+err.Error())
		return
	}
	h.send(userID, "✅ Role updated.")
}

func (h *Handlers) cmdBroadcast(userID int64, args string) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Owner only.")
		return
	}
	users, _ := h.db.GetAllUsers()
	for _, u := range users {
		h.bot.Send(tgbotapi.NewMessage(u.UserID, "📢 𝑩𝑹𝑶𝑨𝑫𝑪𝑨𝑺𝑻 📢\n\n"+args+"\n\n━━━━━━━━━━━━━━━━━━━━━━\n🛡️ Made by Xi"))
	}
	h.send(userID, fmt.Sprintf("✅ Broadcast sent to %d users.", len(users)))
}

func (h *Handlers) cmdStats(userID int64) {
	if !h.admin.IsAdmin(userID) {
		h.send(userID, "❌ Owner only.")
		return
	}
	totalUsers, totalChecks, totalCharged, totalLive, storeCount, err := h.admin.Stats()
	if err != nil {
		h.send(userID, "❌ "+err.Error())
		return
	}
	h.send(userID, utils.AdminStats(totalUsers, totalChecks, totalCharged, totalLive, storeCount))
}
