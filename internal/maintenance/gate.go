// Package maintenance provides a process-wide gate that briefly stops all
// HTTP/API work while a consistent backup snapshot is taken and a longer full
// stop while restored data and photos are swapped.
package maintenance

import (
	"net/http"
	"sync"
	"sync/atomic"
)

const retryAfterSeconds = "5"

// Gate allows many normal requests at once. Lock waits for all in-flight
// requests to finish, prevents new requests from entering, and blocks the
// caller until the process is quiescent.
type Gate struct {
	active atomic.Bool
	mu     sync.RWMutex
}

func New() *Gate {
	return &Gate{}
}

// TryRead enters the normal request section. It returns false when maintenance
// is active or about to start. Every successful call must be followed by
// ReadUnlock.
func (g *Gate) TryRead() bool {
	if g == nil {
		return true
	}

	// Check before acquiring the read lock: while maintenance holds or is
	// waiting for the write lock, RLock would block instead of allowing callers
	// to fail fast.
	if g.active.Load() {
		return false
	}

	if !g.mu.TryRLock() {
		return false
	}
	if g.active.Load() {
		g.mu.RUnlock()
		return false
	}

	return true
}

func (g *Gate) ReadUnlock() {
	if g == nil {
		return
	}

	g.mu.RUnlock()
}

// Active reports whether maintenance is running. It is cheap and is used by
// background workers that must stop starting new work.
func (g *Gate) Active() bool {
	return g != nil && g.active.Load()
}

// Lock enters maintenance mode and waits for in-flight requests. Every Lock
// must be paired with Unlock.
func (g *Gate) Lock() {
	if g == nil {
		return
	}

	g.active.Store(true)
	g.mu.Lock()
}

func (g *Gate) Unlock() {
	if g == nil {
		return
	}

	g.mu.Unlock()
	g.active.Store(false)
}

// Middleware rejects requests while maintenance is active. Backup/restore
// handlers must not be wrapped with it: they acquire the exclusive lock
// themselves.
func (g *Gate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		if !g.TryRead() {
			w.Header().Set("Retry-After", retryAfterSeconds)
			http.Error(w, "Идёт обслуживание данных. Повторите запрос через несколько секунд.", http.StatusServiceUnavailable)
			return
		}
		defer g.ReadUnlock()

		next.ServeHTTP(w, r)
	})
}
