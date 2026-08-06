package rateLimiter

import (
	"sync"
	"sync/atomic"
	"time"
)

type rateLimitEntry struct {
	lastUpdate int64
	count      int
}

type RateLimiter[T comparable] struct {
	limitEntries            map[T]*rateLimitEntry
	limitEntriesLock        sync.Mutex
	rateLimitCleanerRunning atomic.Bool
	resetTime               time.Duration
	countLimit              int
	maxEntries              int
}

func NewRateLimiter[T comparable](resetTime time.Duration, maxEntries int, countLimit int) *RateLimiter[T] {
	if resetTime < 1*time.Second {
		resetTime = 1 * time.Second
	}
	return &RateLimiter[T]{
		limitEntries: make(map[T]*rateLimitEntry),
		resetTime:    resetTime,
		countLimit:   countLimit,
		maxEntries:   maxEntries,
	}
}

func (r *RateLimiter[T]) rateLimitCleaner() {
	if !r.rateLimitCleanerRunning.CompareAndSwap(false, true) {
		return
	}
	go func() {
		for {
			time.Sleep(r.resetTime)
			r.limitEntriesLock.Lock()
			now := time.Now().Unix()
			for k, v := range r.limitEntries {
				if now-v.lastUpdate > int64(r.resetTime.Seconds()) {
					delete(r.limitEntries, k)
				}
			}
			if len(r.limitEntries) == 0 {
				r.rateLimitCleanerRunning.Store(false)
				r.limitEntriesLock.Unlock()
				return
			}
			r.limitEntriesLock.Unlock()
		}
	}()
}

func (r *RateLimiter[T]) RateLimitOK(key T) bool {
	r.limitEntriesLock.Lock()
	defer r.limitEntriesLock.Unlock()
	r.rateLimitCleaner()

	entry, ok := r.limitEntries[key]
	if !ok {
		entry = &rateLimitEntry{}
		if len(r.limitEntries) >= r.maxEntries {
			return false
		}
		r.limitEntries[key] = entry
	}
	if entry.count >= r.countLimit {
		return false
	}
	entry.lastUpdate = time.Now().Unix()
	entry.count++
	return true
}
