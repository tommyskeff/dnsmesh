package cluster

import (
	"context"
	"sync"
	"time"

	"github.com/tommyskeff/dnsmesh/internal/config"
	"github.com/tommyskeff/dnsmesh/internal/logging"
)

const (
	initialBackoff = 5 * time.Second
	maxBackoff     = 5 * time.Minute
	backoffFactor  = 2.0
)

type Manager struct {
	mu       sync.RWMutex
	watchers map[string]*Watcher
	retrying map[string]bool
	domain   string
	updater  DNSTableUpdater
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewManager(domain string, updater DNSTableUpdater) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		watchers: make(map[string]*Watcher),
		retrying: make(map[string]bool),
		domain:   domain,
		updater:  updater,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (m *Manager) UpdateClusters(clusters []config.ClusterConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentNames := make(map[string]bool)
	for _, cfg := range clusters {
		currentNames[cfg.Name] = true
	}

	for name, watcher := range m.watchers {
		if !currentNames[name] {
			watcher.Stop()
			delete(m.watchers, name)
			logging.Info("Stopped watcher for removed cluster: %s", name)
		}
	}

	for _, cfg := range clusters {
		if _, exists := m.watchers[cfg.Name]; !exists {
			go m.startWatcherWithRetry(cfg)
		}
	}
}

func (m *Manager) InitialConnect(clusters []config.ClusterConfig) {
	var wg sync.WaitGroup

	for _, cfg := range clusters {
		wg.Add(1)
		go func(c config.ClusterConfig) {
			defer wg.Done()
			m.tryConnectOnce(c)
		}(cfg)
	}

	wg.Wait()
}

func (m *Manager) tryConnectOnce(cfg config.ClusterConfig) {
	client, err := NewClientFromConfig(cfg)
	if err != nil {
		logging.Error("Failed to create client for cluster %s: %v (will retry in background)", cfg.Name, err)
		go m.startWatcherWithRetry(cfg)
		return
	}

	watcher := NewWatcher(cfg.Name, client, m.domain, m.updater)
	if err := watcher.Start(m.ctx); err != nil {
		logging.Error("Failed to start watcher for cluster %s: %v (will retry in background)", cfg.Name, err)
		go m.startWatcherWithRetry(cfg)
		return
	}

	m.mu.Lock()
	m.watchers[cfg.Name] = watcher
	m.mu.Unlock()

	logging.Info("Started watcher for cluster: %s", cfg.Name)
}

func (m *Manager) startWatcherWithRetry(cfg config.ClusterConfig) {
	m.mu.Lock()
	if m.retrying[cfg.Name] {
		m.mu.Unlock()
		return
	}
	m.retrying[cfg.Name] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.retrying, cfg.Name)
		m.mu.Unlock()
	}()

	backoff := initialBackoff
	attempt := 0

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		client, err := NewClientFromConfig(cfg)
		if err != nil {
			if attempt == 0 {
				logging.Error("Failed to create client for cluster %s: %v (will retry)", cfg.Name, err)
			}
			attempt++
			m.waitWithBackoff(&backoff)
			continue
		}

		watcher := NewWatcher(cfg.Name, client, m.domain, m.updater)
		if err := watcher.Start(m.ctx); err != nil {
			if attempt == 0 {
				logging.Error("Failed to start watcher for cluster %s: %v (will retry)", cfg.Name, err)
			}
			attempt++
			m.waitWithBackoff(&backoff)
			continue
		}

		m.mu.Lock()
		m.watchers[cfg.Name] = watcher
		m.mu.Unlock()

		if attempt > 0 {
			logging.Info("Successfully connected to cluster %s after %d retries", cfg.Name, attempt)
		} else {
			logging.Info("Started watcher for cluster: %s", cfg.Name)
		}
		return
	}
}

func (m *Manager) waitWithBackoff(backoff *time.Duration) {
	select {
	case <-m.ctx.Done():
		return
	case <-time.After(*backoff):
	}

	*backoff = time.Duration(float64(*backoff) * backoffFactor)
	if *backoff > maxBackoff {
		*backoff = maxBackoff
	}
}

func (m *Manager) Stop() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, watcher := range m.watchers {
		watcher.Stop()
	}
	m.watchers = make(map[string]*Watcher)
}

func (m *Manager) GetActiveClusterCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.watchers)
}
