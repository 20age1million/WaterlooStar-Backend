package live_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/dbtest"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/live"
)

// The listener against a real database: a message committed by one connection
// reaches a subscription through LISTEN.
func TestListenerDeliversCommittedSignals(t *testing.T) {
	pool := dbtest.New(t)
	q := sqlcgen.New(pool)
	owner := dbtest.User(t, q, "listener-owner@uwaterloo.ca", true)
	enquirer := dbtest.User(t, q, "listener-enquirer@uwaterloo.ca", true)
	listing := dbtest.Listing(t, q, owner.ID, dbtest.ListingOptions{Title: "Room"})
	c := dbtest.Conversation(t, q, listing, enquirer.ID)

	hub := live.NewHub(live.MaxStreamsPerUser)
	sub, _ := hub.Subscribe(owner.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener := live.NewListener(pool.Config().ConnString(), hub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go listener.Run(ctx)

	// The listener connects asynchronously; send until a signal arrives.
	deadline := time.After(10 * time.Second)
	for {
		dbtest.Message(t, q, c.ID, enquirer.ID, "Hello")
		select {
		case e := <-sub.Events():
			if e.Signal.ConversationID != c.ID || e.Signal.Change != live.ChangeMessage {
				t.Fatalf("got %+v", e)
			}
			return
		case <-time.After(300 * time.Millisecond):
		case <-deadline:
			t.Fatal("no signal reached the subscription")
		}
	}
}
