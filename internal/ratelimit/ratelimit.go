// Package ratelimit puts a cost on guessing.
//
// It is a token bucket per key, held in memory. Three properties matter and are
// deliberate:
//
//   - **The clock is injected.** Every test here runs on a fake clock, so the
//     suite asserts refill behaviour without sleeping for real seconds.
//   - **The map is bounded.** A limiter that allocates a bucket per unique input
//     is itself the denial-of-service, so the number of tracked keys has a hard
//     ceiling and the least recently used are evicted at it. A key that has
//     refilled to full is also forgotten the next time it is read, since it then
//     holds nothing a new bucket would not.
//   - **Checking and spending are separate.** Login spends only on a failed
//     attempt, which means the middleware must be able to ask "is there a token"
//     without taking one.
//
// State lives in this process. One API container serves production today, so a
// restart forgets the counters and a second replica would double the effective
// limit. Both are recorded in the feature's specification rather than hidden.
package ratelimit

import (
	"container/list"
	"sync"
	"time"
)

// DefaultMaxKeys is the ceiling on tracked keys. At roughly a hundred bytes per
// bucket this is single-digit megabytes, and an attacker cycling unique keys
// evicts their own earlier buckets rather than growing the process.
const DefaultMaxKeys = 20_000

// Rule is one limit: Burst attempts, refilling to full over Window.
//
// Burst is what a person can do at once; Window is how long a full recovery
// takes. A Burst of 5 over 15 minutes therefore also means "one more attempt
// every three minutes" once the burst is spent.
type Rule struct {
	// Name identifies the rule in logs. It is not part of the key.
	Name   string
	Burst  int
	Window time.Duration
}

// rate returns tokens restored per nanosecond.
func (r Rule) rate() float64 {
	if r.Window <= 0 || r.Burst <= 0 {
		return 0
	}
	return float64(r.Burst) / float64(r.Window)
}

// Decision is the answer to one question about one key.
type Decision struct {
	// OK reports whether the request may proceed.
	OK bool
	// RetryAfter is how long until the next token, rounded up to the second.
	// Zero when OK.
	RetryAfter time.Duration
}

type bucket struct {
	tokens float64
	last   time.Time
	// elem is this bucket's position in the recency list, for eviction.
	elem *list.Element
}

// Limiter holds the buckets. Safe for concurrent use.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	recency *list.List // front = most recently used; values are keys
	maxKeys int
	now     func() time.Time
}

// New builds a limiter. Pass nil for now to use time.Now.
func New(maxKeys int, now func() time.Time) *Limiter {
	if maxKeys <= 0 {
		maxKeys = DefaultMaxKeys
	}
	if now == nil {
		now = time.Now
	}
	return &Limiter{
		buckets: make(map[string]*bucket),
		recency: list.New(),
		maxKeys: maxKeys,
		now:     now,
	}
}

// Allow takes a token when one is available. This is the ordinary path: check
// and consume in one step, under one lock, so two simultaneous requests cannot
// both spend the last token.
func (l *Limiter) Allow(rule Rule, key string) Decision {
	return l.consult(rule, key, true, false)
}

// Peek reports whether a token is available without taking one.
//
// Login uses this: an attempt is only charged for once it is known to have
// failed, so the middleware asks, the handler pays.
func (l *Limiter) Peek(rule Rule, key string) Decision {
	return l.consult(rule, key, false, false)
}

// Spend charges one token whether or not one is available, and reports the
// state afterwards. Used to record a failure the middleware could not have
// known about in advance.
func (l *Limiter) Spend(rule Rule, key string) Decision {
	return l.consult(rule, key, true, true)
}

// consult is the one place the arithmetic lives.
//
// take says whether a token is removed; force says to remove it even when the
// bucket is empty, which keeps a failing caller's debt honest rather than
// letting it stall at zero.
func (l *Limiter) consult(rule Rule, key string, take, force bool) Decision {
	if rule.Burst <= 0 || rule.Window <= 0 {
		// A rule with no limit permits everything. Better than a zero-value rule
		// silently blocking a route.
		return Decision{OK: true}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	full := float64(rule.Burst)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: full, last: now}
		b.elem = l.recency.PushFront(key)
		l.buckets[key] = b
		l.evictLocked()
	} else {
		// Refill for the elapsed time, capped at full.
		if elapsed := now.Sub(b.last); elapsed > 0 {
			b.tokens += float64(elapsed) * rule.rate()
			if b.tokens > full {
				b.tokens = full
			}
			b.last = now
		}
		l.recency.MoveToFront(b.elem)
	}

	switch {
	case b.tokens >= 1:
		if take {
			b.tokens--
			return Decision{OK: true}
		}
		// Nothing was spent, and the bucket has refilled to full: it now holds
		// exactly what a brand-new one would, so keeping it is pure memory.
		// Dropping it here is what lets idle keys fall out between floods, and
		// recreating it yields the same state.
		if b.tokens >= full {
			l.dropLocked(key, b)
		}
		return Decision{OK: true}
	default:
		if take && force {
			b.tokens--
		}
		return Decision{OK: false, RetryAfter: retryAfter(b.tokens, rule)}
	}
}

// retryAfter is how long until the bucket holds one whole token.
func retryAfter(tokens float64, rule Rule) time.Duration {
	rate := rule.rate()
	if rate <= 0 {
		return rule.Window
	}
	needed := 1 - tokens
	if needed <= 0 {
		return 0
	}
	d := time.Duration(needed / rate)
	// Rounded up: a Retry-After of 0 invites an immediate retry that fails.
	if remainder := d % time.Second; remainder != 0 {
		d += time.Second - remainder
	}
	if d < time.Second {
		d = time.Second
	}
	return d
}

// dropLocked forgets a key. Caller holds the lock.
func (l *Limiter) dropLocked(key string, b *bucket) {
	l.recency.Remove(b.elem)
	delete(l.buckets, key)
}

// evictLocked enforces the ceiling by discarding the least recently used keys.
// Caller holds the lock.
func (l *Limiter) evictLocked() {
	for len(l.buckets) > l.maxKeys {
		oldest := l.recency.Back()
		if oldest == nil {
			return
		}
		key, _ := oldest.Value.(string)
		if b, ok := l.buckets[key]; ok {
			l.dropLocked(key, b)
			continue
		}
		l.recency.Remove(oldest)
	}
}

// Tracked reports how many keys are held. Exposed for the test that proves the
// ceiling holds under a flood of unique keys.
func (l *Limiter) Tracked() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// Reset forgets every bucket. Used by tests, never by the service.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = make(map[string]*bucket)
	l.recency.Init()
}
