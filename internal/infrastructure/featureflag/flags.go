package featureflag

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
)

// Manager handles feature flags with support for boolean toggles,
// gradual percentage rollouts, and instant emergency kill switches.
type Manager struct {
	mu    sync.RWMutex
	flags map[string]Flag
}

// Flag defines the configuration and rollout rules for a single feature flag.
type Flag struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Percentage int    `json:"percentage"`  // 0 to 100 for gradual rollout
	KillSwitch bool   `json:"kill_switch"` // Instant emergency override
}

// NewManager initializes an empty feature flag manager.
func NewManager() *Manager {
	return &Manager{
		flags: make(map[string]Flag),
	}
}

// Set configures or updates a feature flag rule.
func (m *Manager) Set(flag Flag) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if flag.Percentage < 0 {
		flag.Percentage = 0
	}
	if flag.Percentage > 100 {
		flag.Percentage = 100
	}
	m.flags[flag.Name] = flag
}

// IsEnabled evaluates whether a feature flag is active for a given entity key (e.g. userID).
func (m *Manager) IsEnabled(flagName, entityKey string) bool {
	m.mu.RLock()
	flag, exists := m.flags[flagName]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	// 1. Emergency Kill Switch takes immediate precedence
	if flag.KillSwitch {
		return false
	}

	// 2. Global master switch
	if !flag.Enabled {
		return false
	}

	// 3. 100% rollout enabled
	if flag.Percentage >= 100 {
		return true
	}

	// 4. 0% rollout
	if flag.Percentage <= 0 {
		return false
	}

	// 5. Deterministic gradual rollout based on entityKey hash (0 - 99)
	hash := sha256.Sum256([]byte(flagName + ":" + entityKey))
	num := binary.BigEndian.Uint32(hash[:4])
	bucket := int(num % 100)

	return bucket < flag.Percentage
}

// TriggerKillSwitch immediately disables a feature across all entities in emergency scenarios.
// func (m *Manager) TriggerKillSwitch(flagName string) {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	if flag, exists := m.flags[flagName]; exists {
// 		flag.KillSwitch = true
// 		m.flags[flagName] = flag
// 	}
// }
