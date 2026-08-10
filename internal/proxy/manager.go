package proxy

import (
	"albedo-checker/internal/database"
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

type Proxy struct {
	ID       int64
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
	Health   int
	Latency  int
}

type Manager struct {
	db      *database.DB
	proxies []*Proxy
	mu      sync.RWMutex
	key     []byte
}

func NewManager(db *database.DB, encryptionKey string) *Manager {
	return &Manager{
		db:      db,
		proxies: make([]*Proxy, 0),
		key:     []byte(encryptionKey),
	}
}

func (m *Manager) Load() error {
	rows, err := m.db.GetActiveProxies()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxies = make([]*Proxy, 0, len(rows))
	for _, r := range rows {
		p := &Proxy{
			ID:       r.ProxyID,
			Protocol: r.Protocol,
			Host:     r.Host,
			Port:     r.Port,
			Username: r.Username,
			Health:   r.HealthScore,
			Latency:  r.LatencyMs,
		}
		if len(r.PasswordEncrypted) > 0 {
			pw, err := Decrypt(string(r.PasswordEncrypted), m.key)
			if err == nil {
				p.Password = string(pw)
			}
		}
		m.proxies = append(m.proxies, p)
	}
	return nil
}

func (m *Manager) AddProxies(rawList []string) (added, failed int) {
	for _, line := range rawList {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p, ok := parseProxy(line)
		if !ok {
			failed++
			continue
		}
		var pwEnc []byte
		if p.Password != "" {
			enc, err := Encrypt([]byte(p.Password), m.key)
			if err == nil {
				pwEnc = []byte(enc)
			}
		}
		if err := m.db.AddProxy(p.Protocol, p.Host, p.Port, p.Username, pwEnc); err != nil {
			failed++
			continue
		}
		added++
	}
	m.Load()
	return
}

func parseProxy(line string) (*Proxy, bool) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "http://") {
		u, err := url.Parse(line)
		if err != nil {
			return nil, false
		}
		h, p, _ := net.SplitHostPort(u.Host)
		port, _ := strconv.Atoi(p)
		return &Proxy{Protocol: "http", Host: h, Port: port, Username: u.User.Username(), Password: func() string { pw, _ := u.User.Password(); return pw }()}, true
	}
	if strings.HasPrefix(line, "socks5://") {
		u, err := url.Parse(line)
		if err != nil {
			return nil, false
		}
		h, p, _ := net.SplitHostPort(u.Host)
		port, _ := strconv.Atoi(p)
		return &Proxy{Protocol: "socks5", Host: h, Port: port, Username: u.User.Username(), Password: func() string { pw, _ := u.User.Password(); return pw }()}, true
	}
	parts := strings.Split(line, ":")
	if len(parts) == 2 {
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, false
		}
		return &Proxy{Protocol: "http", Host: parts[0], Port: port}, true
	}
	if len(parts) == 4 {
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, false
		}
		return &Proxy{Protocol: "http", Host: parts[0], Port: port, Username: parts[2], Password: parts[3]}, true
	}
	return nil, false
}

func (m *Manager) GetNext() *Proxy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.proxies) == 0 {
		return nil
	}
	candidates := make([]*Proxy, 0)
	for _, p := range m.proxies {
		if p.Health >= 30 {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return m.proxies[rand.Intn(len(m.proxies))]
	}
	return candidates[rand.Intn(len(candidates))]
}

func (m *Manager) Report(proxyID int64, success bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.proxies {
		if p.ID == proxyID {
			if success {
				p.Health += 5
				if p.Health > 100 {
					p.Health = 100
				}
			} else {
				p.Health -= 15
			}
			p.Latency = int(latency.Milliseconds())
			m.db.UpdateProxyHealth(proxyID, p.Health, p.Latency)
			break
		}
	}
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.proxies)
}

func (m *Manager) HealthCheck(ctx context.Context, checkURL string, timeout time.Duration) (alive, dead int) {
	m.mu.RLock()
	list := make([]*Proxy, len(m.proxies))
	copy(list, m.proxies)
	m.mu.RUnlock()

	for _, p := range list {
		client := &fasthttp.Client{
			ReadTimeout:  timeout,
			WriteTimeout: timeout,
		}
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		req.SetRequestURI(checkURL)
		req.Header.SetMethod("GET")
		req.Header.SetUserAgent("Albedo-HealthCheck/1.0")

		err := client.Do(req, resp)
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		if err != nil || resp.StatusCode() != 200 {
			dead++
			m.Report(p.ID, false, timeout)
		} else {
			alive++
			m.Report(p.ID, true, time.Millisecond*200)
		}
	}
	return
}

func (m *Manager) RemoveAll() error {
	m.mu.Lock()
	m.proxies = make([]*Proxy, 0)
	m.mu.Unlock()
	return m.db.RemoveAllProxies()
}

func (p *Proxy) String() string {
	if p.Username != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%d", p.Protocol, p.Username, p.Password, p.Host, p.Port)
	}
	return fmt.Sprintf("%s://%s:%d", p.Protocol, p.Host, p.Port)
}

func (p *Proxy) Display() string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}
