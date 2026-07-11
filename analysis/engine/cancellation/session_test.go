package cancellation

import (
	"context"
	"errors"
	"testing"
)

func TestAttachIsIdempotentAndPreservesCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attached, first := Attach(ctx)
	_, second := Attach(attached)
	if first != second {
		t.Fatal("nested attachment created a second session")
	}
	cancel()
	<-ctx.Done()
	for !first.Token().Canceled() {
	}
	if !errors.Is(first.Token().Err(), context.Canceled) {
		t.Fatalf("token error = %v, want context cancellation", first.Token().Err())
	}
}

func TestPollerIsLocal(t *testing.T) {
	token := &Token{}
	a := NewPoller(token, 2)
	b := NewPoller(token, 2)
	if a.Poll() || b.Poll() {
		t.Fatal("live token polled canceled")
	}
	token.Cancel(context.DeadlineExceeded)
	if !a.Poll() || !b.Poll() {
		t.Fatal("local pollers did not independently observe cancellation")
	}
}
