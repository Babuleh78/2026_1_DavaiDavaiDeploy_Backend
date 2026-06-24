package events

import (
	"sync"
	"testing"
)

// TestAcquireRelease_Balance covers the per-user connection cap and the
// acquire/release counter that CLAUDE.md flags as must-stay-balanced.
func TestAcquireRelease_Balance(t *testing.T) {
	h := NewHandler("")
	const user = "user-1"

	// First maxConnsPerUser acquisitions succeed.
	for i := range maxConnsPerUser {
		if !h.acquire(user) {
			t.Fatalf("acquire %d failed, expected success below the cap", i+1)
		}
	}
	// The next one is over the cap and must be rejected.
	if h.acquire(user) {
		t.Fatal("acquire over the cap succeeded, want rejection")
	}

	// Releasing one frees a slot so a new acquire succeeds again.
	h.release(user)
	if !h.acquire(user) {
		t.Fatal("acquire after release failed, slot was not freed")
	}

	// Releasing everything must remove the user from the map entirely.
	for range maxConnsPerUser {
		h.release(user)
	}
	h.mu.Lock()
	_, present := h.conns[user]
	h.mu.Unlock()
	if present {
		t.Errorf("user still present in conns map after full release: %v", h.conns)
	}
}

func TestAcquireRelease_IsolatedPerUser(t *testing.T) {
	h := NewHandler("")
	// Exhaust user-a's slots.
	for range maxConnsPerUser {
		h.acquire("user-a")
	}
	if h.acquire("user-a") {
		t.Fatal("user-a over cap should be rejected")
	}
	// user-b is unaffected.
	if !h.acquire("user-b") {
		t.Fatal("user-b should acquire its own first slot")
	}
}

// TestAcquireRelease_ConcurrentBalance runs balanced acquire/release pairs from
// many goroutines; the map must end empty (no leaked or negative counts).
func TestAcquireRelease_ConcurrentBalance(t *testing.T) {
	h := NewHandler("")
	const user = "user-x"

	var wg sync.WaitGroup
	for range 200 {
		wg.Go(func() {
			if h.acquire(user) {
				h.release(user)
			}
		})
	}
	wg.Wait()

	h.mu.Lock()
	count := h.conns[user]
	h.mu.Unlock()
	if count != 0 {
		t.Errorf("residual connection count = %d, want 0 after balanced acquire/release", count)
	}
}
