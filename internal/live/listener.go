package live

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Channel is the NOTIFY channel migration 15's triggers signal on.
const Channel = "conversation_events"

// Listener holds one dedicated database connection LISTENing on Channel and
// hands every signal to a Hub. The connection is outside the pool: a LISTEN
// lives on its session, and a pooled connection would be handed to other work.
type Listener struct {
	dsn        string
	hub        *Hub
	log        *slog.Logger
	minBackoff time.Duration
	maxBackoff time.Duration
}

// NewListener returns a listener for the database at dsn.
func NewListener(dsn string, hub *Hub, log *slog.Logger) *Listener {
	return &Listener{dsn: dsn, hub: hub, log: log, minBackoff: 500 * time.Millisecond, maxBackoff: 30 * time.Second}
}

// Run listens until ctx is done, reconnecting with capped backoff when the
// connection is lost. After a reconnect it resyncs every subscription, because
// signals sent while it was away are gone.
func (l *Listener) Run(ctx context.Context) {
	backoff := l.minBackoff
	connectedBefore := false
	for {
		err := l.session(ctx, connectedBefore, func() {
			connectedBefore = true
			backoff = l.minBackoff
		})
		if ctx.Err() != nil {
			return
		}
		l.log.Warn("live listener lost its connection; reconnecting",
			slog.String("error", errString(err)), slog.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > l.maxBackoff {
			backoff = l.maxBackoff
		}
	}
}

// session runs one connection's lifetime. onListening is called once LISTEN
// has succeeded.
func (l *Listener) session(ctx context.Context, resync bool, onListening func()) error {
	conn, err := pgx.Connect(ctx, l.dsn)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	onListening()
	if resync {
		l.log.Info("live listener reconnected; resyncing streams")
		l.hub.Resync()
	}

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		sig, err := ParsePayload(n.Payload)
		if err != nil {
			l.log.Error("live listener skipped a malformed signal", slog.String("error", err.Error()))
			continue
		}
		l.hub.Publish(sig)
	}
}

// ParsePayload decodes a trigger's payload: {"u": user, "c": conversation,
// "k": change}.
func ParsePayload(payload string) (Signal, error) {
	var p struct {
		U uuid.UUID `json:"u"`
		C uuid.UUID `json:"c"`
		K Change    `json:"k"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return Signal{}, err
	}
	if p.U == uuid.Nil || p.C == uuid.Nil {
		return Signal{}, errors.New("live: signal without a user or conversation")
	}
	if !p.K.Known() {
		return Signal{}, errors.New("live: unknown change " + string(p.K))
	}
	return Signal{UserID: p.U, ConversationID: p.C, Change: p.K}, nil
}

func errString(err error) string {
	if err == nil {
		return "connection ended"
	}
	return err.Error()
}
