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

	gotUntil, gotSource, coolingDown := cfg.GetSessionCooldownInfoByIndex(0, now)
	if !coolingDown {
		t.Fatal("expected session to be cooling down")
	}
	if !gotUntil.Equal(resetAt) {
		t.Fatalf("expected stored cooldown until %s, got %s", resetAt, gotUntil)
	}
	if gotSource != CooldownSourceOfficial {
		t.Fatalf("expected official cooldown source, got %s", gotSource)
	}

	_, coolingDown = cfg.GetSessionCooldownByIndex(0, resetAt.Add(time.Second))
	if coolingDown {
		t.Fatal("expected expired cooldown to auto clear")
	}
}

func TestCooldownSessionAfterRateLimitIgnoresStaleResetTime(t *testing.T) {
	cfg := &Config{
		Sessions: []SessionInfo{
			{SessionKey: "sk-test"},
		},
	}
	now := time.Date(2026, 6, 20, 19, 22, 55, 0, time.Local)

	cooldownUntil, cooldownSource := cfg.CooldownSessionAfterRateLimit("sk-test", now, now)
	if !cooldownUntil.IsZero() {
		t.Fatalf("expected no cooldown for stale reset time, got %s", cooldownUntil)
	}
	if cooldownSource != "" {
		t.Fatalf("expected empty cooldown source, got %s", cooldownSource)
	}

	gotUntil, gotSource, coolingDown := cfg.GetSessionCooldownInfoByIndex(0, now.Add(time.Minute))
	if coolingDown {
		t.Fatalf("expected stale reset time not to freeze session, got until %s source %s", gotUntil, gotSource)
	}
}

func TestCooldownSessionAfterRateLimitDoesNotFreezeWithoutResetTime(t *testing.T) {
	cfg := &Config{
		Sessions: []SessionInfo{
			{SessionKey: "sk-test"},
		},
	}
	now := time.Date(2026, 6, 20, 19, 22, 55, 0, time.Local)

	cooldownUntil, cooldownSource := cfg.CooldownSessionAfterRateLimit("sk-test", time.Time{}, now)
	if !cooldownUntil.IsZero() {
		t.Fatalf("expected no cooldown without official reset time, got %s", cooldownUntil)
	}
	if cooldownSource != "" {
		t.Fatalf("expected empty cooldown source, got %s", cooldownSource)
	}

	if gotUntil, gotSource, coolingDown := cfg.GetSessionCooldownInfoByIndex(0, now.Add(time.Minute)); coolingDown {
		t.Fatalf("expected missing reset time not to freeze session, got until %s source %s", gotUntil, gotSource)
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

	cooldownUntil, cooldownSource := cfg.CooldownSessionAfterRateLimit("sk-test", resetAt, now)
	if !cooldownUntil.Equal(resetAt) {
		t.Fatalf("expected official reset cooldown until %s, got %s", resetAt, cooldownUntil)
	}
	if cooldownSource != CooldownSourceOfficial {
		t.Fatalf("expected official cooldown source, got %s", cooldownSource)
	}
}

func TestAcquireSessionLeaseSkipsBusySession(t *testing.T) {
	cfg := &Config{
		Sessions: []SessionInfo{
			{SessionKey: "sk-a"},
			{SessionKey: "sk-b"},
		},
		MaxConcurrentPerKey:  1,
		MaxGlobalConcurrency: 2,
	}
	now := time.Date(2026, 6, 21, 16, 45, 0, 0, time.Local)

	first := cfg.AcquireSessionLease(0, nil, now)
	if !first.OK || first.Lease.Index != 0 {
		t.Fatalf("expected first lease to use session 0, got ok=%v index=%d reason=%s", first.OK, first.Lease.Index, first.Reason)
	}
	defer first.Lease.Release()

	second := cfg.AcquireSessionLease(0, map[int]bool{0: true}, now.Add(time.Second))
	if !second.OK || second.Lease.Index != 1 {
		t.Fatalf("expected second lease to use session 1, got ok=%v index=%d reason=%s", second.OK, second.Lease.Index, second.Reason)
	}
	second.Lease.Release()
}

func TestAcquireSessionLeaseHonorsGlobalLimit(t *testing.T) {
	cfg := &Config{
		Sessions: []SessionInfo{
			{SessionKey: "sk-a"},
			{SessionKey: "sk-b"},
		},
		MaxConcurrentPerKey:  1,
		MaxGlobalConcurrency: 1,
	}
	now := time.Date(2026, 6, 21, 16, 45, 0, 0, time.Local)

	first := cfg.AcquireSessionLease(0, nil, now)
	if !first.OK {
		t.Fatalf("expected first lease, got %s", first.Reason)
	}
	defer first.Lease.Release()

	second := cfg.AcquireSessionLease(1, nil, now.Add(time.Second))
	if second.OK {
		second.Lease.Release()
		t.Fatal("expected global concurrency limit to block second lease")
	}
	if second.Reason != "all Claude sessions are busy; global concurrency limit reached" {
		t.Fatalf("unexpected reason: %s", second.Reason)
	}
}

func TestAcquireSessionLeaseReleaseMakesSessionAvailable(t *testing.T) {
	cfg := &Config{
		Sessions: []SessionInfo{
			{SessionKey: "sk-a"},
		},
		MaxConcurrentPerKey:  1,
		MaxGlobalConcurrency: 1,
	}
	now := time.Date(2026, 6, 21, 16, 45, 0, 0, time.Local)

	first := cfg.AcquireSessionLease(0, nil, now)
	if !first.OK {
		t.Fatalf("expected first lease, got %s", first.Reason)
	}
	first.Lease.Release()

	second := cfg.AcquireSessionLease(0, nil, now.Add(time.Second))
	if !second.OK {
		t.Fatalf("expected released session to be reusable, got %s", second.Reason)
	}
	second.Lease.Release()
}

func TestNormalizeInternalRetryCount(t *testing.T) {
	if got := NormalizeInternalRetryCount(0); got != DefaultInternalRetryCount {
		t.Fatalf("expected default retry count, got %d", got)
	}
	if got := NormalizeInternalRetryCount(3); got != 3 {
		t.Fatalf("expected retry count 3, got %d", got)
	}
	if got := NormalizeInternalRetryCount(99); got != 10 {
		t.Fatalf("expected retry count cap 10, got %d", got)
	}
}
