package ratelimit

import "time"

// Every limit in the service is in this file, with what it protects and why the
// number. A limit nobody can find is a limit nobody will tune.
//
// The shape of all of them is the same: a burst a real person could plausibly
// need, refilling over a window long enough that a script gains nothing. None of
// them is a lockout — buckets always refill, because a lockout would let an
// attacker deny a student their own account for free.
var (
	// LoginPerEmail is charged only for a *failed* attempt. Five wrong passwords
	// is more than a person mistypes; after that, one more every three minutes.
	// A correct password costs nothing, so normal use never meets this.
	LoginPerEmail = Rule{Name: "login_per_email", Burst: 5, Window: 15 * time.Minute}

	// LoginGlobal catches the attacker who varies the address to stay under the
	// per-email limit. One failed login per second, service-wide, sustained — far
	// above real traffic on a site this size and far below what a script wants.
	LoginGlobal = Rule{Name: "login_global", Burst: 60, Window: time.Minute}

	// RegisterPerEmail: an address can only be registered once, so repeated
	// attempts are a script or a misunderstanding. Three is enough to recover
	// from a validation error.
	RegisterPerEmail = Rule{Name: "register_per_email", Burst: 3, Window: time.Hour}

	// RegisterGlobal caps account creation overall, which is the one that stops
	// the users table being filled.
	RegisterGlobal = Rule{Name: "register_global", Burst: 30, Window: 10 * time.Minute}

	// ResetRequestPerEmail is the tightest of the three per-address limits,
	// because this endpoint sends mail to someone else's inbox on demand. Once
	// email delivery exists, an uncapped version of this is a way to harass a
	// person using our name and our sending reputation.
	ResetRequestPerEmail = Rule{Name: "reset_request_per_email", Burst: 3, Window: time.Hour}

	// ResetRequestGlobal bounds the outbound mail bill.
	ResetRequestGlobal = Rule{Name: "reset_request_global", Burst: 30, Window: time.Hour}

	// TokenPerToken caps attempts against one confirmation or reset token. The
	// tokens are 32 random bytes, so this is not what stops brute force — it
	// stops a replay loop against a token someone has.
	TokenPerToken = Rule{Name: "token_per_token", Burst: 10, Window: time.Hour}

	// TokenGlobal is what actually bounds guessing at tokens, since every guess
	// is a different key and would otherwise get its own fresh bucket.
	TokenGlobal = Rule{Name: "token_global", Burst: 60, Window: 10 * time.Minute}

	// WritePerUser is a person posting, not a script filling the hub. Thirty
	// posts an hour is far more than anyone has ever needed and still leaves a
	// spammer with a number an admin can clear up by hand.
	WritePerUser = Rule{Name: "write_per_user", Burst: 30, Window: time.Hour}
)

// GlobalKey is the key every service-wide backstop shares, one per rule name.
// The rule name is part of it so the backstops do not share a bucket.
func GlobalKey(rule Rule) string { return "global:" + rule.Name }
