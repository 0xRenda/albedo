package bin

import (
	"albedo-checker/internal/database"
	"time"
)

func (l *Lookup) CleanupOldCache(db *database.DB) error {
	_, err := db.Exec(`DELETE FROM bin_cache WHERE fetched_at < datetime('now', '-30 days')`)
	return err
}

func (l *Lookup) StartCacheCleaner(db *database.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		l.CleanupOldCache(db)
	}
}
