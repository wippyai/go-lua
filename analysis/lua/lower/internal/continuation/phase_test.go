package continuation

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"testing"
)

func TestStackIsLIFOAndPublishesOneResultShape(t *testing.T) {
	var stack Stack
	stack.Push(Source)
	stack.Push(Eval)
	if owner, ok := stack.Pop(); !ok || owner != Eval {
		t.Fatalf("Pop = %v/%v, want Eval/true", owner, ok)
	}
	stack.SetResult(keyspace.MakeTerm(keyspace.FamilyInteger, 1), true)
	if term, open := stack.Result(); term == 0 || !open {
		t.Fatalf("Result = %v/%v, want authored open result", term, open)
	}
}
