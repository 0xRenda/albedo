package user

import (
	"albedo-checker/internal/database"
	"fmt"
)

func (m *Manager) IsAdmin(userID int64, adminID int64) bool {
	return userID == adminID
}

func (m *Manager) IsOwner(userID int64) (bool, error) {
	u, err := m.db.GetUser(userID)
	if err != nil {
		return false, err
	}
	return u != nil && u.Role == "OWNER", nil
}

func (m *Manager) SetRole(userID int64, role string, durationDays int) error {
	var expires *time.Time
	if durationDays > 0 && role == "PREMIUM" {
		t := time.Now().Add(time.Duration(durationDays) * 24 * time.Hour)
		expires = &t
	}
	return m.db.UpdateUserRole(userID, role, expires)
}

func (m *Manager) GetStats(userID int64) (*database.User, error) {
	return m.db.GetUser(userID)
}
