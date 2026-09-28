package featureflag

import (
	"fmt"
	"testing"
)

func TestFeatureFlag_BasicToggle(t *testing.T) {
	m := NewManager()
	m.Set(Flag{
		Name:       "new-dashboard",
		Enabled:    true,
		Percentage: 100,
	})

	if !m.IsEnabled("new-dashboard", "user-1") {
		t.Errorf("expected flag to be enabled")
	}

	m.Set(Flag{
		Name:       "new-dashboard",
		Enabled:    false,
		Percentage: 100,
	})

	if m.IsEnabled("new-dashboard", "user-1") {
		t.Errorf("expected flag to be disabled")
	}
}

func TestFeatureFlag_GradualRollout(t *testing.T) {
	m := NewManager()
	m.Set(Flag{
		Name:       "beta-export",
		Enabled:    true,
		Percentage: 20, // 20% of users
	})

	enabledCount := 0
	total := 1000

	for i := 0; i < total; i++ {
		userID := fmt.Sprintf("usr-%d", i)
		if m.IsEnabled("beta-export", userID) {
			enabledCount++
		}
	}

	// For a sample of 1000 users, 20% rollout should yield between 17% and 23%
	ratio := float64(enabledCount) / float64(total)
	if ratio < 0.17 || ratio > 0.23 {
		t.Errorf("expected ~20%% enabled, got %f (%d/%d)", ratio, enabledCount, total)
	}
}

// func TestFeatureFlag_EmergencyKillSwitch(t *testing.T) {
// 	m := NewManager()
// 	m.Set(Flag{
// 		Name:       "ai-task-summarizer",
// 		Enabled:    true,
// 		Percentage: 100,
// 	})

// 	if !m.IsEnabled("ai-task-summarizer", "user-1") {
// 		t.Errorf("expected feature to be active")
// 	}

// 	// Trigger emergency kill switch
// 	m.TriggerKillSwitch("ai-task-summarizer")

// 	if m.IsEnabled("ai-task-summarizer", "user-1") {
// 		t.Errorf("expected kill switch to immediately disable feature for all users")
// 	}
// }
