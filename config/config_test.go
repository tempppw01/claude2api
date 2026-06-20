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

func TestCooldownSessionAfterRateLimitFallsBackForStaleResetTime(t *testing.T) {
	cfg := &Config{
		Sessions: []SessionInfo{
			{SessionKey: "sk-test"},
		},
	}
	now := time.Date(2026, 6, 20, 19, 22, 55, 0, time.Local)

	cooldownUntil := cfg.CooldownSessionAfterRateLimit("sk-test", now, now)
	expected := now.Add(SessionRateLimitCooldown)
	if !cooldownUntil.Equal(expected) {
		t.Fatalf("expected fallback cooldown until %s, got %s", expected, cooldownUntil)
	}

	gotUntil, coolingDown := cfg.GetSessionCooldownByIndex(0, now.Add(time.Minute))
	if !coolingDown {
		t.Fatal("expected stale reset fallback to keep session cooling down")
	}
	if !gotUntil.Equal(expected) {
		t.Fatalf("expected stored fallback cooldown until %s, got %s", expected, gotUntil)
	}
}

func TestCooldownSessionAfterRateLimitUsesFutureResetTime(t *testing.T) {
	cfg := &Config{
		Sessions: []SessionInfo{
			{SessionKey: "sk-test"},
		},
	}
	now := time.Date(2026, 6, 20, 19, 22, 55, 0, time.Local)
	resetAt := now.Add(2 * time.Hour)

	cooldownUntil := cfg.CooldownSessionAfterRateLimit("sk-test", resetAt, now)
	if !cooldownUntil.Equal(resetAt) {
		t.Fatalf("expected official reset cooldown until %s, got %s", resetAt, cooldownUntil)
	}
}
