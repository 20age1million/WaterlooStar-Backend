package live_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/live"
)

func receive(t *testing.T, s *live.Subscription) live.Event {
	t.Helper()
	select {
	case e := <-s.Events():
		return e
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	return live.Event{}
}

func nothing(t *testing.T, s *live.Subscription) {
	t.Helper()
	select {
	case e := <-s.Events():
		t.Fatalf("unexpected event %+v", e)
	default:
	}
}

func TestPublishReachesEverySubscriptionOfThatUserOnly(t *testing.T) {
	h := live.NewHub(live.MaxStreamsPerUser)
	alice, bob := uuid.New(), uuid.New()
	a1, _ := h.Subscribe(alice)
	a2, _ := h.Subscribe(alice)
	b1, _ := h.Subscribe(bob)

	sig := live.Signal{UserID: alice, ConversationID: uuid.New(), Change: live.ChangeMessage}
	h.Publish(sig)

	if got := receive(t, a1); got.Signal != sig || got.Resync {
		t.Errorf("a1 got %+v", got)
	}
	if got := receive(t, a2); got.Signal != sig {
		t.Errorf("a2 got %+v", got)
	}
	nothing(t, b1)
}

func TestSubscribeRefusesBeyondTheLimit(t *testing.T) {
	h := live.NewHub(2)
	u := uuid.New()
	s1, _ := h.Subscribe(u)
	if _, err := h.Subscribe(u); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Subscribe(u); !errors.Is(err, live.ErrTooManyStreams) {
		t.Fatalf("third subscription: err = %v, want ErrTooManyStreams", err)
	}
	// Another user is unaffected, and closing one frees a place.
	if _, err := h.Subscribe(uuid.New()); err != nil {
		t.Fatal(err)
	}
	s1.Close()
	s1.Close() // idempotent
	if _, err := h.Subscribe(u); err != nil {
		t.Fatalf("after closing one: %v", err)
	}
}

func TestASlowSubscriptionIsDroppedWithoutBlockingOthers(t *testing.T) {
	h := live.NewHub(live.MaxStreamsPerUser)
	u := uuid.New()
	slow, _ := h.Subscribe(u)
	fast, _ := h.Subscribe(u)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			h.Publish(live.Signal{UserID: u, ConversationID: uuid.New(), Change: live.ChangeMessage})
			<-fast.Events()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publishing blocked on a stalled subscription")
	}
	select {
	case <-slow.Done():
	default:
		t.Fatal("the stalled subscription was not dropped")
	}
	if h.Count(u) != 1 {
		t.Errorf("Count = %d, want 1 (the fast one)", h.Count(u))
	}
}

func TestResyncReachesEveryone(t *testing.T) {
	h := live.NewHub(live.MaxStreamsPerUser)
	a, _ := h.Subscribe(uuid.New())
	b, _ := h.Subscribe(uuid.New())
	h.Resync()
	if !receive(t, a).Resync || !receive(t, b).Resync {
		t.Fatal("resync not delivered")
	}
}

func TestCloseEndsSubscriptionsAndRefusesNew(t *testing.T) {
	h := live.NewHub(live.MaxStreamsPerUser)
	s, _ := h.Subscribe(uuid.New())
	h.Close()
	select {
	case <-s.Done():
	default:
		t.Fatal("subscription still open after Close")
	}
	if _, err := h.Subscribe(uuid.New()); !errors.Is(err, live.ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

func TestParsePayload(t *testing.T) {
	u, c := uuid.New(), uuid.New()
	sig, err := live.ParsePayload(`{"u":"` + u.String() + `","c":"` + c.String() + `","k":"closed"}`)
	if err != nil {
		t.Fatal(err)
	}
	if sig != (live.Signal{UserID: u, ConversationID: c, Change: live.ChangeClosed}) {
		t.Errorf("got %+v", sig)
	}
	for _, bad := range []string{
		`not json`,
		`{"u":"` + u.String() + `","c":"` + c.String() + `","k":"deleted"}`,
		`{"c":"` + c.String() + `","k":"message"}`,
	} {
		if _, err := live.ParsePayload(bad); err == nil {
			t.Errorf("ParsePayload(%s) accepted", bad)
		}
	}
}
