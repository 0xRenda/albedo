package user

import (
	"albedo-checker/internal/database"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

type Manager struct {
	db           *database.DB
	defaultLimit int
	keyPrefix    string
}

func NewManager(db *database.DB, defaultLimit int, keyPrefix string) *Manager {
	return &Manager{db: db, defaultLimit: defaultLimit, keyPrefix: keyPrefix}
}

func (m *Manager) Register(userID int64, username string) error {
	return m.db.RegisterUser(userID, username, m.defaultLimit)
}

func (m *Manager) Get(userID int64) (*database.User, error) {
	return m.db.GetUser(userID)
}

func (m *Manager) CanCheck(userID int64, requested int) (bool, error) {
	u, err := m.db.GetUser(userID)
	if err != nil {
		return false, err
	}
	if u == nil {
		m.Register(userID, "")
		u, _ = m.db.GetUser(userID)
	}
	if u.Role == "OWNER" || u.Role == "PREMIUM" {
		if u.Role == "PREMIUM" && u.PremiumExpiresAt != nil && u.PremiumExpiresAt.Before(time.Now()) {
			m.db.UpdateUserRole(userID, "FREE", nil)
			return m.CanCheck(userID, requested)
		}
		return true, nil
	}
	return u.CardsCheckedToday+requested <= u.DailyLimit, nil
}

func (m *Manager) IncrementChecks(userID int64, count, live, charged int) error {
	return m.db.IncrementUserChecks(userID, count, live, charged)
}

func (m *Manager) GeneratePremiumKey(durationDays, maxUses int, createdBy int64) (string, error) {
	key := fmt.Sprintf("%s%s-%s-%s-%s", m.keyPrefix, randHex(4), randHex(4), randHex(4), randHex(4))
	err := m.db.CreatePremiumKey(key, durationDays, maxUses, createdBy)
	return key, err
}

func (m *Manager) RedeemKey(userID int64, key string) error {
	k, err := m.db.GetPremiumKey(key)
	if err != nil {
		return err
	}
	if k == nil {
		return fmt.Errorf("invalid key")
	}
	if !k.IsActive {
		return fmt.Errorf("key revoked")
	}
	if k.UsedCount >= k.MaxUses {
		return fmt.Errorf("key exhausted")
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("key expired")
	}

	expires := time.Now().Add(time.Duration(k.DurationDays) * 24 * time.Hour)
	if err := m.db.UpdateUserRole(userID, "PREMIUM", &expires); err != nil {
		return err
	}
	return m.db.RedeemPremiumKey(key, userID)
}

func randHex(n int) string {
	const hex = "ABCDEF0123456789"
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(hex[rand.Intn(len(hex))])
	}
	return sb.String()
}
