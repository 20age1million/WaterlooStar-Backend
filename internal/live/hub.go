// Package live carries conversation signals from PostgreSQL to the students'
// open event streams (GET /me/stream, Phase 19).
//
// Database triggers (migration 15) NOTIFY on the `conversation_events` channel
// whenever a conversation changes. Each API process runs one Listener, which
// LISTENs on that channel and hands every signal to the process's Hub. The Hub
// knows which streams in this process belong to which student and passes the
// signal to each of them.
//
// A signal says which conversation changed and how, and nothing else. A stream
// turns it into an event; the browser re-reads through the ordinary endpoints.
package live

import (
	"errors"
	"sync"

	"github.com/google/uuid"
)

// Change is how a conversation changed. The values are the contract's
// LiveChange enum and the trigger payloads' `k`.
type Change string

const (
	ChangeOpened        Change = "opened"
	ChangeMessage       Change = "message"
	ChangeRead          Change = "read"
	ChangeClosed        Change = "closed"
	ChangeAddressShared Change = "address_shared"
	ChangeAvailability  Change = "availability"
)

// Known reports whether c is a change the contract documents.
func (c Change) Known() bool {
	switch c {
	case ChangeOpened, ChangeMessage, ChangeRead, ChangeClosed, ChangeAddressShared, ChangeAvailability:
		return true
	}
	return false
}

// Signal is one trigger payload: for this user, this conversation changed.
type Signal struct {
	UserID         uuid.UUID
	ConversationID uuid.UUID
	Change         Change
}

// Event is what a subscription receives: a signal, or a resync telling it that
// signals may have been missed.
type Event struct {
	Signal Signal
	Resync bool
}

// MaxStreamsPerUser is how many streams one account may hold open in one
// process: several tabs and a phone.
const MaxStreamsPerUser = 5

// subscriptionBuffer is how many events a subscription may fall behind by
// before it is dropped. A stream writes each event as it arrives, so only a
// stalled connection fills it.
const subscriptionBuffer = 16

var (
	// ErrTooManyStreams refuses a subscription beyond MaxStreamsPerUser.
	ErrTooManyStreams = errors.New("live: too many open streams for this account")
	// ErrClosed refuses a subscription after the hub has closed (shutdown).
	ErrClosed = errors.New("live: hub closed")
)

// Hub routes signals to the subscriptions of the user they name. Safe for
// concurrent use.
type Hub struct {
	mu      sync.Mutex
	perUser int
	subs    map[uuid.UUID]map[*Subscription]struct{}
	closed  bool
}

// NewHub returns an empty hub allowing perUser subscriptions per account.
func NewHub(perUser int) *Hub {
	return &Hub{perUser: perUser, subs: map[uuid.UUID]map[*Subscription]struct{}{}}
}

// Subscription is one open stream's place in the hub.
type Subscription struct {
	hub    *Hub
	user   uuid.UUID
	events chan Event
	done   chan struct{}
	once   sync.Once
}

// Events delivers this subscription's signals and resyncs.
func (s *Subscription) Events() <-chan Event { return s.events }

// Done is closed when the subscription ends: closed by its stream, dropped for
// falling behind, or ended by the hub closing.
func (s *Subscription) Done() <-chan struct{} { return s.done }

// Close removes the subscription from the hub. Safe to call more than once.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.hub.removeLocked(s)
}

// Subscribe adds a subscription for user.
func (h *Hub) Subscribe(user uuid.UUID) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	set := h.subs[user]
	if len(set) >= h.perUser {
		return nil, ErrTooManyStreams
	}
	if set == nil {
		set = map[*Subscription]struct{}{}
		h.subs[user] = set
	}
	s := &Subscription{
		hub: h, user: user,
		events: make(chan Event, subscriptionBuffer),
		done:   make(chan struct{}),
	}
	set[s] = struct{}{}
	return s, nil
}

// Publish hands sig to every subscription of its user. It never blocks: a
// subscription whose buffer is full is dropped, so one stalled connection
// cannot hold up the others. Its browser reconnects and re-reads.
func (h *Hub) Publish(sig Signal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[sig.UserID] {
		h.deliverLocked(s, Event{Signal: sig})
	}
}

// Resync tells every subscription that signals may have been missed.
func (h *Hub) Resync() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for s := range set {
			h.deliverLocked(s, Event{Resync: true})
		}
	}
}

// Close ends every subscription and refuses new ones. Called on shutdown, so
// open streams end at once instead of holding the server's drain open.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, set := range h.subs {
		for s := range set {
			h.removeLocked(s)
		}
	}
}

// Count reports how many subscriptions user holds. For tests and diagnostics.
func (h *Hub) Count(user uuid.UUID) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[user])
}

func (h *Hub) deliverLocked(s *Subscription, e Event) {
	select {
	case s.events <- e:
	default:
		h.removeLocked(s)
	}
}

func (h *Hub) removeLocked(s *Subscription) {
	set := h.subs[s.user]
	if _, ok := set[s]; ok {
		delete(set, s)
		if len(set) == 0 {
			delete(h.subs, s.user)
		}
	}
	s.once.Do(func() { close(s.done) })
}
