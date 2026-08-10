package store

import (
	"albedo-checker/internal/database"
	"math/rand"
	"sync"
	"time"
)

type Manager struct {
	db     *database.DB
	stores []*database.Store
	mu     sync.RWMutex
	idx    int
}

func NewManager(db *database.DB) *Manager {
	return &Manager{db: db}
}

func (m *Manager) Load() error {
	stores, err := m.db.GetActiveStores()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.stores = stores
	m.mu.Unlock()
	return nil
}

func (m *Manager) Add(url, name string, addedBy int64) error {
	if err := m.db.AddStore(url, name, addedBy); err != nil {
		return err
	}
	return m.Load()
}

func (m *Manager) Remove(url string) error {
	if err := m.db.RemoveStore(url); err != nil {
		return err
	}
	return m.Load()
}

func (m *Manager) GetNext() *database.Store {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.stores) == 0 {
		return nil
	}
	healthy := make([]*database.Store, 0)
	for i := range m.stores {
		if m.stores[i].HealthScore >= 30 {
			healthy = append(healthy, m.stores[i])
		}
	}
	if len(healthy) == 0 {
		s := m.stores[m.idx%len(m.stores)]
		m.idx++
		return s
	}
	s := healthy[m.idx%len(healthy)]
	m.idx++
	return s
}

func (m *Manager) Report(storeID int64, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.stores {
		if s.StoreID == storeID {
			if success {
				s.HealthScore += 2
				if s.HealthScore > 100 {
					s.HealthScore = 100
				}
			} else {
				s.HealthScore -= 10
			}
			s.TotalChecks++
			m.db.UpdateStoreHealth(storeID, s.HealthScore)
			break
		}
	}
}

func (m *Manager) List() []database.Store {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]database.Store, len(m.stores))
	copy(out, m.stores)
	return out
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.stores)
}

func (m *Manager) Clear() error {
	m.mu.Lock()
	m.stores = nil
	m.mu.Unlock()
	return m.db.ClearStores()
}

func (m *Manager) HealthCheckAll() error {
	stores, err := m.db.GetActiveStores()
	if err != nil {
		return err
	}
	for _, s := range stores {
		// Simple connectivity check would go here
		s.LastCheck = func() *time.Time { t := time.Now(); return &t }()
	}
	return nil
}
