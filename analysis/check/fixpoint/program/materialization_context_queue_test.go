package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestMaterializationContextQueueConsumesNewContextsOnce(t *testing.T) {
	fn := &ast.FunctionExpr{}
	first := summary.DefaultSummaryKey(ref.FromSymbol(1))
	first.Entry.Facts = 1
	second := summary.DefaultSummaryKey(ref.FromSymbol(1))
	second.Entry.Facts = 2
	keys := &programKeys{contexts: newContextIndex()}
	keys.contexts.appendContext(fn, first, state.State{}, nil)

	queue := newMaterializationContextQueue(keys)
	got, ok := queue.Next()
	if !ok || got.key != first {
		t.Fatalf("first queue item = %#v/%v, want first", got.key, ok)
	}

	keys.contexts.appendContext(fn, first, state.State{}, nil)
	keys.contexts.appendContext(fn, second, state.State{}, nil)
	got, ok = queue.Next()
	if !ok || got.key != second {
		t.Fatalf("second queue item = %#v/%v, want newly appended second", got.key, ok)
	}
	if got, ok = queue.Next(); ok {
		t.Fatalf("queue produced duplicate or unexpected context: %#v", got.key)
	}
}
