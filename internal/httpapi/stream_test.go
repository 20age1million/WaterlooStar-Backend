package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/live"
)

// GET /me/stream (Phase 19). The in-memory fake has no triggers, so these
// tests publish to the hub directly; the triggers are tested against real
// PostgreSQL in internal/db, and the listener in internal/live.

type sseFrame struct {
	event, data, comment string
}

// openStream opens the stream with h's session and returns its frames as they
// arrive. The request ends when the test does.
func openStream(t *testing.T, h *harness, cookies ...*http.Cookie) (*http.Response, <-chan sseFrame) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.server.URL+"/me/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := h.client
	if len(cookies) > 0 {
		client = &http.Client{}
		for _, c := range cookies {
			req.AddCookie(c)
		}
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })

	frames := make(chan sseFrame, 32)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(res.Body)
		var f sseFrame
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				frames <- f
				f = sseFrame{}
			case strings.HasPrefix(line, "event: "):
				f.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				f.data = strings.TrimPrefix(line, "data: ")
			case strings.HasPrefix(line, ": "):
				f.comment = strings.TrimPrefix(line, ": ")
			}
		}
	}()
	return res, frames
}

func nextFrame(t *testing.T, frames <-chan sseFrame) sseFrame {
	t.Helper()
	select {
	case f, ok := <-frames:
		if !ok {
			t.Fatal("stream ended")
		}
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("no frame within 3s")
	}
	return sseFrame{}
}

func signedInUserID(t *testing.T, h *harness) uuid.UUID {
	t.Helper()
	res := h.do(t, http.MethodGet, "/me", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /me: %d", res.StatusCode)
	}
	return uuid.MustParse(decode[userBody](t, res).ID)
}

func TestStreamNeedsASession(t *testing.T) {
	h := newHarness(t)
	res := h.do(t, http.MethodGet, "/me/stream", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func TestStreamSendsReadyThenSignalsWithUnreadCounts(t *testing.T) {
	owner, enquirer, listingID := conversationParties(t, newHarness(t))
	c := started(t, enquirer, listingID)
	ownerID := signedInUserID(t, owner)

	res, frames := openStream(t, owner)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	for header, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-store",
		"X-Accel-Buffering": "no",
	} {
		if got := res.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	ready := nextFrame(t, frames)
	if ready.event != "ready" {
		t.Fatalf("first event = %q, want ready", ready.event)
	}
	var r struct {
		Unread unreadJSON `json:"unread"`
		Resync bool       `json:"resync"`
	}
	if err := json.Unmarshal([]byte(ready.data), &r); err != nil {
		t.Fatal(err)
	}
	if r.Unread.Conversations != 1 || r.Unread.Messages != 1 || r.Resync {
		t.Errorf("ready = %+v, want one unread conversation and message, no resync", r)
	}

	// As the trigger would signal a reply.
	send(t, enquirer, c.ID, "Also, is parking included?")
	owner.api.LiveHub().Publish(live.Signal{UserID: ownerID, ConversationID: uuid.MustParse(c.ID), Change: live.ChangeMessage})

	conv := nextFrame(t, frames)
	if conv.event != "conversation" {
		t.Fatalf("event = %q, want conversation", conv.event)
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(conv.data), &ev); err != nil {
		t.Fatal(err)
	}
	if ev["conversation_id"] != c.ID || ev["change"] != "message" || len(ev) != 2 {
		t.Errorf("conversation event = %v, want only the id and change", ev)
	}
	if strings.Contains(conv.data, "parking") {
		t.Error("an event carried message text")
	}

	counts := nextFrame(t, frames)
	if counts.event != "unread" {
		t.Fatalf("event = %q, want unread", counts.event)
	}
	if u := decodeString[unreadJSON](t, counts.data); u.Messages != 2 {
		t.Errorf("unread = %+v, want 2 messages", u)
	}

	// A resync re-sends ready.
	owner.api.LiveHub().Resync()
	again := nextFrame(t, frames)
	if again.event != "ready" || !strings.Contains(again.data, `"resync":true`) {
		t.Errorf("after resync: %+v", again)
	}
}

func TestStreamSendsAHeartbeatWhenSilent(t *testing.T) {
	h := newHarness(t)
	h.api.SetStreamHeartbeat(50 * time.Millisecond)
	register(t, h, "beat@uwaterloo.ca", "beat")

	_, frames := openStream(t, h)
	nextFrame(t, frames) // ready
	if f := nextFrame(t, frames); f.comment != "ping" {
		t.Fatalf("frame = %+v, want a ping comment", f)
	}
}

func TestStreamEndsWhenTheAccessTokenExpires(t *testing.T) {
	h := newHarness(t)
	register(t, h, "expiring@uwaterloo.ca", "expiring")
	id := signedInUserID(t, h)

	// A token minted so that it has one second left.
	minted := auth.NewTokenServiceAt("a-test-secret-that-is-long-enough-to-pass",
		func() time.Time { return time.Now().Add(-auth.AccessTokenTTL + time.Second) })
	token, _, err := minted.MintAccessToken(auth.Principal{UserID: id, Role: "user", Verified: true})
	if err != nil {
		t.Fatal(err)
	}

	_, frames := openStream(t, h, &http.Cookie{Name: auth.SessionCookieName, Value: token})
	nextFrame(t, frames) // ready
	if f := nextFrame(t, frames); f.event != "expired" {
		t.Fatalf("frame = %+v, want expired", f)
	}
	select {
	case _, ok := <-frames:
		if ok {
			t.Fatal("stream continued after expired")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not end after expired")
	}
}

func TestASixthStreamIsRefused(t *testing.T) {
	h := newHarness(t)
	register(t, h, "tabs@uwaterloo.ca", "tabs")
	for i := 0; i < live.MaxStreamsPerUser; i++ {
		res, frames := openStream(t, h)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("stream %d: %d", i+1, res.StatusCode)
		}
		nextFrame(t, frames)
	}
	res, _ := openStream(t, h)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("sixth stream: %d, want 429", res.StatusCode)
	}
}

func decodeString[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return v
}
