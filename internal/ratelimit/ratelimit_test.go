package ratelimit_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/20age1million/WaterlooStar-Backend/internal/ratelimit"
)

// clock is the injected time every test here runs on. No test sleeps: a suite
// that waits for real seconds is a suite that flakes in CI and then gets
// deleted.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var testRule = ratelimit.Rule{Name: "test", Burst: 3, Window: 30 * time.Second}

func TestAllowSpendsTheBurstThenRefuses(t *testing.T) {
	clk := newClock()
	l := ratelimit.New(0, clk.Now)

	for i := range testRule.Burst {
		if d := l.Allow(testRule, "k"); !d.OK {
			t.Fatalf("attempt %d refused, burst is %d", i+1, testRule.Burst)
		}
	}

	d := l.Allow(testRule, "k")
	if d.OK {
		t.Fatal("attempt past the burst was allowed")
	}
	if d.RetryAfter <= 0 {
		t.Fatal("refusal carried no wait, so a client has nothing to act on")
	}
	if d.RetryAfter > testRule.Window {
		t.Fatalf("wait %s exceeds the window %s", d.RetryAfter, testRule.Window)
	}
}

func TestRefillRestoresOneTokenAtATime(t *testing.T) {
	clk := newClock()
	l := ratelimit.New(0, clk.Now)

	for range testRule.Burst {
		l.Allow(testRule, "k")
	}
	if d := l.Allow(testRule, "k"); d.OK {
		t.Fatal("bucket should be empty")
	}

	// One token's worth of time: 30s window over a burst of 3.
	clk.advance(10 * time.Second)
	if d := l.Allow(testRule, "k"); !d.OK {
		t.Fatal("one token should have refilled")
	}
	if d := l.Allow(testRule, "k"); d.OK {
		t.Fatal("only one token should have refilled")
	}

	// A full window restores everything, and never more than the burst.
	clk.advance(2 * testRule.Window)
	for i := range testRule.Burst {
		if d := l.Allow(testRule, "k"); !d.OK {
			t.Fatalf("attempt %d refused after a full refill", i+1)
		}
	}
	if d := l.Allow(testRule, "k"); d.OK {
		t.Fatal("bucket refilled beyond its burst")
	}
}

func TestKeysDoNotShareAnAllowance(t *testing.T) {
	clk := newClock()
	l := ratelimit.New(0, clk.Now)

	for range testRule.Burst {
		l.Allow(testRule, "one")
	}
	if d := l.Allow(testRule, "one"); d.OK {
		t.Fatal("first key should be exhausted")
	}
	if d := l.Allow(testRule, "two"); !d.OK {
		t.Fatal("a second identity was refused for the first one's spending")
	}
}

func TestPeekDoesNotSpendAndSpendDoesNotNeedATokenFirst(t *testing.T) {
	clk := newClock()
	l := ratelimit.New(0, clk.Now)

	// This is the login shape: the middleware peeks, the handler charges only a
	// failed attempt.
	for range 10 {
		if d := l.Peek(testRule, "k"); !d.OK {
			t.Fatal("peeking spent a token")
		}
	}

	for range testRule.Burst {
		l.Spend(testRule, "k")
	}
	if d := l.Peek(testRule, "k"); d.OK {
		t.Fatal("peek should refuse once the burst has been spent")
	}

	// Spending past empty must keep counting down, so a caller who keeps failing
	// does not sit at exactly one token's worth of debt forever.
	before := l.Allow(testRule, "k").RetryAfter
	l.Spend(testRule, "k")
	after := l.Allow(testRule, "k").RetryAfter
	if after <= before {
		t.Fatalf("further failures did not lengthen the wait: %s then %s", before, after)
	}
}

func TestTrackedKeysStayUnderTheCeiling(t *testing.T) {
	clk := newClock()
	const ceiling = 64
	l := ratelimit.New(ceiling, clk.Now)

	// A flood of unique keys is the attack against the limiter itself: without a
	// bound, this is how the limiter becomes the denial-of-service.
	for i := range 10_000 {
		l.Allow(testRule, fmt.Sprintf("key-%d", i))
	}

	if tracked := l.Tracked(); tracked > ceiling {
		t.Fatalf("tracked %d keys, ceiling is %d", tracked, ceiling)
	}
}

func TestAFullBucketIsForgotten(t *testing.T) {
	clk := newClock()
	l := ratelimit.New(0, clk.Now)

	// A bucket back at full holds nothing a new one would not, so holding it
	// would be pure memory. It is forgotten the next time it is read without
	// being spent.
	l.Allow(testRule, "k")
	if l.Tracked() == 0 {
		t.Fatal("a partially spent bucket should be tracked")
	}

	clk.advance(2 * testRule.Window)
	if d := l.Peek(testRule, "k"); !d.OK {
		t.Fatal("a refilled bucket should allow")
	}
	if tracked := l.Tracked(); tracked != 0 {
		t.Fatalf("a refilled bucket was kept: %d tracked", tracked)
	}

	// Forgetting it must not hand out a second burst: the state it is recreated
	// with is the state it had.
	for i := range testRule.Burst {
		if d := l.Allow(testRule, "k"); !d.OK {
			t.Fatalf("attempt %d refused after the bucket was forgotten", i+1)
		}
	}
	if d := l.Allow(testRule, "k"); d.OK {
		t.Fatal("a forgotten bucket allowed more than its burst")
	}
}

func TestRuleWithNoLimitAllowsEverything(t *testing.T) {
	l := ratelimit.New(0, newClock().Now)

	// A zero-value Rule reaching this code would otherwise refuse every request
	// on a route somebody forgot to configure.
	var unset ratelimit.Rule
	for range 100 {
		if d := l.Allow(unset, "k"); !d.OK {
			t.Fatal("an unconfigured rule refused a request")
		}
	}
}

func TestConcurrentCallersSpendExactlyTheBurst(t *testing.T) {
	clk := newClock()
	l := ratelimit.New(0, clk.Now)

	rule := ratelimit.Rule{Name: "race", Burst: 50, Window: time.Hour}

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if d := l.Allow(rule, "shared"); d.OK {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// The clock does not move, so exactly the burst may pass. More would mean
	// two callers both spent the last token.
	if allowed != rule.Burst {
		t.Fatalf("allowed %d of a burst of %d", allowed, rule.Burst)
	}
}

func TestEveryPolicyRuleIsUsable(t *testing.T) {
	clk := newClock()
	l := ratelimit.New(0, clk.Now)

	// A rule with a zero burst or window would silently allow everything, which
	// is how a limit stops being a limit without anyone noticing.
	rules := []ratelimit.Rule{
		ratelimit.LoginPerEmail, ratelimit.LoginGlobal,
		ratelimit.RegisterPerEmail, ratelimit.RegisterGlobal,
		ratelimit.ResetRequestPerEmail, ratelimit.ResetRequestGlobal,
		ratelimit.TokenPerToken, ratelimit.TokenGlobal,
		ratelimit.WritePerUser,
	}
	for _, rule := range rules {
		if rule.Name == "" {
			t.Error("a policy rule has no name, so its refusals cannot be traced")
		}
		if rule.Burst <= 0 || rule.Window <= 0 {
			t.Errorf("%s permits everything: burst %d, window %s", rule.Name, rule.Burst, rule.Window)
			continue
		}
		for range rule.Burst {
			l.Allow(rule, rule.Name)
		}
		if d := l.Allow(rule, rule.Name); d.OK {
			t.Errorf("%s allowed more than its burst of %d", rule.Name, rule.Burst)
		}
	}
}
