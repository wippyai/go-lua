// Package cancellation provides cooperative, solve-scoped cancellation.
package cancellation

import (
	"context"
	"sync/atomic"
)

// Session owns the cancellation signal for one analysis request. Consumers only
// receive its Token; a session is never retained by prepared or cached state.
type Session struct {
	token *Token
}

// Token is safe to share between concurrent traversals. Its cancellation cause
// is terminal and immutable once observed.
type Token struct {
	state atomic.Pointer[cancelState]
	ctx   context.Context
}

type cancelState struct{ err error }

type contextKey struct{}

// Attach returns ctx carrying its solve session. It is idempotent, so nested
// solves recover the same token rather than creating independently cancelable
// work.
func Attach(ctx context.Context) (context.Context, *Session) {
	if ctx == nil {
		ctx = context.Background()
	}
	if session, ok := ctx.Value(contextKey{}).(*Session); ok && session != nil {
		return ctx, session
	}
	session := &Session{token: &Token{ctx: ctx}}
	if err := ctx.Err(); err != nil {
		session.token.Cancel(err)
	} else {
		context.AfterFunc(ctx, func() { session.token.Cancel(ctx.Err()) })
	}
	return context.WithValue(ctx, contextKey{}, session), session
}

// WithSession attaches session to ctx. It is intended for internal adapters
// which already own a solve session and must preserve that identity.
func WithSession(ctx context.Context, session *Session) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if session == nil {
		attached, _ := Attach(ctx)
		return attached
	}
	if existing, ok := ctx.Value(contextKey{}).(*Session); ok && existing == session {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, session)
}

// FromContext returns an attached session when one is present. Callers that
// accept a context but cannot replace it still receive a correctly wired local
// session; public entry points should use Attach so descendants share it.
func FromContext(ctx context.Context) *Session {
	if ctx != nil {
		if session, ok := ctx.Value(contextKey{}).(*Session); ok && session != nil {
			return session
		}
	}
	_, session := Attach(ctx)
	return session
}

// Token returns the session's cooperative cancellation probe.
func (s *Session) Token() *Token {
	if s == nil {
		return nil
	}
	return s.token
}

// Cancel records the first cancellation cause. A nil cause is normalized to
// context.Canceled.
func (t *Token) Cancel(err error) {
	if t == nil {
		return
	}
	if err == nil {
		err = context.Canceled
	}
	t.state.CompareAndSwap(nil, &cancelState{err: err})
}

// Canceled reports whether this token has been canceled.
func (t *Token) Canceled() bool { return t != nil && t.Err() != nil }

// Err returns the cancellation cause, or nil while the token is live.
func (t *Token) Err() error {
	if t == nil {
		return nil
	}
	state := t.state.Load()
	if state == nil {
		if t.ctx == nil || t.ctx.Err() == nil {
			return nil
		}
		t.Cancel(t.ctx.Err())
		state = t.state.Load()
		if state == nil {
			return t.ctx.Err()
		}
	}
	return state.err
}

const (
	// EveryExpensive is appropriate for recursive projection, hashing, and
	// other iterations whose bodies may do substantial work.
	EveryExpensive = 64
	// EveryCheap is appropriate for fact dispatch and worklist bookkeeping.
	EveryCheap = 256
)

// Poller keeps traversal-local polling cadence. It intentionally has no shared
// mutable state: simultaneous traversals must not influence one another.
type Poller struct {
	token *Token
	every uint
	n     uint
}

func NewPoller(token *Token, every uint) Poller {
	if every == 0 {
		every = 1
	}
	return Poller{token: token, every: every}
}

// Poll checks at entry and then every configured number of calls.
func (p *Poller) Poll() bool {
	if p == nil || p.token == nil {
		return false
	}
	p.n++
	if p.n != 1 && p.n%p.every != 0 {
		return false
	}
	return p.token.Canceled()
}
