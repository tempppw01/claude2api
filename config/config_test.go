package config

import (
	"testing"
	"time"
)

func TestCooldownSessionUntilUsesOfficialResetTime(t *testing.T) {
	cfg := &Config{
		Sessions: []SessionInfo{
			{SessionKey: "sk-test"},
		},
	}
	now := time.Date(2026, 6, 18, 19, 30, 0, 0, time.Local)
	resetAt := now.Add(3 * time.Hour)

	cooldownUntil := cfg.CooldownSessionUntil("sk-test", resetAt)
	if !cooldownUntil.Equal(resetAt) {
		t.Fatalf("expected cooldown until %s, got %s", resetAt, cooldownUntil)
	}

	gotUntil, coolingDown := cfg.GetSessionCooldownByIndex(0, now)
	if !coolingDown {
		t.Fatal("expected session to be cooling down")
	}
	if !gotUntil.Equal(resetAt) {
		t.Fatalf("expected stored cooldown until %s, got %s", resetAt, gotUntil)
	}

	_, coolingDown = cfg.GetSessionCooldownByIndex(0, resetAt.Add(time.Second))
	if coolingDown {
		t.Fatal("expected expired cooldown to auto clear")
	}
}
