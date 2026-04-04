package service

import (
	"sync"
	"time"
)

type attemptState struct {
	failures  int
	lockedTil time.Time
	windowTil time.Time
}

type slidingState struct {
	count     int
	windowTil time.Time
}

// AccessGuard は verify/download の悪用対策を担う。
type AccessGuard struct {
	mu sync.Mutex

	verify map[string]attemptState
	dl     map[string]slidingState

	VerifyMaxAttempts int
	VerifyLockout     time.Duration
	DownloadLimit     int
	DownloadWindow    time.Duration
	now               func() time.Time
}

func NewAccessGuard() *AccessGuard {
	return &AccessGuard{
		verify:            map[string]attemptState{},
		dl:                map[string]slidingState{},
		VerifyMaxAttempts: 5,
		VerifyLockout:     10 * time.Minute,
		DownloadLimit:     10,
		DownloadWindow:    time.Minute,
		now:               time.Now,
	}
}

func (g *AccessGuard) VerifyAllowed(token string) bool {
	now := g.now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.verify[token]
	if now.Before(st.lockedTil) {
		return false
	}
	return true
}

func (g *AccessGuard) RegisterVerifyFailure(token string) {
	now := g.now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.verify[token]
	if now.After(st.windowTil) {
		st.failures = 0
		st.windowTil = now.Add(g.VerifyLockout)
	}
	st.failures++
	if st.failures >= g.VerifyMaxAttempts {
		st.lockedTil = now.Add(g.VerifyLockout)
		st.failures = 0
		st.windowTil = st.lockedTil
	}
	g.verify[token] = st
}

func (g *AccessGuard) ResetVerify(token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.verify, token)
}

func (g *AccessGuard) AllowDownload(key string) bool {
	now := g.now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.dl[key]
	if now.After(st.windowTil) {
		st = slidingState{count: 1, windowTil: now.Add(g.DownloadWindow)}
		g.dl[key] = st
		return true
	}
	if st.count >= g.DownloadLimit {
		return false
	}
	st.count++
	g.dl[key] = st
	return true
}
