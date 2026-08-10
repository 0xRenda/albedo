package admin

import "fmt"

func (a *Admin) ValidateKeyFormat(key string) bool {
	return len(key) > 0 && (key[:7] == "ALBEDO-" || key[:7] == "albedo-")
}

func (a *Admin) GetKeyInfo(key string) (string, error) {
	k, err := a.db.GetPremiumKey(key)
	if err != nil {
		return "", err
	}
	if k == nil {
		return "", fmt.Errorf("key not found")
	}
	info := fmt.Sprintf("Key: %s\nRole: %s\nDuration: %d days\nUses: %d/%d\nActive: %v", k.KeyID, k.Role, k.DurationDays, k.UsedCount, k.MaxUses, k.IsActive)
	return info, nil
}
