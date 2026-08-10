package admin

import (
	"albedo-checker/internal/database"
	"albedo-checker/internal/user"
	"fmt"
	"strconv"
	"strings"
)

type Admin struct {
	userMgr *user.Manager
	db      *database.DB
	adminID int64
}

func NewAdmin(um *user.Manager, db *database.DB, adminID int64) *Admin {
	return &Admin{userMgr: um, db: db, adminID: adminID}
}

func (a *Admin) IsAdmin(userID int64) bool {
	return userID == a.adminID
}

func (a *Admin) GenKey(args string) (string, int, int, error) {
	parts := strings.Fields(args)
	days := 7
	uses := 1
	if len(parts) > 0 {
		if d, err := strconv.Atoi(parts[0]); err == nil {
			days = d
		}
	}
	if len(parts) > 1 {
		if u, err := strconv.Atoi(parts[1]); err == nil {
			uses = u
		}
	}
	key, err := a.userMgr.GeneratePremiumKey(days, uses, a.adminID)
	return key, days, uses, err
}

func (a *Admin) RevokeKey(key string) error {
	return a.db.RevokePremiumKey(key)
}

func (a *Admin) ListUsers() ([]database.User, error) {
	return a.db.GetAllUsers()
}

func (a *Admin) SetRole(userIDStr, role string, durationDays int) error {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return err
	}
	return a.userMgr.SetRole(userID, role, durationDays)
}

func (a *Admin) Broadcast(msg string) string {
	return msg
}

func (a *Admin) Stats() (int, int, int, int, int, error) {
	users, err := a.db.GetAllUsers()
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	totalUsers := len(users)
	totalChecks := 0
	totalCharged := 0
	totalLive := 0
	for _, u := range users {
		totalChecks += u.TotalChecks
		totalCharged += u.TotalCharged
		totalLive += u.TotalLive
	}
	stores, _ := a.db.GetActiveStores()
	return totalUsers, totalChecks, totalCharged, totalLive, len(stores), nil
}
