package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Live updates (Phase 19): the triggers in migration 15.
//
// Each change a participant would see in an open conversation must signal
// exactly the listed recipients, and an unrelated change must signal nobody.
// A signal names a recipient, a conversation and a change - nothing more.

type signal struct {
	User         uuid.UUID `json:"u"`
	Conversation uuid.UUID `json:"c"`
	Change       string    `json:"k"`
}

// listen holds a connection LISTENing on the triggers' channel. drain returns
// what arrived after the change it is called after, sorted for comparison.
type listen struct {
	t    *testing.T
	conn *pgxpool.Conn
}

func startListening(t *testing.T, pool *pgxpool.Pool) *listen {
	t.Helper()
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	t.Cleanup(conn.Release)
	if _, err := conn.Exec(context.Background(), "LISTEN conversation_events"); err != nil {
		t.Fatalf("listen: %v", err)
	}
	return &listen{t: t, conn: conn}
}

func (l *listen) drain() []signal {
	l.t.Helper()
	var got []signal
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		n, err := l.conn.Conn().WaitForNotification(ctx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			l.t.Fatalf("wait: %v", err)
		}
		var s signal
		if err := json.Unmarshal([]byte(n.Payload), &s); err != nil {
			l.t.Fatalf("payload %q: %v", n.Payload, err)
		}
		// A signal carries these three keys and nothing else.
		var keys map[string]any
		_ = json.Unmarshal([]byte(n.Payload), &keys)
		if len(keys) != 3 {
			l.t.Errorf("payload %q has %d keys, want 3", n.Payload, len(keys))
		}
		got = append(got, s)
	}
	sortSignals(got)
	return got
}

func sortSignals(s []signal) {
	sort.Slice(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if a.Change != b.Change {
			return a.Change < b.Change
		}
		if a.User != b.User {
			return a.User.String() < b.User.String()
		}
		return a.Conversation.String() < b.Conversation.String()
	})
}

func expectSignals(t *testing.T, what string, got []signal, want ...signal) {
	t.Helper()
	sortSignals(want)
	if len(got) != len(want) {
		t.Fatalf("%s: got %d signals %+v, want %d %+v", what, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: signal %d = %+v, want %+v", what, i, got[i], want[i])
		}
	}
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestLiveSignalsForConversationChanges(t *testing.T) {
	pool := dbtest.New(t)
	q := sqlcgen.New(pool)
	p := newPair(t, q, "live-")
	l := startListening(t, pool)

	c := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)
	expectSignals(t, "opened", l.drain(),
		signal{p.owner.ID, c.ID, "opened"}, signal{p.enquirer.ID, c.ID, "opened"})

	dbtest.Message(t, q, c.ID, p.enquirer.ID, "Is it still free?")
	expectSignals(t, "message", l.drain(),
		signal{p.owner.ID, c.ID, "message"}, signal{p.enquirer.ID, c.ID, "message"})

	// A read goes to the reader only.
	exec(t, pool, `UPDATE conversations SET owner_read_at = now() WHERE id = $1`, c.ID)
	expectSignals(t, "owner read", l.drain(), signal{p.owner.ID, c.ID, "read"})
	exec(t, pool, `UPDATE conversations SET enquirer_read_at = now() WHERE id = $1`, c.ID)
	expectSignals(t, "enquirer read", l.drain(), signal{p.enquirer.ID, c.ID, "read"})

	exec(t, pool, `UPDATE conversations SET address_shared_at = now() WHERE id = $1`, c.ID)
	expectSignals(t, "address shared", l.drain(),
		signal{p.owner.ID, c.ID, "address_shared"}, signal{p.enquirer.ID, c.ID, "address_shared"})

	exec(t, pool, `UPDATE conversations SET closed_at = now(), closed_by = $2 WHERE id = $1`, c.ID, p.owner.ID)
	expectSignals(t, "closed", l.drain(),
		signal{p.owner.ID, c.ID, "closed"}, signal{p.enquirer.ID, c.ID, "closed"})

	// Touching a conversation without changing what a participant sees.
	exec(t, pool, `UPDATE conversations SET updated_at = now() WHERE id = $1`, c.ID)
	expectSignals(t, "unrelated conversation update", l.drain())
}

func TestLiveSignalsForModeration(t *testing.T) {
	pool := dbtest.New(t)
	q := sqlcgen.New(pool)
	p := newPair(t, q, "livemod-")
	second := dbtest.User(t, q, "livemod-second@uwaterloo.ca", true)
	c1 := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)
	c2 := dbtest.Conversation(t, q, p.listing, second.ID)
	l := startListening(t, pool)

	// A title edit changes nothing a conversation's availability depends on.
	exec(t, pool, `UPDATE listings SET title = 'Renamed room' WHERE id = $1`, p.listing.ID)
	expectSignals(t, "title edit", l.drain())

	// Taken down: both sides of every conversation about the listing.
	exec(t, pool, `UPDATE listings SET removed_at = now(), removed_reason = 'Spam' WHERE id = $1`, p.listing.ID)
	expectSignals(t, "takedown", l.drain(),
		signal{p.owner.ID, c1.ID, "availability"}, signal{p.enquirer.ID, c1.ID, "availability"},
		signal{p.owner.ID, c2.ID, "availability"}, signal{second.ID, c2.ID, "availability"})

	exec(t, pool, `UPDATE listings SET removed_at = NULL, removed_reason = NULL, address_line = '12 New St' WHERE id = $1`, p.listing.ID)
	expectSignals(t, "restored and corrected", l.drain(),
		signal{p.owner.ID, c1.ID, "availability"}, signal{p.enquirer.ID, c1.ID, "availability"},
		signal{p.owner.ID, c2.ID, "availability"}, signal{second.ID, c2.ID, "availability"})

	// The owner suspended: the other side of each of the owner's conversations.
	exec(t, pool, `UPDATE users SET suspended_at = now(), suspend_reason = 'Spam' WHERE id = $1`, p.owner.ID)
	expectSignals(t, "owner suspended", l.drain(),
		signal{p.enquirer.ID, c1.ID, "availability"}, signal{second.ID, c2.ID, "availability"})

	// An enquirer suspended: the owner, about that conversation only.
	exec(t, pool, `UPDATE users SET suspended_at = now(), suspend_reason = 'Spam' WHERE id = $1`, second.ID)
	expectSignals(t, "enquirer suspended", l.drain(), signal{p.owner.ID, c2.ID, "availability"})

	// Another user column changing signals nothing.
	exec(t, pool, `UPDATE users SET email_notifications = false WHERE id = $1`, p.enquirer.ID)
	expectSignals(t, "preference change", l.drain())
}

func TestLiveSignalsOnlyOnCommit(t *testing.T) {
	pool := dbtest.New(t)
	q := sqlcgen.New(pool)
	p := newPair(t, q, "livetx-")
	c := dbtest.Conversation(t, q, p.listing, p.enquirer.ID)
	l := startListening(t, pool)

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(),
		`INSERT INTO messages (conversation_id, sender_id, body) VALUES ($1, $2, 'never sent')`, c.ID, p.owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	expectSignals(t, "rolled back message", l.drain())
}
