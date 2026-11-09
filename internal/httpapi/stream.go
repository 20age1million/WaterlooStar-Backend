package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
	"github.com/20age1million/WaterlooStar-Backend/internal/live"
)

// StreamHeartbeat is how long a stream may stay silent before a comment line
// is sent, inside nginx's 60-second proxy_read_timeout.
const StreamHeartbeat = 25 * time.Second

// streamMyEvents serves GET /me/stream (Phase 19): server-sent events telling
// this tab when one of the student's conversations changes, with fresh unread
// counts. Registered on gin by hand, not through the strict handler, because
// each event has to be flushed as it is written.
//
// The events are signals. None carries a message's text, an address or the
// other student's id; the client re-reads through the ordinary endpoints.
func (s *Server) streamMyEvents(c *gin.Context) {
	ctx := c.Request.Context()
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		apierror.Write(c, http.StatusUnauthorized, apierror.CodeUnauthorized, "Log in to get live updates.")
		return
	}

	sub, err := s.live.Subscribe(principal.UserID)
	switch {
	case errors.Is(err, live.ErrTooManyStreams):
		apierror.TooManyRequests(c,
			fmt.Sprintf("You have live updates open in %d places already. Close one and try again.", live.MaxStreamsPerUser),
			time.Minute)
		return
	case err != nil:
		apierror.Write(c, http.StatusServiceUnavailable, apierror.CodeUnavailable, "The service is restarting. Try again shortly.")
		return
	}
	defer sub.Close()

	unread, err := s.unreadFor(c, principal)
	if err != nil {
		s.log.Error("stream unread counts", slog.String("error", err.Error()))
		apierror.Internal(c)
		return
	}

	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("Connection", "keep-alive")
	// nginx buffers proxied responses by default, which would hold events
	// back until a buffer fills. This header turns that off per response.
	h.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	w := &sseWriter{c: c}
	if !w.event("ready", gen.LiveReady{Unread: unread}) {
		return
	}

	// The stream ends when the session's access token does: an account that
	// signed out or was suspended stops hearing within AccessTokenTTL, and
	// the browser reconnects through the frontend, which refreshes first.
	var expired <-chan time.Time
	if !principal.ExpiresAt.IsZero() {
		t := time.NewTimer(time.Until(principal.ExpiresAt))
		defer t.Stop()
		expired = t.C
	}
	heartbeat := time.NewTimer(s.heartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Done():
			return
		case <-expired:
			w.event("expired", struct{}{})
			return
		case <-heartbeat.C:
			if !w.comment("ping") {
				return
			}
		case e := <-sub.Events():
			if !s.writeLiveEvent(c, w, principal, e) {
				return
			}
		}
		heartbeat.Reset(s.heartbeat)
	}
}

// writeLiveEvent turns one hub event into stream events. False ends the stream.
func (s *Server) writeLiveEvent(c *gin.Context, w *sseWriter, principal auth.Principal, e live.Event) bool {
	if !e.Resync {
		ok := w.event("conversation", gen.LiveConversationEvent{
			ConversationId: openapi_types.UUID(e.Signal.ConversationID),
			Change:         gen.LiveChange(e.Signal.Change),
		})
		if !ok {
			return false
		}
	}
	unread, err := s.unreadFor(c, principal)
	if err != nil {
		// The signal went out; only the counts are missing. Ending the stream
		// makes the browser reconnect and start from a fresh `ready`.
		s.log.Error("stream unread counts", slog.String("error", err.Error()))
		return false
	}
	if e.Resync {
		return w.event("ready", gen.LiveReady{Unread: unread, Resync: true})
	}
	return w.event("unread", unread)
}

func (s *Server) unreadFor(c *gin.Context, principal auth.Principal) (gen.UnreadCounts, error) {
	counts, err := s.queries.UnreadCountsForUser(c.Request.Context(), principal.UserID)
	if err != nil {
		return gen.UnreadCounts{}, err
	}
	return gen.UnreadCounts{Conversations: int(counts.Conversations), Messages: int(counts.Messages)}, nil
}

// sseWriter writes server-sent events and flushes each one.
type sseWriter struct{ c *gin.Context }

func (w *sseWriter) event(name string, data any) bool {
	body, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w.c.Writer, "event: %s\ndata: %s\n\n", name, body); err != nil {
		return false
	}
	w.c.Writer.Flush()
	return true
}

func (w *sseWriter) comment(text string) bool {
	if _, err := fmt.Fprintf(w.c.Writer, ": %s\n\n", text); err != nil {
		return false
	}
	w.c.Writer.Flush()
	return true
}
